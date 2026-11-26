package events

import (
	"errors"
	"sync"
	"testing"
)

func TestBrokerSequenceOwnershipAndCapacity(t *testing.T) {
	b := New()
	subs := make([]*Subscription, 0, MaxSubscribers)
	for i := 0; i < MaxSubscribers; i++ {
		s, boundary, err := b.Subscribe()
		if err != nil || boundary != 0 {
			t.Fatalf("subscribe %d: %d %v", i, boundary, err)
		}
		subs = append(subs, s)
	}
	if _, _, err := b.Subscribe(); !errors.Is(err, ErrFull) {
		t.Fatalf("over capacity: %v", err)
	}
	data := []byte(`{"id":"one"}`)
	seq, err := b.Publish("instance.registered", data)
	if err != nil || seq != 1 {
		t.Fatalf("publish: %d %v", seq, err)
	}
	data[7] = 'X'
	for _, s := range subs {
		e := <-s.Events()
		if e.Sequence != 1 || e.Data != `{"id":"one"}` {
			t.Fatalf("mutable or wrong event: %+v", e)
		}
	}
	if _, err := b.Publish("instance.log", make([]byte, MaxPayload+1)); !errors.Is(err, ErrPayload) {
		t.Fatalf("oversize: %v", err)
	}
	subs[0].Cancel()
	select {
	case <-subs[0].Done():
	default:
		t.Fatal("cancel did not close")
	}
	if _, _, err := b.Subscribe(); err != nil {
		t.Fatal(err)
	}
	b.Close()
	for _, s := range subs {
		select {
		case <-s.Done():
		default:
			t.Fatal("close did not release subscriber")
		}
	}
	if _, err := b.Publish("instance.state", nil); !errors.Is(err, ErrClosed) {
		t.Fatalf("publish after close: %v", err)
	}
}

func TestBrokerSlowSubscriberIsolationAndConcurrentPublication(t *testing.T) {
	b := New()
	slow, _, _ := b.Subscribe()
	fast, _, _ := b.Subscribe()
	for i := 0; i < QueueSize; i++ {
		if _, err := b.Publish("instance.state", []byte(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= QueueSize; i++ {
		e := <-fast.Events()
		if e.Sequence != uint64(i) {
			t.Fatalf("gap at %d: %+v", i, e)
		}
	}
	if _, err := b.Publish("instance.state", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-slow.Done():
	default:
		t.Fatal("slow subscriber not disconnected")
	}
	select {
	case <-fast.Done():
		t.Fatal("fast subscriber disconnected")
	default:
	}
	if e := <-fast.Events(); e.Sequence != QueueSize+1 {
		t.Fatalf("fast lost event: %+v", e)
	}
	b.Close()

	b = New()
	ordered, _, _ := b.Subscribe()
	const publishers = 8
	const each = 16
	var wg sync.WaitGroup
	results := make(chan error, publishers*each)
	for i := 0; i < publishers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < each; j++ {
				_, err := b.Publish("instance.state", []byte(`{}`))
				results <- err
			}
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= QueueSize; i++ {
		e := <-ordered.Events()
		if e.Sequence != uint64(i) {
			t.Fatalf("gap at %d: %+v", i, e)
		}
	}
	b.Close()
}

func TestBrokerPublishCloseRace(t *testing.T) {
	b := New()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, _ = b.Publish("instance.state", []byte(`{}`))
			}
		}()
	}
	b.Close()
	wg.Wait()
	if b.SubscriberCount() != 0 {
		t.Fatal("subscribers retained")
	}
}
