package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"cserver/internal/service"
)

func TestHealthEndpoint(t *testing.T) {
	request := httptest.NewRequest(
		http.MethodGet,
		"/healthz",
		nil,
	)

	response := httptest.NewRecorder()

	NewHandler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf(
			"expected status 200, got %d",
			response.Code,
		)
	}

	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("unexpected content type: %q", got)
	}

	var body struct {
		Status string `json:"status"`
	}

	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body.Status != "ok" {
		t.Fatalf("expected status ok, got %q", body.Status)
	}
}

func TestInfoEndpoint(t *testing.T) {
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/info",
		nil,
	)

	response := httptest.NewRecorder()

	NewHandler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf(
			"expected status 200, got %d",
			response.Code,
		)
	}

	var info service.ApplicationInfo

	if err := json.Unmarshal(response.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	expected := service.GetApplicationInfo()

	if info != expected {
		t.Fatalf(
			"expected %+v, got %+v",
			expected,
			info,
		)
	}
}

func TestUnknownEndpoint(t *testing.T) {
	request := httptest.NewRequest(
		http.MethodGet,
		"/unknown",
		nil,
	)

	response := httptest.NewRecorder()

	NewHandler().ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf(
			"expected status 404, got %d",
			response.Code,
		)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/info",
		nil,
	)

	response := httptest.NewRecorder()

	NewHandler().ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf(
			"expected status 405, got %d",
			response.Code,
		)
	}
}
