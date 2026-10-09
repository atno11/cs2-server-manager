package httpapi

import (
	"encoding/json"
	"net/http"

	"cserver/internal/service"
)

// NewHandler creates the HTTP API router.
func NewHandler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("GET /api/v1/info", handleInfo)

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

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(value)
}
