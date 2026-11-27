// Package apiclient provides a bounded local client for the public REST and SSE API.
package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/atno11/cs2-server-manager/internal/apitypes"
)

const maxRESTBody = 64 * 1024
const restTimeout = 15 * time.Second

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// StatusError reports an HTTP status without including credentials or response bodies.
type StatusError struct {
	Method string
	Code   int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("api %s: status %d", e.Method, e.Code)
}

type Client struct {
	base      *url.URL
	token     string
	http      *http.Client
	transport *http.Transport
}

// New accepts only a numeric loopback HTTP authority with no URL credentials,
// path, query or fragment. The supplied bearer token is never put in a URL.
func New(endpoint, token string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "http" || u.User != nil || strings.ContainsAny(endpoint, "?#") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || u.Path != "" || u.RawPath != "" || u.Opaque != "" {
		return nil, errors.New("invalid local API endpoint")
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return nil, errors.New("API endpoint must be numeric loopback")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return nil, errors.New("invalid API endpoint port")
	}
	if len(token) != 64 {
		return nil, errors.New("invalid API credential")
	}
	for _, c := range token {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return nil, errors.New("invalid API credential")
		}
	}
	t := &http.Transport{
		Proxy:                  nil,
		DialContext:            (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ResponseHeaderTimeout:  10 * time.Second,
		MaxResponseHeaderBytes: 16 * 1024,
		MaxConnsPerHost:        16,
		MaxIdleConns:           8,
		MaxIdleConnsPerHost:    4,
		IdleConnTimeout:        30 * time.Second,
	}
	h := &http.Client{Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &Client{base: u, token: token, http: h, transport: t}, nil
}

func (c *Client) CloseIdleConnections() { c.transport.CloseIdleConnections() }

func validID(id string) bool { return idPattern.MatchString(id) }
func validInstance(v apitypes.Instance) bool {
	return validID(v.ID) && v.Port >= 1 && v.Port <= 65535 && v.State != ""
}
func instancePath(id string) (string, error) {
	if !validID(id) {
		return "", errors.New("invalid instance ID")
	}
	return "/api/instances/" + id, nil
}

func (c *Client) request(ctx context.Context, method, path string, body []byte, stream bool, expected int) (*http.Response, error) {
	u := *c.base
	u.Path = path
	r, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if stream {
		r.Close = true
	}
	response, err := c.http.Do(r)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("API network failure; operation outcome may be unknown")
	}
	if response.StatusCode != expected {
		response.Body.Close()
		return nil, &StatusError{Method: method, Code: response.StatusCode}
	}
	return response, nil
}

func readBounded(body io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("API response too large")
	}
	return b, nil
}
func (c *Client) jsonRequest(ctx context.Context, method, path string, request any, result any) error {
	ctx, cancel := context.WithTimeout(ctx, restTimeout)
	defer cancel()
	var body []byte
	if request != nil {
		var err error
		body, err = json.Marshal(request)
		if err != nil {
			return err
		}
		if len(body) > 16*1024 {
			return errors.New("API request too large")
		}
	}
	expected := 200
	if method == "DELETE" {
		expected = 204
	} else if method == "POST" && path == "/api/instances" {
		expected = 201
	}
	response, err := c.request(ctx, method, path, body, false, expected)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if result == nil {
		_, err = readBounded(response.Body, maxRESTBody)
		return err
	}
	b, err := readBounded(response.Body, maxRESTBody)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, result); err != nil {
		return errors.New("invalid API response")
	}
	return nil
}

func (c *Client) List(ctx context.Context) ([]apitypes.Instance, error) {
	var v []apitypes.Instance
	err := c.jsonRequest(ctx, "GET", "/api/instances", nil, &v)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, errors.New("invalid API instance list")
	}
	if len(v) > 32 {
		return nil, errors.New("too many API instances")
	}
	seen := make(map[string]bool, len(v))
	for _, item := range v {
		if !validInstance(item) || seen[item.ID] {
			return nil, errors.New("invalid API instance list")
		}
		seen[item.ID] = true
	}
	return v, nil
}
func (c *Client) Get(ctx context.Context, id string) (apitypes.Instance, error) {
	var v apitypes.Instance
	path, err := instancePath(id)
	if err != nil {
		return v, err
	}
	err = c.jsonRequest(ctx, "GET", path, nil, &v)
	if err == nil && (!validInstance(v) || v.ID != id) {
		return apitypes.Instance{}, errors.New("invalid API instance")
	}
	return v, err
}
func (c *Client) Register(ctx context.Context, d apitypes.Definition) (apitypes.Instance, error) {
	var v apitypes.Instance
	if !validID(d.ID) {
		return v, errors.New("invalid instance ID")
	}
	if len(d.Executable) > 4096 || len(d.WorkDir) > 4096 || len(d.Args) > 32 {
		return v, errors.New("API definition exceeds limit")
	}
	total := 0
	for _, a := range d.Args {
		if len(a) > 256 {
			return v, errors.New("API definition exceeds limit")
		}
		total += len(a)
	}
	if total > 4096 {
		return v, errors.New("API definition exceeds limit")
	}
	err := c.jsonRequest(ctx, "POST", "/api/instances", d, &v)
	if err == nil && (!validInstance(v) || v.ID != d.ID) {
		return apitypes.Instance{}, errors.New("invalid API instance")
	}
	return v, err
}
func (c *Client) Remove(ctx context.Context, id string) error {
	path, err := instancePath(id)
	if err != nil {
		return err
	}
	return c.jsonRequest(ctx, "DELETE", path, nil, nil)
}
func (c *Client) action(ctx context.Context, id, action string) (apitypes.Instance, error) {
	var v apitypes.Instance
	path, err := instancePath(id)
	if err != nil {
		return v, err
	}
	err = c.jsonRequest(ctx, "POST", path+"/"+action, nil, &v)
	if err == nil && (!validInstance(v) || v.ID != id) {
		return apitypes.Instance{}, errors.New("invalid API instance")
	}
	return v, err
}
func (c *Client) Start(ctx context.Context, id string) (apitypes.Instance, error) {
	return c.action(ctx, id, "start")
}
func (c *Client) Stop(ctx context.Context, id string) (apitypes.Instance, error) {
	return c.action(ctx, id, "stop")
}
func (c *Client) Restart(ctx context.Context, id string) (apitypes.Instance, error) {
	return c.action(ctx, id, "restart")
}

type LogSnapshot struct {
	Bytes      []byte
	Start, End uint64
}

func (c *Client) Logs(ctx context.Context, id string) (LogSnapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, restTimeout)
	defer cancel()
	var out LogSnapshot
	path, err := instancePath(id)
	if err != nil {
		return out, err
	}
	r, err := c.request(ctx, "GET", path+"/logs", nil, false, 200)
	if err != nil {
		return out, err
	}
	defer r.Body.Close()
	b, err := readBounded(r.Body, 64*1024)
	if err != nil {
		return out, err
	}
	start, e1 := strconv.ParseUint(r.Header.Get("X-Log-Start"), 10, 64)
	end, e2 := strconv.ParseUint(r.Header.Get("X-Log-End"), 10, 64)
	if e1 != nil || e2 != nil || end < start || end-start != uint64(len(b)) {
		return out, errors.New("invalid log offsets")
	}
	return LogSnapshot{Bytes: b, Start: start, End: end}, nil
}

// IsStatus tests a status without exposing response bytes or the bearer token.
func IsStatus(err error, code int) bool {
	var s *StatusError
	return errors.As(err, &s) && s.Code == code
}

// URL returns the validated local endpoint without the credential.
func (c *Client) URL() string { return strings.TrimSuffix(c.base.String(), "/") }
