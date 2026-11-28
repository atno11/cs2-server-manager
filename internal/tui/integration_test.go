package tui

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/atno11/cs2-server-manager/internal/api"
	"github.com/atno11/cs2-server-manager/internal/apiclient"
	"github.com/atno11/cs2-server-manager/internal/apitypes"
	"github.com/atno11/cs2-server-manager/internal/manager"
)

func TestRealHTTPModelWorkflow(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "simulator.sh")
	// Each instance uses its own work directory; the simulator emits sanitized
	// identifiable output and exits on TERM.
	script := "#!/bin/sh\ntrap 'exit 0' TERM\nprintf 'simulated-%s S3CRET " + strings.Repeat("x", 100) + "\\n' \"$PWD\"\nwhile :; do sleep 1; done\n"
	if err := os.WriteFile(exe, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	workA, workB := filepath.Join(root, "a"), filepath.Join(root, "b")
	for _, dir := range []string{workA, workB} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	token := strings.Repeat("a", 64)
	managerRef, err := manager.New(filepath.Join(root, "data"), manager.Policy{Executables: []string{exe}, ExecutableRoots: []string{root}, WorkRoots: []string{root}, KnownSecrets: []string{"S3CRET"}, InternalSecrets: []string{token}})
	if err != nil {
		t.Fatal(err)
	}
	defer managerRef.Close()
	srv := httptest.NewUnstartedServer(nil)
	srv.Config.Handler = api.Handler(managerRef, token, srv.Listener.Addr().String(), api.Options{})
	srv.Start()
	defer srv.Close()
	client, err := apiclient.New(srv.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	m := New(ctx, LiveService{Client: client}, client.URL())
	defer m.Close()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	sub := m.subscribe()()
	if v, ok := sub.(subscribedMsg); !ok || v.err != nil {
		t.Fatalf("subscribe: %+v", sub)
	}
	m.Update(sub)
	read := func() apiclient.Event {
		t.Helper()
		e, err := m.stream.Next()
		if err != nil {
			t.Fatal(err)
		}
		m.Update(eventMsg{gen: m.gen, event: e})
		return e
	}
	if e := read(); e.Type != "snapshot" || m.status != connected {
		t.Fatal("initial authoritative snapshot")
	}
	defs := []apitypes.Definition{{ID: "alpha", Executable: exe, WorkDir: workA, Port: 27015}, {ID: "beta", Executable: exe, WorkDir: workB, Port: 27016}}
	for _, d := range defs {
		result := m.mutate("register", d.ID, d)()
		m.Update(result)
		for {
			if e := read(); e.Type == "instance.registered" && e.Instance.ID == d.ID {
				break
			}
		}
	}
	if len(m.instances) != 2 {
		t.Fatal("two instances not registered")
	}
	// Start through the production command and consume authoritative state/logs.
	result := m.mutate("start", "alpha", apitypes.Definition{})()
	m.Update(result)
	running, redacted := false, false
	for !running || !redacted {
		e := read()
		if e.Type == "instance.state" && e.Instance.ID == "alpha" && e.Instance.State == "running" {
			running = true
		}
		if e.Type == "instance.log" && e.Log.ID == "alpha" && strings.Contains(string(e.Log.Bytes), "[REDACTED]") {
			redacted = true
		}
	}
	if !running || !redacted {
		t.Fatal("running/log event missing")
	}
	alphaLog := m.selectID("alpha")
	if alphaLog == nil {
		t.Fatal("selected log fetch missing")
	}
	m.Update(alphaLog())
	if log, ok := m.state.Log(); !ok || log.ID != "alpha" || strings.Contains(string(log.Bytes), "S3CRET") || !strings.Contains(string(log.Bytes), "[REDACTED]") {
		t.Fatal("retained log recovery was not sanitized")
	}
	if cmd := m.selectID("beta"); cmd == nil {
		t.Fatal("selection did not fetch independent logs")
	}
	if log, ok := m.state.Log(); ok && log.ID == "alpha" {
		t.Fatal("old selected log remained visible")
	}
	result = m.mutate("start", "beta", apitypes.Definition{})()
	m.Update(result)
	for {
		e := read()
		if e.Type == "instance.state" && e.Instance.ID == "beta" && e.Instance.State == "running" {
			break
		}
	}
	result = m.mutate("stop", "alpha", apitypes.Definition{})()
	m.Update(result)
	for {
		e := read()
		if e.Type == "instance.state" && e.Instance.ID == "alpha" && e.Instance.State == "stopped" {
			break
		}
	}
	result = m.mutate("restart", "alpha", apitypes.Definition{})()
	m.Update(result)
	for {
		e := read()
		if e.Type == "instance.state" && e.Instance.ID == "alpha" && e.Instance.State == "running" {
			break
		}
	}
	result = m.mutate("stop", "alpha", apitypes.Definition{})()
	m.Update(result)
	for {
		e := read()
		if e.Type == "instance.state" && e.Instance.ID == "alpha" && e.Instance.State == "stopped" {
			break
		}
	}
	result = m.mutate("remove", "alpha", apitypes.Definition{})()
	m.Update(result)
	for {
		e := read()
		if e.Type == "instance.removed" && e.RemovedID == "alpha" {
			break
		}
	}
	if len(m.instances) != 1 || m.instances[0].ID != "beta" {
		t.Fatal("removal did not update authoritative view")
	}
	// A fresh authenticated stream must restore retained logs for the same
	// selected instance before any new incremental event is consumed.
	recoverSelected := func(wantState string) {
		t.Helper()
		m.reconnect(context.Canceled)
		result := m.subscribe()()
		sub, ok := result.(subscribedMsg)
		if !ok || sub.err != nil {
			t.Fatalf("resubscribe: %+v", result)
		}
		m.Update(sub)
		e, err := m.stream.Next()
		if err != nil || e.Type != "snapshot" {
			t.Fatalf("reconnect snapshot: %s %v", e.Type, err)
		}
		_, cmd := m.Update(eventMsg{gen: m.gen, event: e})
		batch, ok := cmd().(tea.BatchMsg)
		if !ok || len(batch) < 2 {
			t.Fatalf("snapshot lacks log fetch before next event read: %d commands", len(batch))
		}
		logResult, ok := batch[0]().(logMsg)
		if !ok || logResult.err != nil {
			t.Fatalf("retained fetch: %+v", logResult)
		}
		m.Update(logResult)
		log, ok := m.state.Log()
		if !ok || log.ID != "beta" || m.instances[0].State != wantState {
			t.Fatalf("reconnected selected log or state missing: log=%+v instances=%+v", log, m.instances)
		}
	}
	recoverSelected("running")
	result = m.mutate("stop", "beta", apitypes.Definition{})()
	m.Update(result)
	for {
		e := read()
		if e.Type == "instance.state" && e.Instance.ID == "beta" && e.Instance.State == "stopped" {
			break
		}
	}
	recoverSelected("stopped")
	result = m.mutate("start", "beta", apitypes.Definition{})()
	m.Update(result)
	for {
		e := read()
		if e.Type == "instance.state" && e.Instance.ID == "beta" && e.Instance.State == "running" {
			break
		}
	}
	m.Close()
	if _, err := client.Get(ctx, "beta"); err != nil {
		t.Fatalf("TUI exit stopped daemon or managed instance: %v", err)
	}
	if got, err := managerRef.Get("beta"); err != nil || got.State != "running" {
		t.Fatalf("TUI exit stopped process: %s", got.State)
	}
}
