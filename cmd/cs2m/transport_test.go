package main

import (
	"net/http"
	"testing"
	"time"

	"github.com/atno11/cs2-server-manager/internal/api"
	"github.com/atno11/cs2-server-manager/internal/manager"
)

func TestValidationUIDefaultAndOptIn(t *testing.T) {
	t.Setenv("CS2M_VALIDATION_UI", "")
	if validationUIEnabled() {
		t.Fatal("validation UI enabled by default")
	}
	t.Setenv("CS2M_VALIDATION_UI", "1")
	if !validationUIEnabled() {
		t.Fatal("validation UI opt-in ignored")
	}
	t.Setenv("CS2M_VALIDATION_UI", "true")
	if validationUIEnabled() {
		t.Fatal("unexpected UI setting accepted")
	}
}

// Existing transport regression scenarios retain their fixtures after the move.
func handler(m *manager.Manager, token, authority string) http.Handler {
	return api.Handler(m, token, authority, api.Options{ValidationUI: true, HTML: htmlPage, JavaScript: browserScript})
}
func handlerWithHeartbeat(m *manager.Manager, token, authority string, heartbeat time.Duration) http.Handler {
	return api.Handler(m, token, authority, api.Options{ValidationUI: true, HTML: htmlPage, JavaScript: browserScript, Heartbeat: heartbeat})
}
func handlerWithStreamConfig(m *manager.Manager, token, authority string, heartbeat, writeTimeout time.Duration) http.Handler {
	return api.Handler(m, token, authority, api.Options{ValidationUI: true, HTML: htmlPage, JavaScript: browserScript, Heartbeat: heartbeat, WriteTimeout: writeTimeout})
}
