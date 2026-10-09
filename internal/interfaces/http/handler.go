package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"cserver/internal/core"
	"cserver/internal/service"
)

type serversResponse struct {
	Servers []core.DiscoveredServer `json:"servers"`
	Count   int                     `json:"count"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// NewHandler preserves the Stage 1 handler API.
func NewHandler() http.Handler {
	return NewHandlerWithDiscovery(nil)
}

// NewHandlerWithDiscovery creates a router with a shared
// read-only server discovery service.
func NewHandlerWithDiscovery(
	discovery *service.ServerDiscoveryService,
) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("GET /api/v1/info", handleInfo)
	mux.HandleFunc(
		"GET /api/v1/servers",
		handleServers(discovery),
	)

	return mux
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func handleInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(
		w,
		http.StatusOK,
		service.GetApplicationInfo(),
	)
}

func handleServers(
	discovery *service.ServerDiscoveryService,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Discovery can reveal local filesystem paths.
		// Do not allow intermediary caching.
		w.Header().Set("Cache-Control", "no-store")

		if discovery == nil {
			writeJSON(
				w,
				http.StatusServiceUnavailable,
				errorResponse{
					Error: "discovery_unavailable",
				},
			)
			return
		}

		servers, err := discovery.List(r.Context())
		if err != nil {
			code := "discovery_unavailable"

			if errors.Is(
				err,
				service.ErrDiscoveryRootNotConfigured,
			) {
				code = "discovery_not_configured"
			}

			// Do not expose internal filesystem errors,
			// configuration values or secrets over HTTP.
			writeJSON(
				w,
				http.StatusServiceUnavailable,
				errorResponse{Error: code},
			)
			return
		}

		if servers == nil {
			servers = []core.DiscoveredServer{}
		}

		writeJSON(
			w,
			http.StatusOK,
			serversResponse{
				Servers: servers,
				Count:   len(servers),
			},
		)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(value)
}
