package apiclient

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/atno11/cs2-server-manager/internal/apitypes"
)

const maxSSEFrame = 20 * 1024
const streamIdleTimeout = 35 * time.Second

var ErrEventGap = errors.New("event sequence gap; reconnect for a fresh snapshot")
var ErrStreamInterrupted = errors.New("event stream interrupted; reconnect for a fresh snapshot")
var ErrInvalidEvent = errors.New("invalid event stream")

type Event struct {
	Sequence  uint64
	Type      string
	Snapshot  *apitypes.Snapshot
	Instance  *apitypes.Instance
	RemovedID string
	Log       *apitypes.LogEvent
	Raw       json.RawMessage // unknown future event types only
}

// Stream is a pull-based reader. A fresh stream always begins with a snapshot.
// The caller owns Close and reconnects explicitly after any read error.
type Stream struct {
	body        io.ReadCloser
	reader      *bufio.Reader
	conn        net.Conn
	last        uint64
	initialized bool
	closed      atomic.Bool
	closeOnce   sync.Once
	closeErr    error
}

func (c *Client) Subscribe(ctx context.Context) (*Stream, error) {
	u := *c.base
	u.Path = "/api/events"
	r, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	r.Header.Set("Authorization", "Bearer "+c.token)
	r.Header.Set("Cache-Control", "no-store")
	r.Close = true
	var mu sync.Mutex
	var conn net.Conn
	var deadlineErr error
	trace := &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) {
		mu.Lock()
		defer mu.Unlock()
		conn = info.Conn
		deadlineErr = conn.SetReadDeadline(time.Now().Add(streamIdleTimeout))
	}}
	r = r.WithContext(httptrace.WithClientTrace(r.Context(), trace))
	response, err := c.http.Do(r)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrStreamInterrupted
	}
	if response.StatusCode != 200 {
		response.Body.Close()
		return nil, &StatusError{Method: "GET", Code: response.StatusCode}
	}
	mu.Lock()
	streamConn, setupErr := conn, deadlineErr
	mu.Unlock()
	media, _, mediaErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if streamConn == nil || setupErr != nil || mediaErr != nil || media != "text/event-stream" {
		response.Body.Close()
		return nil, ErrInvalidEvent
	}
	return &Stream{body: response.Body, reader: bufio.NewReaderSize(response.Body, 4096), conn: streamConn}, nil
}

func (s *Stream) Close() error {
	s.closeOnce.Do(func() { s.closed.Store(true); s.closeErr = s.body.Close() })
	return s.closeErr
}

func (s *Stream) line() (string, error) {
	var b []byte
	for {
		c, err := s.reader.ReadByte()
		if err != nil {
			if err == io.EOF {
				return "", ErrStreamInterrupted
			}
			return "", err
		}
		if c == '\n' {
			if len(b) > 0 && b[len(b)-1] == '\r' {
				b = b[:len(b)-1]
			}
			if !utf8.Valid(b) {
				return "", ErrInvalidEvent
			}
			return string(b), nil
		}
		if len(b) >= maxSSEFrame {
			return "", ErrInvalidEvent
		}
		b = append(b, c)
	}
}

func (s *Stream) Next() (Event, error) {
	if s.closed.Load() {
		return Event{}, ErrStreamInterrupted
	}
	var kind, id string
	var data []byte
	frameBytes, dataLines := 0, 0
	for {
		line, err := s.line()
		if err != nil {
			return Event{}, err
		}
		frameBytes += len(line) + 1
		if frameBytes > maxSSEFrame {
			return Event{}, ErrInvalidEvent
		}
		if line == "" {
			if err = s.conn.SetReadDeadline(time.Now().Add(streamIdleTimeout)); err != nil {
				return Event{}, ErrStreamInterrupted
			}
			if dataLines == 0 {
				frameBytes = 0
				kind = ""
				id = ""
				continue
			}
			return s.decode(kind, id, data)
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, has := strings.Cut(line, ":")
		if !has {
			value = ""
		}
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			kind = value
		case "id":
			if strings.ContainsRune(value, 0) {
				return Event{}, ErrInvalidEvent
			}
			id = value
		case "data":
			if dataLines > 0 {
				data = append(data, '\n')
			}
			data = append(data, value...)
			dataLines++
		}
	}
}

func (s *Stream) decode(kind, id string, data []byte) (Event, error) {
	if id == "" || len(id) > 20 {
		return Event{}, ErrInvalidEvent
	}
	for _, c := range id {
		if c < '0' || c > '9' {
			return Event{}, ErrInvalidEvent
		}
	}
	seq, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return Event{}, ErrInvalidEvent
	}
	if !json.Valid(data) {
		return Event{}, ErrInvalidEvent
	}
	if !s.initialized {
		if kind != "snapshot" || len(data) > 16*1024 {
			return Event{}, ErrInvalidEvent
		}
	} else if seq != s.last+1 || s.last == ^uint64(0) {
		return Event{}, ErrEventGap
	}
	e := Event{Sequence: seq, Type: kind}
	switch kind {
	case "snapshot":
		if s.initialized {
			return Event{}, ErrInvalidEvent
		}
		var value apitypes.Snapshot
		if json.Unmarshal(data, &value) != nil || value.Instances == nil || len(value.Instances) > 32 {
			return Event{}, ErrInvalidEvent
		}
		seen := make(map[string]bool, len(value.Instances))
		for _, item := range value.Instances {
			if !validInstance(item) || seen[item.ID] {
				return Event{}, ErrInvalidEvent
			}
			seen[item.ID] = true
		}
		e.Snapshot = &value
	case "instance.registered", "instance.state":
		if len(data) > 4096 {
			return Event{}, ErrInvalidEvent
		}
		var value apitypes.InstanceChange
		if json.Unmarshal(data, &value) != nil || !validInstance(value.Instance) {
			return Event{}, ErrInvalidEvent
		}
		e.Instance = &value.Instance
	case "instance.removed":
		if len(data) > 4096 {
			return Event{}, ErrInvalidEvent
		}
		var value apitypes.Removed
		if json.Unmarshal(data, &value) != nil || value.ID == "" {
			return Event{}, ErrInvalidEvent
		}
		e.RemovedID = value.ID
	case "instance.log":
		if len(data) > 4096 {
			return Event{}, ErrInvalidEvent
		}
		var value apitypes.LogEvent
		if json.Unmarshal(data, &value) != nil || value.ID == "" || value.End < value.Start || value.End-value.Start > 2048 || uint64(len(value.Bytes)) != value.End-value.Start {
			return Event{}, ErrInvalidEvent
		}
		e.Log = &value
	default:
		if len(data) > 4096 {
			return Event{}, ErrInvalidEvent
		}
		e.Raw = append(json.RawMessage(nil), data...)
	}
	s.initialized = true
	s.last = seq
	return e, nil
}
