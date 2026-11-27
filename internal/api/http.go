package api

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/atno11/cs2-server-manager/internal/apitypes"
	"github.com/atno11/cs2-server-manager/internal/events"
	"github.com/atno11/cs2-server-manager/internal/manager"
	"github.com/atno11/cs2-server-manager/internal/security"
)

func public(v manager.Instance) apitypes.Instance {
	out := apitypes.Instance{ID: v.ID, Port: v.Port, State: v.State}
	if v.Error != "" {
		out.Error = "operation failed"
	}
	if v.LastTermination != "" {
		out.LastTermination = "previous termination failed"
	}
	return out
}
func publicList(v []manager.Instance) []apitypes.Instance {
	out := make([]apitypes.Instance, 0, len(v))
	for _, item := range v {
		out = append(out, public(item))
	}
	return out
}

type Options struct {
	ValidationUI bool
	HTML         string
	JavaScript   string
	Heartbeat    time.Duration
	WriteTimeout time.Duration
}

func Handler(m *manager.Manager, token, authority string, options Options) http.Handler {
	heartbeat, writeTimeout := options.Heartbeat, options.WriteTimeout
	if heartbeat == 0 {
		heartbeat = 15 * time.Second
	}
	if writeTimeout == 0 {
		writeTimeout = streamWriteTimeout
	}
	mux := http.NewServeMux()
	streamSlots := make(chan struct{}, events.MaxSubscribers)
	if options.ValidationUI {
		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = io.WriteString(w, options.HTML)
		})
		mux.HandleFunc("GET /ui.js", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = io.WriteString(w, options.JavaScript)
		})
	}
	mux.HandleFunc("GET /api/instances", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, publicList(m.List())) })
	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		select {
		case streamSlots <- struct{}{}:
			defer func() { <-streamSlots }()
		default:
			writeError(w, http.StatusServiceUnavailable)
			return
		}
		streamEvents(w, r, m, heartbeat, writeTimeout)
	})
	mux.HandleFunc("POST /api/instances", func(w http.ResponseWriter, r *http.Request) {
		media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" || (params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8")) {
			writeError(w, 415)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			jsonError(w, err)
			return
		}
		var d manager.Definition
		if err := manager.DecodeUniqueJSON(body, &d); err != nil {
			jsonError(w, err)
			return
		}
		if err := m.Register(d); err != nil {
			writeError(w, 400)
			return
		}
		v, _ := m.Get(d.ID)
		respond(w, 201, public(v))
	})
	mux.HandleFunc("GET /api/instances/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, err := m.Get(r.PathValue("id"))
		if err != nil {
			writeError(w, 404)
			return
		}
		respond(w, 200, public(v))
	})
	mux.HandleFunc("DELETE /api/instances/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := m.Remove(r.PathValue("id")); err != nil {
			operationError(w, err)
			return
		}
		w.WriteHeader(204)
	})
	for _, action := range []struct {
		name string
		run  func(string) error
	}{{"start", m.Start}, {"stop", m.Stop}, {"restart", m.Restart}} {
		a := action
		mux.HandleFunc("POST /api/instances/{id}/"+a.name, func(w http.ResponseWriter, r *http.Request) {
			if err := a.run(r.PathValue("id")); err != nil {
				operationError(w, err)
				return
			}
			v, _ := m.Get(r.PathValue("id"))
			respond(w, 200, public(v))
		})
	}
	mux.HandleFunc("GET /api/instances/{id}/logs", func(w http.ResponseWriter, r *http.Request) {
		out, start, end, err := m.LogsSnapshot(r.PathValue("id"))
		if err != nil {
			writeError(w, 404)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Log-Start", strconv.FormatUint(start, 10))
		w.Header().Set("X-Log-End", strconv.FormatUint(end, 10))
		_, _ = io.WriteString(w, out)
	})
	sem := make(chan struct{}, 64)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w = &statusOnlyWriter{ResponseWriter: w}
		if r.Host != authority {
			writeError(w, 421)
			return
		}
		if len(r.Header.Values("Origin")) > 1 || r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+authority {
			writeError(w, 403)
			return
		}
		site := r.Header.Get("Sec-Fetch-Site")
		if site == "cross-site" || site == "same-site" {
			writeError(w, 403)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			values := r.Header.Values("Authorization")
			if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") || !security.EqualToken(strings.TrimPrefix(values[0], "Bearer "), token) {
				w.Header().Set("WWW-Authenticate", "Bearer")
				writeError(w, 401)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
		}
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
		default:
			writeError(w, 503)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// statusOnlyWriter keeps successful responses intact and omits generic error
// bodies, including errors generated by ServeMux for unknown methods or paths.
type statusOnlyWriter struct {
	http.ResponseWriter
	wroteHeader  bool
	suppressBody bool
}

func (w *statusOnlyWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusOnlyWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.suppressBody = code >= 400
	w.ResponseWriter.WriteHeader(code)
}
func (w *statusOnlyWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.suppressBody {
		return len(p), nil
	}
	return w.ResponseWriter.Write(p)
}
func writeError(w http.ResponseWriter, code int) { w.WriteHeader(code) }
func jsonError(w http.ResponseWriter, err error) {
	var max *http.MaxBytesError
	if errors.As(err, &max) {
		writeError(w, 413)
		return
	}
	writeError(w, 400)
}
func operationError(w http.ResponseWriter, err error) {
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, 404)
		return
	}
	writeError(w, 409)
}
func respond(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
