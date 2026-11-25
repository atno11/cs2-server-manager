package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/atno11/cs2-server-manager/internal/manager"
	"github.com/atno11/cs2-server-manager/internal/security"
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
	if err := os.MkdirAll(dir, 0700); err != nil {
		log.Fatal("runtime directory unavailable")
	}
	policy, err := manager.ReadPolicy(filepath.Join(dir, "policy.json"))
	if err != nil {
		log.Fatal("invalid operator policy: ", err)
	}
	token, err := security.LoadOrCreateToken(dir)
	if err != nil {
		log.Fatal("credential bootstrap failed: ", err)
	}
	policy.KnownSecrets = append(policy.KnownSecrets, token)
	m, err := manager.New(dir, policy)
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
	srv := &http.Server{Addr: addr, Handler: handler(m, token, addr), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8 * 1024}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(errors.Join(err, m.Close()))
	}
	log.Printf("listening on %s", listener.Addr())
	if err := serve(ctx, srv, newLimitedListener(listener, 64), m); err != nil {
		log.Fatal(err)
	}

}
func serve(ctx context.Context, srv *http.Server, listener net.Listener, m *manager.Manager) error {
	return serveWithTimeout(ctx, srv, listener, m, 15*time.Second)
}
func serveWithTimeout(ctx context.Context, srv *http.Server, listener net.Listener, m *manager.Manager, timeout time.Duration) error {
	ctx, cancel := context.WithCancel(ctx)
	shutdownResult := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdownCtx, finish := context.WithTimeout(context.Background(), timeout)
		defer finish()
		err := srv.Shutdown(shutdownCtx)
		if err != nil {
			err = errors.Join(err, srv.Close())
		}
		shutdownResult <- err
	}()
	serveErr := srv.Serve(listener)
	cancel()
	shutdownErr := <-shutdownResult
	closeErr := m.Close()
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	return errors.Join(serveErr, shutdownErr, closeErr)
}

type publicInstance struct {
	ID              string `json:"id"`
	Port            int    `json:"port"`
	State           string `json:"state"`
	Error           string `json:"error,omitempty"`
	LastTermination string `json:"last_termination,omitempty"`
}

func public(v manager.Instance) publicInstance {
	out := publicInstance{ID: v.ID, Port: v.Port, State: v.State}
	if v.Error != "" {
		out.Error = "operation failed"
	}
	if v.LastTermination != "" {
		out.LastTermination = "previous termination failed"
	}
	return out
}
func publicList(v []manager.Instance) []publicInstance {
	out := make([]publicInstance, 0, len(v))
	for _, item := range v {
		out = append(out, public(item))
	}
	return out
}
func handler(m *manager.Manager, token, authority string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, page)
	})
	mux.HandleFunc("GET /api/instances", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, publicList(m.List())) })
	mux.HandleFunc("POST /api/instances", func(w http.ResponseWriter, r *http.Request) {
		media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" || (params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8")) {
			http.Error(w, "application/json required", 415)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		var d manager.Definition
		if err := dec.Decode(&d); err != nil {
			jsonError(w, err)
			return
		}
		var trailing any
		if err := dec.Decode(&trailing); err != io.EOF {
			jsonError(w, err)
			return
		}
		if err := m.Register(d); err != nil {
			http.Error(w, "invalid instance definition", 400)
			return
		}
		v, _ := m.Get(d.ID)
		respond(w, 201, public(v))
	})
	mux.HandleFunc("GET /api/instances/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, err := m.Get(r.PathValue("id"))
		if err != nil {
			http.Error(w, "instance not found", 404)
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
		out, err := m.Logs(r.PathValue("id"))
		if err != nil {
			http.Error(w, "instance not found", 404)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, out)
	})
	sem := make(chan struct{}, 64)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != authority {
			http.Error(w, "invalid host", 421)
			return
		}
		if len(r.Header.Values("Origin")) > 1 || r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+authority {
			http.Error(w, "origin denied", 403)
			return
		}
		site := r.Header.Get("Sec-Fetch-Site")
		if site == "cross-site" || site == "same-site" {
			http.Error(w, "origin denied", 403)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			values := r.Header.Values("Authorization")
			if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") || !security.EqualToken(strings.TrimPrefix(values[0], "Bearer "), token) {
				w.Header().Set("WWW-Authenticate", "Bearer")
				http.Error(w, "authentication required", 401)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
		}
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
		default:
			http.Error(w, "server busy", 503)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
func jsonError(w http.ResponseWriter, err error) {
	var max *http.MaxBytesError
	if errors.As(err, &max) {
		http.Error(w, "request body too large", 413)
		return
	}
	http.Error(w, "invalid JSON document", 400)
}
func operationError(w http.ResponseWriter, err error) {
	if errors.Is(err, os.ErrNotExist) {
		http.Error(w, "instance not found", 404)
		return
	}
	http.Error(w, "operation unavailable", 409)
}
func respond(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

const page = `<!doctype html><html><head><meta charset="utf-8"><title>CS2 Server Manager</title><style>body{font:16px system-ui;max-width:900px;margin:2rem auto;padding:0 1rem}table{border-collapse:collapse;width:100%}td,th{padding:.6rem;border-bottom:1px solid #ccc;text-align:left}button{margin:.15rem}</style></head><body><h1>CS2 Server Manager</h1><label>Administrator token <input id="token" type="password" autocomplete="off"></label><button id="connect">Connect</button><table><thead><tr><th>ID</th><th>Port</th><th>State</th><th>Operations</th></tr></thead><tbody id="instances"></tbody></table><h2>Register</h2><form id="form"><input name="id" placeholder="ID" required><input name="executable" placeholder="Trusted executable path" required><input name="work_dir" placeholder="Allowed work directory" required><input name="port" type="number" min="1" max="65535" placeholder="Port" required><button>Register</button></form><pre id="output"></pre><script>
let token='';const output=document.getElementById('output');
async function api(path,options={}){options.headers={...(options.headers||{}),Authorization:'Bearer '+token};let r=await fetch(path,options);if(!r.ok)throw Error('Request failed ('+r.status+')');return r}
async function refresh(){try{let rows=await (await api('/api/instances')).json();let body=document.getElementById('instances');body.replaceChildren();for(let item of rows){let tr=document.createElement('tr');for(let value of [item.id,item.port,item.state]){let td=document.createElement('td');td.textContent=value;tr.append(td)}let td=document.createElement('td');for(let action of ['start','stop','restart','logs']){let b=document.createElement('button');b.textContent=action;b.onclick=async()=>{try{if(action==='logs')output.textContent=await (await api('/api/instances/'+encodeURIComponent(item.id)+'/logs')).text();else{await api('/api/instances/'+encodeURIComponent(item.id)+'/'+action,{method:'POST'});await refresh()}}catch(e){output.textContent=e.message}};td.append(b)}tr.append(td);body.append(tr)}}catch(e){output.textContent=e.message}}
document.getElementById('connect').onclick=()=>{token=document.getElementById('token').value;document.getElementById('token').value='';refresh()};document.getElementById('form').onsubmit=async e=>{e.preventDefault();let d=Object.fromEntries(new FormData(e.target));d.port=Number(d.port);d.args=[];try{await api('/api/instances',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(d)});await refresh()}catch(err){output.textContent=err.message}};
</script></body></html>`
