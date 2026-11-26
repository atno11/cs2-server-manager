package main

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/atno11/cs2-server-manager/internal/events"
	"github.com/atno11/cs2-server-manager/internal/manager"
)

const streamWriteTimeout = 5 * time.Second

func streamEvents(w http.ResponseWriter, r *http.Request, m *manager.Manager, heartbeat, writeTimeout time.Duration) {
	snapshot, sub, err := m.Subscribe()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable)
		return
	}
	defer sub.Cancel()
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	controller := http.NewResponseController(w)
	write := func(frame string) bool {
		_ = controller.SetWriteDeadline(time.Now().Add(writeTimeout))
		if _, err := w.Write([]byte(frame)); err != nil {
			return false
		}
		if err := controller.Flush(); err != nil {
			return false
		}
		_ = controller.SetWriteDeadline(time.Now().Add(heartbeat + writeTimeout))
		return true
	}
	if !write(eventFrame(snapshot)) {
		return
	}
	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-sub.Done():
			return
		default:
		}
		select {
		case <-r.Context().Done():
			return
		case <-sub.Done():
			return
		case event := <-sub.Events():
			select {
			case <-sub.Done():
				return
			default:
			}
			if !write(eventFrame(event)) {
				return
			}
		case <-ticker.C:
			if !write(": heartbeat\n\n") {
				return
			}
		}
	}
}

func eventFrame(e events.Event) string {
	var b strings.Builder
	b.Grow(len(e.Data) + 96)
	b.WriteString("id: ")
	b.WriteString(strconv.FormatUint(e.Sequence, 10))
	b.WriteString("\nevent: ")
	b.WriteString(e.Type)
	b.WriteString("\ndata: ")
	b.WriteString(e.Data)
	b.WriteString("\n\n")
	return b.String()
}
