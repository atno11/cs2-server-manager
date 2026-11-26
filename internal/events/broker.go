// Package events distributes immutable, bounded events to live subscribers.
package events

import (
	"errors"
	"sync"
)

const MaxSubscribers = 8
const QueueSize = 128
const MaxPayload = 4096

var ErrClosed = errors.New("event broker closed")
var ErrFull = errors.New("event subscriber limit reached")
var ErrPayload = errors.New("event payload exceeds limit")

// Event is immutable after publication. Sequence numbers start at one and
// increase for every accepted publication, including when no one subscribes.
type Event struct {
	Sequence uint64
	Type     string
	Data     string
}

type Broker struct {
	mu     sync.Mutex
	closed bool
	seq    uint64
	next   uint64
	subs   map[uint64]*Subscription
}

type Subscription struct {
	broker *Broker
	id     uint64
	queue  chan Event
	done   chan struct{}
}

func New() *Broker { return &Broker{subs: make(map[uint64]*Subscription)} }

// Subscribe atomically registers a consumer and returns the sequence boundary.
// The caller may build a state snapshot while holding its own state lock.
func (b *Broker) Subscribe() (*Subscription, uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, 0, ErrClosed
	}
	if len(b.subs) == MaxSubscribers {
		return nil, 0, ErrFull
	}
	b.next++
	s := &Subscription{broker: b, id: b.next, queue: make(chan Event, QueueSize), done: make(chan struct{})}
	b.subs[s.id] = s
	return s, b.seq, nil
}

func (s *Subscription) Events() <-chan Event  { return s.queue }
func (s *Subscription) Done() <-chan struct{} { return s.done }
func (s *Subscription) Cancel()               { s.broker.remove(s.id) }

func (b *Broker) remove(id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if s := b.subs[id]; s != nil {
		delete(b.subs, id)
		close(s.done)
	}
}

// Publish never waits for a consumer. A full queue disconnects that subscriber;
// a connected subscriber therefore receives every sequence after its boundary.
func (b *Broker) Publish(kind string, data []byte) (uint64, error) {
	if len(data) > MaxPayload {
		return 0, ErrPayload
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return 0, ErrClosed
	}
	if b.seq == ^uint64(0) {
		return 0, ErrClosed
	}
	b.seq++
	e := Event{Sequence: b.seq, Type: kind, Data: string(data)}
	for id, s := range b.subs {
		select {
		case s.queue <- e:
		default:
			delete(b.subs, id)
			close(s.done)
		}
	}
	return e.Sequence, nil
}

func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for id, s := range b.subs {
		delete(b.subs, id)
		close(s.done)
	}
}

func (b *Broker) SubscriberCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}
