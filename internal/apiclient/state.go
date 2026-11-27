package apiclient

import (
	"errors"
	"slices"
	"sync"

	"github.com/atno11/cs2-server-manager/internal/apitypes"
)

var ErrLogGap = errors.New("log byte gap; fetch retained logs again")
var ErrStaleLog = errors.New("stale log snapshot")
var ErrLogBacklog = errors.New("log recovery backlog exceeded")

func validLogEvent(e apitypes.LogEvent) bool {
	return validID(e.ID) && e.End >= e.Start && e.End-e.Start <= 2048 && uint64(len(e.Bytes)) == e.End-e.Start
}

// LogBuffer retains at most the most recent 64 KiB of sanitized bytes.
type LogBuffer struct {
	ID         string
	Start, End uint64
	Bytes      []byte
}

func NewLogBuffer(id string, s LogSnapshot) (LogBuffer, error) {
	if !validID(id) || len(s.Bytes) > 64*1024 || s.End < s.Start || s.End-s.Start != uint64(len(s.Bytes)) {
		return LogBuffer{}, errors.New("invalid log snapshot")
	}
	return LogBuffer{ID: id, Start: s.Start, End: s.End, Bytes: append([]byte(nil), s.Bytes...)}, nil
}
func (b *LogBuffer) Apply(e apitypes.LogEvent) error {
	if e.ID != b.ID || !validLogEvent(e) {
		return errors.New("invalid log event")
	}
	if e.End <= b.End {
		return nil
	}
	if e.Start > b.End {
		return ErrLogGap
	}
	extra := e.Bytes[b.End-e.Start:]
	b.Bytes = append(b.Bytes, extra...)
	if len(b.Bytes) > 64*1024 {
		b.Bytes = append([]byte(nil), b.Bytes[len(b.Bytes)-64*1024:]...)
	}
	b.End = e.End
	b.Start = b.End - uint64(len(b.Bytes))
	return nil
}

// LogTicket identifies one selected-instance recovery attempt. A newer
// selection, removal, registration or stream snapshot invalidates old tickets.
type LogTicket struct {
	id         string
	generation uint64
}

type State struct {
	mu         sync.Mutex
	instances  map[string]apitypes.Instance
	selected   string
	generation uint64
	loading    bool
	pending    []apitypes.LogEvent
	log        LogBuffer
}

func NewState() *State       { return &State{instances: make(map[string]apitypes.Instance)} }
func (s *State) invalidate() { s.generation++; s.loading = false; s.pending = nil; s.log = LogBuffer{} }

// Apply consumes validated Stream events in sequence. The caller must reconnect
// after any stream error; a new snapshot replaces the whole public view.
func (s *State) Apply(e Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch e.Type {
	case "snapshot":
		if e.Snapshot == nil || len(e.Snapshot.Instances) > 32 {
			return ErrInvalidEvent
		}
		next := make(map[string]apitypes.Instance, len(e.Snapshot.Instances))
		for _, v := range e.Snapshot.Instances {
			if !validInstance(v) || len(next) >= 32 {
				return ErrInvalidEvent
			}
			if _, exists := next[v.ID]; exists {
				return ErrInvalidEvent
			}
			next[v.ID] = v
		}
		s.instances = next
		s.invalidate()
		if _, ok := next[s.selected]; !ok {
			s.selected = ""
		}
	case "instance.registered":
		if e.Instance == nil || !validInstance(*e.Instance) {
			return ErrInvalidEvent
		}
		if _, ok := s.instances[e.Instance.ID]; !ok && len(s.instances) >= 32 {
			return ErrInvalidEvent
		}
		if e.Instance.ID == s.selected {
			s.invalidate()
		}
		s.instances[e.Instance.ID] = *e.Instance
	case "instance.state":
		if e.Instance == nil || !validInstance(*e.Instance) {
			return ErrInvalidEvent
		}
		if _, ok := s.instances[e.Instance.ID]; !ok && len(s.instances) >= 32 {
			return ErrInvalidEvent
		}
		s.instances[e.Instance.ID] = *e.Instance
	case "instance.removed":
		if !validID(e.RemovedID) {
			return ErrInvalidEvent
		}
		delete(s.instances, e.RemovedID)
		if e.RemovedID == s.selected {
			s.invalidate()
			s.selected = ""
		}
	case "instance.log":
		if e.Log == nil || !validLogEvent(*e.Log) {
			return ErrInvalidEvent
		}
		if e.Log.ID != s.selected {
			return nil
		}
		if s.loading {
			if len(s.pending) >= 128 {
				return ErrLogBacklog
			}
			copyEvent := *e.Log
			copyEvent.Bytes = append([]byte(nil), e.Log.Bytes...)
			s.pending = append(s.pending, copyEvent)
			return nil
		}
		if s.log.ID == "" {
			return ErrStaleLog
		}
		return s.log.Apply(*e.Log)
	}
	return nil
}

func (s *State) Instances() []apitypes.Instance {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]apitypes.Instance, 0, len(s.instances))
	for _, v := range s.instances {
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b apitypes.Instance) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return out
}

// BeginLog starts one retained-log fetch for the selected instance. Event
// bytes arriving during the fetch are buffered up to 128 events.
func (s *State) BeginLog(id string) (LogTicket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.instances[id]; !ok {
		return LogTicket{}, errors.New("unknown instance")
	}
	s.invalidate()
	s.selected = id
	s.loading = true
	return LogTicket{id: id, generation: s.generation}, nil
}

// CompleteLog rejects responses from older selections or incarnations and
// applies all buffered incremental events by byte offset.
func (s *State) CompleteLog(ticket LogTicket, snapshot LogSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ticket.id != s.selected || ticket.generation != s.generation || !s.loading {
		return ErrStaleLog
	}
	log, err := NewLogBuffer(ticket.id, snapshot)
	if err != nil {
		return err
	}
	for _, event := range s.pending {
		if err = log.Apply(event); err != nil {
			return err
		}
	}
	s.log = log
	s.loading = false
	s.pending = nil
	return nil
}
func (s *State) Log() (LogBuffer, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.log.ID == "" {
		return LogBuffer{}, false
	}
	out := s.log
	out.Bytes = append([]byte(nil), out.Bytes...)
	return out, true
}
