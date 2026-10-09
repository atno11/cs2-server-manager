package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cserver/internal/core"
	"cserver/internal/service"
)

type discoveryReaderStub struct {
	servers []core.DiscoveredServer
	err     error
}

func (f discoveryReaderStub) Discover(
	_ context.Context,
	_ string,
) ([]core.DiscoveredServer, error) {
	return f.servers, f.err
}

func newTestDiscovery(
	t *testing.T,
	root string,
	reader discoveryReaderStub,
) *service.ServerDiscoveryService {
	t.Helper()

	discovery, err := service.NewServerDiscoveryService(
		root,
		reader,
	)
	if err != nil {
		t.Fatal(err)
	}

	return discovery
}

func TestServersEndpoint(t *testing.T) {
	discovery := newTestDiscovery(
		t,
		t.TempDir(),
		discoveryReaderStub{
			servers: []core.DiscoveredServer{
				{
					ID:       "aim",
					Name:     "AIM",
					Status:   core.DiscoveryComplete,
					Evidence: []string{"env", "compose"},
				},
			},
		},
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/servers",
		nil,
	)

	response := httptest.NewRecorder()

	NewHandlerWithDiscovery(discovery).ServeHTTP(
		response,
		request,
	)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}

	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("expected no-store cache policy")
	}

	var body serversResponse

	if err := json.Unmarshal(
		response.Body.Bytes(),
		&body,
	); err != nil {
		t.Fatal(err)
	}

	if body.Count != 1 ||
		len(body.Servers) != 1 ||
		body.Servers[0].ID != "aim" {
		t.Fatalf("unexpected response: %#v", body)
	}
}

func TestServersEndpointEmpty(t *testing.T) {
	discovery := newTestDiscovery(
		t,
		t.TempDir(),
		discoveryReaderStub{},
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/servers",
		nil,
	)

	response := httptest.NewRecorder()

	NewHandlerWithDiscovery(discovery).ServeHTTP(
		response,
		request,
	)

	if response.Code != http.StatusOK ||
		strings.TrimSpace(response.Body.String()) !=
			`{"servers":[],"count":0}` {
		t.Fatalf(
			"unexpected empty response: %d %s",
			response.Code,
			response.Body.String(),
		)
	}
}

func TestServersEndpointNotConfigured(t *testing.T) {
	discovery := newTestDiscovery(
		t,
		"",
		discoveryReaderStub{},
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/servers",
		nil,
	)

	response := httptest.NewRecorder()

	NewHandlerWithDiscovery(discovery).ServeHTTP(
		response,
		request,
	)

	if response.Code != http.StatusServiceUnavailable ||
		!strings.Contains(
			response.Body.String(),
			"discovery_not_configured",
		) {
		t.Fatalf(
			"unexpected response: %d %s",
			response.Code,
			response.Body.String(),
		)
	}
}

func TestServersEndpointSanitizesErrors(t *testing.T) {
	discovery := newTestDiscovery(
		t,
		t.TempDir(),
		discoveryReaderStub{
			err: errors.New("secret diagnostic"),
		},
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/servers",
		nil,
	)

	response := httptest.NewRecorder()

	NewHandlerWithDiscovery(discovery).ServeHTTP(
		response,
		request,
	)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", response.Code)
	}

	if strings.Contains(
		response.Body.String(),
		"secret diagnostic",
	) {
		t.Fatal("HTTP response leaked an internal error")
	}
}

func TestServersEndpointMethodNotAllowed(t *testing.T) {
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/servers",
		nil,
	)

	response := httptest.NewRecorder()

	NewHandler().ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", response.Code)
	}
}

func TestServersEndpointWithoutService(t *testing.T) {
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/servers",
		nil,
	)

	response := httptest.NewRecorder()

	NewHandler().ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", response.Code)
	}
}
