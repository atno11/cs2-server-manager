package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cserver/internal/core"
)

func TestServersEndpointInfrastructureContract(t *testing.T) {
	infrastructure := &core.InfrastructureMetadata{
		OverlayFS: &core.OverlayFSMetadata{
			LowerDirectories: []string{"/example/base/data"},
			UpperDirectory:   "/example/overlay/aim/upper",
			WorkDirectory:    "/example/overlay/aim/work",
			MergedDirectory:  "/example/servers/aim/merged",
			Source:           "systemd_unit",
			RuntimeState:     core.RuntimeNotChecked,
		},
		Systemd: &core.SystemdMountMetadata{
			UnitName:     "aim.mount",
			UnitFile:     "/example/units/aim.mount",
			Where:        "/example/servers/aim/merged",
			RuntimeState: core.RuntimeNotChecked,
		},
	}

	discovery := newTestDiscovery(
		t,
		t.TempDir(),
		discoveryReaderStub{
			servers: []core.DiscoveredServer{
				{
					ID:             "aim",
					Name:           "AIM Server",
					Directory:      "/example/servers/aim",
					Status:         core.DiscoveryComplete,
					Evidence:       []string{"env", "compose"},
					Infrastructure: infrastructure,
				},
				{
					ID:        "skills",
					Name:      "Skills Server",
					Directory: "/example/servers/skills",
					Status:    core.DiscoveryIncomplete,
					Evidence:  []string{"env"},
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

	NewHandlerWithDiscovery(discovery).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing no-store cache policy")
	}
	if response.Header().Get("Content-Type") != "application/json" {
		t.Fatal("unexpected content type")
	}

	var body serversResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}

	if body.Count != 2 || len(body.Servers) != 2 {
		t.Fatalf("unexpected inventory: %#v", body)
	}

	got := body.Servers[0].Infrastructure
	if got == nil || got.OverlayFS == nil || got.Systemd == nil {
		t.Fatalf("missing metadata: %#v", got)
	}
	if got.Systemd.RuntimeState != core.RuntimeNotChecked ||
		got.OverlayFS.RuntimeState != core.RuntimeNotChecked {
		t.Fatal("runtime state must remain not_checked")
	}
	if got.OverlayFS.Source != "systemd_unit" ||
		got.Systemd.UnitName != "aim.mount" {
		t.Fatalf("unexpected metadata: %#v", got)
	}
	if body.Servers[1].Infrastructure != nil {
		t.Fatal("unconfigured instance unexpectedly has metadata")
	}

	var raw struct {
		Servers []map[string]json.RawMessage `json:"servers"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}

	if _, present := raw.Servers[0]["infrastructure"]; !present {
		t.Fatal("infrastructure JSON field missing")
	}
	if _, present := raw.Servers[1]["infrastructure"]; present {
		t.Fatal("optional infrastructure field was not omitted")
	}

	// Existing fields and envelope are still available.
	for _, key := range []string{
		"id", "name", "directory", "status", "evidence",
	} {
		if _, present := raw.Servers[0][key]; !present {
			t.Fatalf("existing field %q disappeared", key)
		}
	}

	if strings.Contains(response.Body.String(), "password=") ||
		strings.Contains(response.Body.String(), "API_TOKEN") {
		t.Fatal("HTTP response included raw sensitive configuration")
	}
}

func TestServersEndpointInfrastructureWarningsOnly(t *testing.T) {
	discovery := newTestDiscovery(
		t,
		t.TempDir(),
		discoveryReaderStub{
			servers: []core.DiscoveredServer{
				{
					ID:       "aim",
					Name:     "AIM",
					Evidence: []string{"env"},
					Infrastructure: &core.InfrastructureMetadata{
						Warnings: []core.DiscoveryWarning{
							{
								Code:    "systemd_directory_unavailable",
								Message: "Configured systemd unit directory is unavailable or unsafe",
							},
						},
					},
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
	NewHandlerWithDiscovery(discovery).ServeHTTP(response, request)

	var body serversResponse
	if response.Code != http.StatusOK ||
		json.Unmarshal(response.Body.Bytes(), &body) != nil {
		t.Fatalf(
			"unexpected response: %d %s",
			response.Code,
			response.Body.String(),
		)
	}

	if body.Servers[0].Infrastructure == nil ||
		len(body.Servers[0].Infrastructure.Warnings) != 1 ||
		body.Servers[0].Infrastructure.OverlayFS != nil ||
		body.Servers[0].Infrastructure.Systemd != nil {
		t.Fatalf("unexpected warning-only metadata: %#v", body)
	}
}
