package apiclient

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/atno11/cs2-server-manager/internal/api"
	"github.com/atno11/cs2-server-manager/internal/apitypes"
	"github.com/atno11/cs2-server-manager/internal/manager"
)

func TestHeadlessSimulatedWorkflow(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "simulator.sh")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nprintf 'alpha S3CRET omega "+strings.Repeat("x", 100)+"\\n'\nwhile :; do sleep 1; done\n"), 0700); err != nil {
		t.Fatal(err)
	}
	m, err := manager.New(filepath.Join(root, "data"), manager.Policy{Executables: []string{exe}, ExecutableRoots: []string{root}, WorkRoots: []string{root}, KnownSecrets: []string{"S3CRET"}, InternalSecrets: []string{testToken}})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	ts := httptest.NewUnstartedServer(nil)
	ts.Config.Handler = api.Handler(m, testToken, ts.Listener.Addr().String(), api.Options{})
	ts.Start()
	defer ts.Close()
	c, err := New(ts.URL, testToken)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	stream, err := c.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	first, err := stream.Next()
	if err != nil || first.Type != "snapshot" || first.Snapshot == nil || len(first.Snapshot.Instances) != 0 {
		t.Fatalf("initial snapshot: %+v %v", first, err)
	}
	state := NewState()
	if err = state.Apply(first); err != nil {
		t.Fatal(err)
	}
	d := apitypes.Definition{ID: "one", Executable: exe, WorkDir: root, Port: 27015}
	created, err := c.Register(ctx, d)
	if err != nil || created.ID != "one" {
		t.Fatalf("register: %+v %v", created, err)
	}
	e, err := stream.Next()
	if err != nil || e.Type != "instance.registered" || e.Instance == nil || e.Instance.ID != "one" {
		t.Fatalf("registered event: %+v %v", e, err)
	}
	if err = state.Apply(e); err != nil {
		t.Fatal(err)
	}
	list, err := c.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %+v %v", list, err)
	}
	if _, err = c.Get(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Start(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	running, redacted := false, false
	for !running || !redacted {
		e, err = stream.Next()
		if err != nil {
			t.Fatal(err)
		}
		if e.Type == "instance.state" && e.Instance != nil && e.Instance.State == "running" {
			running = true
		}
		if e.Type == "instance.log" && e.Log != nil && strings.Contains(string(e.Log.Bytes), "[REDACTED]") {
			redacted = true
		}
		if err = state.Apply(e); err != nil && err != ErrStaleLog {
			t.Fatal(err)
		}
	}
	logs, err := c.Logs(ctx, "one")
	if err != nil || strings.Contains(string(logs.Bytes), "S3CRET") || !strings.Contains(string(logs.Bytes), "[REDACTED]") {
		t.Fatalf("sanitized log: %q %v", logs.Bytes, err)
	}
	if _, err = c.Stop(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	for {
		e, err = stream.Next()
		if err != nil {
			t.Fatal(err)
		}
		if e.Type == "instance.state" && e.Instance != nil && e.Instance.State == "stopped" {
			break
		}
	}
	if err = c.Remove(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	for {
		e, err = stream.Next()
		if err != nil {
			t.Fatal(err)
		}
		if e.Type == "instance.removed" && e.RemovedID == "one" {
			break
		}
	}
	if err = stream.Close(); err != nil {
		t.Fatal(err)
	}
	m.StopEvents()
	ts.Close()
	if got := m.EventSubscriberCount(); got != 0 {
		t.Fatalf("stream resources retained: %d", got)
	}
}
