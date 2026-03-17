package bus_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/theglove44/tastytrade-cli/internal/bus"
)

// ── Basic fan-out ─────────────────────────────────────────────────────────────

func TestBroker_Subscribe_ReceivesPublished(t *testing.T) {
	b := bus.New[int](nil)
	ch := b.Subscribe(8)

	b.Publish(1)
	b.Publish(2)
	b.Publish(3)

	for want := 1; want <= 3; want++ {
		select {
		case got := <-ch:
			if got != want {
				t.Errorf("event %d: got %d, want %d", want, got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for event %d", want)
		}
	}
}

func TestBroker_FanOut_MultipleSubscribers(t *testing.T) {
	b := bus.New[string](nil)
	ch1 := b.Subscribe(4)
	ch2 := b.Subscribe(4)
	ch3 := b.Subscribe(4)

	events := []string{"alpha", "beta", "gamma"}
	for _, ev := range events {
		b.Publish(ev)
	}

	for _, ch := range []<-chan string{ch1, ch2, ch3} {
		for _, want := range events {
			select {
			case got := <-ch:
				if got != want {
					t.Errorf("got %q, want %q", got, want)
				}
			case <-time.After(time.Second):
				t.Fatalf("timeout waiting for %q", want)
			}
		}
	}
}

// ── Drop-on-full semantics ────────────────────────────────────────────────────

// TestBroker_DropOnFull verifies non-blocking publish semantics: a full
// subscriber channel drops the event; other subscribers still receive it.
func TestBroker_DropOnFull(t *testing.T) {
	b := bus.New[int](nil)
	full := b.Subscribe(0) // zero-capacity: always full
	ok := b.Subscribe(5)   // capacity exactly matches publish count

	publishDone := make(chan struct{})
	go func() {
		defer close(publishDone)
		for i := 0; i < 5; i++ {
			b.Publish(i)
		}
	}()

	select {
	case <-publishDone:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on a full subscriber channel")
	}

	select {
	case v := <-full:
		t.Errorf("zero-capacity subscriber received %d — expected drop", v)
	default:
	}

	for want := 0; want < 5; want++ {
		select {
		case got := <-ok:
			if got != want {
				t.Errorf("ok channel: got %d, want %d", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("ok channel: timeout waiting for %d", want)
		}
	}
}

// TestBroker_DropOnFull_CallsOnDrop verifies that the onDrop callback is
// invoked exactly once per dropped event, and that Drops() reflects the total.
func TestBroker_DropOnFull_CallsOnDrop(t *testing.T) {
	var callbackCount atomic.Int64
	onDrop := func() { callbackCount.Add(1) }

	b := bus.New[int](onDrop)
	_ = b.Subscribe(0) // always full — every publish is a drop

	b.Publish(1)
	b.Publish(2)
	b.Publish(3)

	if got := callbackCount.Load(); got != 3 {
		t.Errorf("onDrop called %d times, want 3", got)
	}
	if got := b.Drops(); got != 3 {
		t.Errorf("Drops() = %d, want 3", got)
	}
}

// TestBroker_NilOnDrop_NoPanic verifies that nil onDrop is safe — a common
// case for buses where drops are only counted at the metric layer.
func TestBroker_NilOnDrop_NoPanic(t *testing.T) {
	b := bus.New[int](nil)
	_ = b.Subscribe(0) // always full
	b.Publish(42)      // must not panic with nil onDrop
	if b.Drops() != 1 {
		t.Errorf("Drops() = %d, want 1", b.Drops())
	}
}

// TestBroker_Drops_PartialDrop verifies that Drops() counts only the drops
// (full channels), not successful sends.
func TestBroker_Drops_PartialDrop(t *testing.T) {
	b := bus.New[int](nil)
	full := b.Subscribe(0) // drops everything
	ok := b.Subscribe(4)   // receives everything
	_ = full

	b.Publish(1)
	b.Publish(2)

	// full drops 2, ok receives 2
	if d := b.Drops(); d != 2 {
		t.Errorf("Drops() = %d, want 2", d)
	}
	// ok channel should have both events
	for range []int{1, 2} {
		select {
		case <-ok:
		case <-time.After(time.Second):
			t.Fatal("ok channel missing event")
		}
	}
}

// ── Close behaviour ───────────────────────────────────────────────────────────

func TestBroker_Close_ClosesSubscriberChannels(t *testing.T) {
	b := bus.New[int](nil)
	ch1 := b.Subscribe(4)
	ch2 := b.Subscribe(4)

	b.Publish(42)
	b.Close()

	for _, ch := range []<-chan int{ch1, ch2} {
		for {
			select {
			case _, ok := <-ch:
				if !ok {
					goto closed
				}
			case <-time.After(time.Second):
				t.Fatal("subscriber channel not closed within 1s after Close()")
			}
		}
	closed:
	}
}

func TestBroker_Close_IdempotentDoubleClosed(t *testing.T) {
	b := bus.New[int](nil)
	_ = b.Subscribe(4)
	b.Close()
	b.Close() // must not panic
}

func TestBroker_PublishAfterClose_NoOp(t *testing.T) {
	b := bus.New[int](nil)
	b.Close()
	b.Publish(99) // must not panic
}

func TestBroker_SubscribeAfterClose_ReturnsClosed(t *testing.T) {
	b := bus.New[int](nil)
	b.Close()
	ch := b.Subscribe(4)

	select {
	case _, ok := <-ch:
		if ok {
			t.Error("channel should be closed (ok=false) when subscribing after Close")
		}
	case <-time.After(time.Second):
		t.Fatal("channel from post-Close Subscribe is not closed — consumer would hang")
	}
}

// ── Consumer drain via WaitGroup ──────────────────────────────────────────────

// TestBroker_ConsumerDrain_WaitGroup verifies the shutdown pattern used in
// root.go: Close() → wg.Wait() ensures all buffered events are processed before
// the caller proceeds (e.g. store.Close()).
//
// This is the canonical test for graceful consumer drain.
func TestBroker_ConsumerDrain_WaitGroup(t *testing.T) {
	b := bus.New[int](nil)
	ch := b.Subscribe(64)

	// Publish 20 events before starting the consumer — they buffer in the channel.
	for i := 0; i < 20; i++ {
		b.Publish(i)
	}

	var processed atomic.Int64
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range ch {
			processed.Add(1)
			time.Sleep(time.Microsecond) // simulate light work
		}
	}()

	// Close the bus — consumer drains the buffered 20 events then exits.
	b.Close()
	// wg.Wait() must not return until all events are processed.
	waitDone := make(chan struct{})
	go func() { wg.Wait(); close(waitDone) }()

	select {
	case <-waitDone:
	case <-time.After(2 * time.Second):
		t.Fatal("wg.Wait() did not return within 2s after Close()")
	}

	if got := processed.Load(); got != 20 {
		t.Errorf("consumer processed %d events, want 20 — events were lost during drain", got)
	}
}

// TestBroker_ConsumerDrain_StoreClosedAfterDrain verifies the ordering
// guarantee: the "store" (simulated by a flag) is only closed after all
// consumers have exited. This is the critical property that prevents fill-write
// races on shutdown.
func TestBroker_ConsumerDrain_StoreClosedAfterDrain(t *testing.T) {
	b := bus.New[int](nil)
	ch := b.Subscribe(32)

	for i := 0; i < 10; i++ {
		b.Publish(i)
	}

	storeClosed := false
	allProcessed := false
	var mu sync.Mutex

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range ch {
			// Simulate store write — verify store not yet closed.
			mu.Lock()
			if storeClosed {
				t.Error("store was closed while consumer was still processing")
			}
			mu.Unlock()
		}
		// Consumer done — mark for verification.
		mu.Lock()
		allProcessed = true
		mu.Unlock()
	}()

	b.Close()
	wg.Wait()

	mu.Lock()
	storeClosed = true // simulate st.Close() after wg.Wait()
	if !allProcessed {
		t.Error("wg.Wait() returned before consumer finished processing")
	}
	mu.Unlock()
}

// ── Concurrency ───────────────────────────────────────────────────────────────

func TestBroker_ConcurrentPublish_Safe(t *testing.T) {
	b := bus.New[int](nil)
	ch := b.Subscribe(256)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				b.Publish(n*100 + j)
			}
		}(i)
	}
	wg.Wait()
	b.Close()

	for range ch {
	}
}
