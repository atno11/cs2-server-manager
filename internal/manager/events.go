package manager

import (
	"encoding/json"
	"errors"
	"os"
	"slices"

	"github.com/atno11/cs2-server-manager/internal/events"
)

const maxSnapshotPayload = 16 * 1024
const maxLogEventBytes = 2048

// EventInstance contains the same public fields and generic indicators as REST.
type EventInstance struct {
	ID              string `json:"id"`
	Port            int    `json:"port"`
	State           string `json:"state"`
	Error           string `json:"error,omitempty"`
	LastTermination string `json:"last_termination,omitempty"`
}

func eventInstance(e *entry) EventInstance {
	v := EventInstance{ID: e.def.ID, Port: e.def.Port, State: e.state}
	if e.err != "" {
		v.Error = "operation failed"
	}
	if e.lastTermination != "" {
		v.LastTermination = "previous termination failed"
	}
	return v
}

func (m *Manager) newEntry(d Definition, workInfo os.FileInfo) *entry {
	r := newRing(m.policy.secrets...)
	r.onOutput = func(start uint64, data []byte) { m.publishLog(d.ID, start, data) }
	return &entry{def: copyDefinition(d), state: "stopped", log: r, workInfo: workInfo}
}

func (m *Manager) publishInstanceLocked(kind string, e *entry) {
	if m.events == nil {
		return
	}
	data, _ := json.Marshal(struct {
		Instance EventInstance `json:"instance"`
	}{eventInstance(e)})
	_, _ = m.events.Publish(kind, data)
}

func (m *Manager) publishRemovedLocked(id string) {
	if m.events == nil {
		return
	}
	data, _ := json.Marshal(struct {
		ID string `json:"id"`
	}{id})
	_, _ = m.events.Publish("instance.removed", data)
}

func (m *Manager) publishLog(id string, start uint64, data []byte) {
	if m.events == nil {
		return
	}
	for len(data) > 0 {
		n := len(data)
		if n > maxLogEventBytes {
			n = maxLogEventBytes
		}
		chunk := data[:n]
		payload, _ := json.Marshal(struct {
			ID    string `json:"id"`
			Start uint64 `json:"start"`
			End   uint64 `json:"end"`
			Bytes []byte `json:"bytes"`
		}{id, start, start + uint64(n), chunk})
		_, _ = m.events.Publish("instance.log", payload)
		start += uint64(n)
		data = data[n:]
	}
}

// Subscribe returns a snapshot at a sequence boundary. Manager state changes
// publish under m.mu, so every later change is queued after that boundary.
func (m *Manager) Subscribe() (events.Event, *events.Subscription, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return events.Event{}, nil, errors.New("manager closed")
	}
	if m.events == nil {
		m.mu.Unlock()
		return events.Event{}, nil, events.ErrClosed
	}
	sub, boundary, err := m.events.Subscribe()
	if err != nil {
		m.mu.Unlock()
		return events.Event{}, nil, err
	}
	instances := make([]EventInstance, 0, len(m.entries))
	for _, e := range m.entries {
		instances = append(instances, eventInstance(e))
	}
	m.mu.Unlock()
	slices.SortFunc(instances, func(a, b EventInstance) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	data, err := json.Marshal(struct {
		Instances []EventInstance `json:"instances"`
	}{instances})
	if err != nil || len(data) > maxSnapshotPayload {
		sub.Cancel()
		return events.Event{}, nil, errors.New("snapshot exceeds limit")
	}
	return events.Event{Sequence: boundary, Type: "snapshot", Data: string(data)}, sub, nil
}

// StopEvents makes active streams exit before HTTP waits for its handlers.
func (m *Manager) StopEvents() {
	if m.events != nil {
		m.events.Close()
	}
}

func (m *Manager) EventSubscriberCount() int {
	if m.events == nil {
		return 0
	}
	return m.events.SubscriberCount()
}
