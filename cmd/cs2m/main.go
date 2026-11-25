package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/atno11/cs2-server-manager/internal/manager"
)

func main() {
	if os.Geteuid() == 0 {
		log.Fatal("run as an unprivileged user")
	}
	dir := os.Getenv("CS2M_DATA")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatal(err)
		}
		dir = filepath.Join(home, ".local", "share", "cs2m")
	}
	m, err := manager.New(dir)
	if err != nil {
		log.Fatal(err)
	}
	addr := os.Getenv("CS2M_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		log.Fatal("CS2M_ADDR must use a loopback IP address")
	}
	srv := &http.Server{Addr: addr, Handler: handler(m), ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		m.Close()
		log.Fatal(err)
	}
	log.Printf("listening on %s", listener.Addr())
	if err := serve(ctx, srv, listener, m); err != nil {
		log.Fatal(err)
	}

}
func serve(ctx context.Context, srv *http.Server, listener net.Listener, m *manager.Manager) error {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		shutdownCtx, finish := context.WithTimeout(context.Background(), 15*time.Second)
		defer finish()
		_ = srv.Shutdown(shutdownCtx)
	}()
	err := srv.Serve(listener)
	cancel()
	<-done
	m.Close()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func handler(m *manager.Manager) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = page.Execute(w, m.List())
	})
	mux.HandleFunc("GET /api/instances", func(w http.ResponseWriter, r *http.Request) { respond(w, http.StatusOK, m.List()) })
	mux.HandleFunc("POST /api/instances", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		var d manager.Definition
		if err := dec.Decode(&d); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if err := m.Register(d); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		respond(w, 201, d)
	})
	instanceHandler := func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var err error
		switch {
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/logs"):
			var s string
			s, err = m.Logs(id)
			if err == nil {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				_, _ = fmt.Fprint(w, s)
				return
			}
		case r.Method == "GET":
			var v manager.Instance
			v, err = m.Get(id)
			if err == nil {
				respond(w, 200, v)
				return
			}
		case r.Method == "DELETE":
			err = m.Remove(id)
			if err == nil {
				w.WriteHeader(204)
				return
			}
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/start"):
			err = m.Start(id)
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/stop"):
			err = m.Stop(id)
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/restart"):
			err = m.Restart(id)
		default:
			http.NotFound(w, r)
			return
		}
		if err != nil {
			code := 409
			if errors.Is(err, os.ErrNotExist) {
				code = 404
			}
			http.Error(w, err.Error(), code)
			return
		}
		v, _ := m.Get(id)
		respond(w, 200, v)
	}
	for _, pattern := range []string{"/api/instances/{id}", "/api/instances/{id}/start", "/api/instances/{id}/stop", "/api/instances/{id}/restart", "/api/instances/{id}/logs"} {
		mux.HandleFunc(pattern, instanceHandler)
	}
	return mux
}
func respond(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

var page = template.Must(template.New("page").Parse(`<!doctype html><html><head><meta charset="utf-8"><title>CS2 Server Manager</title><style>body{font:16px system-ui;max-width:900px;margin:2rem auto;padding:0 1rem}table{border-collapse:collapse;width:100%}td,th{padding:.6rem;border-bottom:1px solid #ccc;text-align:left}button{margin:.15rem}</style></head><body><h1>CS2 Server Manager</h1><p>Local instance management</p><table><tr><th>ID</th><th>Port</th><th>State</th><th>Operations</th></tr>{{range .}}<tr><td>{{.ID}}</td><td>{{.Port}}</td><td>{{.State}}</td><td><button onclick="op('{{.ID}}','start')">Start</button><button onclick="op('{{.ID}}','stop')">Stop</button><button onclick="op('{{.ID}}','restart')">Restart</button><button onclick="logs('{{.ID}}')">Logs</button></td></tr>{{end}}</table><h2>Register</h2><form id="form"><input name="id" placeholder="ID" required><input name="executable" placeholder="Absolute executable path" required><input name="work_dir" placeholder="Absolute work directory" required><input name="port" type="number" min="1" max="65535" placeholder="Port" required><button>Register</button></form><pre id="output"></pre><script>async function op(id,action){let r=await fetch('/api/instances/'+encodeURIComponent(id)+'/'+action,{method:'POST'});if(!r.ok)alert(await r.text());location.reload()}async function logs(id){let r=await fetch('/api/instances/'+encodeURIComponent(id)+'/logs');document.getElementById('output').textContent=await r.text()}document.getElementById('form').onsubmit=async e=>{e.preventDefault();let d=Object.fromEntries(new FormData(e.target));d.port=Number(d.port);d.args=[];let r=await fetch('/api/instances',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(d)});if(!r.ok)alert(await r.text());else location.reload()}</script></body></html>`))
