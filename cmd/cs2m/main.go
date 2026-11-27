package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/atno11/cs2-server-manager/internal/api"
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
	policy.InternalSecrets = []string{token}
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
	srv := &http.Server{Addr: addr, Handler: api.Handler(m, token, addr, api.Options{ValidationUI: validationUIEnabled(), HTML: htmlPage, JavaScript: browserScript}), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8 * 1024}
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
func validationUIEnabled() bool { return os.Getenv("CS2M_VALIDATION_UI") == "1" }
func serve(ctx context.Context, srv *http.Server, listener net.Listener, m *manager.Manager) error {
	return serveWithTimeout(ctx, srv, listener, m, 15*time.Second)
}
func serveWithTimeout(ctx context.Context, srv *http.Server, listener net.Listener, m *manager.Manager, timeout time.Duration) error {
	ctx, cancel := context.WithCancel(ctx)
	shutdownResult := make(chan error, 1)
	go func() {
		<-ctx.Done()
		m.StopEvents()
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
