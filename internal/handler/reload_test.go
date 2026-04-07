package handler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestReloadWorker_CoalescesBurst verifies that a burst of requestReload
// calls within the debounce window results in a single loadWorkspace
// invocation.
func TestReloadWorker_CoalescesBurst(t *testing.T) {
	s := New("test")

	var loadCount int32
	s.loadHook = func(ctx context.Context) {
		atomic.AddInt32(&loadCount, 1)
	}

	s.startReloadWorker()
	t.Cleanup(func() { s.stopReloadWorker() })

	for range 10 {
		s.requestReload("burst")
	}

	// Wait for debounce window + load to complete. Give generous slack.
	waitFor(t, 2*time.Second, func() bool {
		return atomic.LoadInt32(&loadCount) >= 1
	})

	// Allow any additional loads a chance to fire, then assert count.
	time.Sleep(reloadDebounceWindow + 200*time.Millisecond)
	if got := atomic.LoadInt32(&loadCount); got != 1 {
		t.Errorf("expected exactly 1 load after burst, got %d", got)
	}
}

// TestReloadWorker_EventDuringLoad verifies that a reload request arriving
// while a load is in progress triggers a second load after the first
// completes.
func TestReloadWorker_EventDuringLoad(t *testing.T) {
	s := New("test")

	var loadCount int32
	release := make(chan struct{})
	firstLoadStarted := make(chan struct{})

	s.loadHook = func(ctx context.Context) {
		n := atomic.AddInt32(&loadCount, 1)
		if n == 1 {
			close(firstLoadStarted)
			// Block the first load until the test releases it.
			select {
			case <-release:
			case <-ctx.Done():
			}
		}
	}

	s.startReloadWorker()
	t.Cleanup(func() { s.stopReloadWorker() })

	// Kick the first load.
	s.requestReload("first")

	// Wait until we're inside loadWorkspace.
	select {
	case <-firstLoadStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first load did not start")
	}

	// Queue a second reload while the first is still in flight.
	s.requestReload("second")

	// Release the first load.
	close(release)

	// Wait for the second load to arrive.
	waitFor(t, 2*time.Second, func() bool {
		return atomic.LoadInt32(&loadCount) >= 2
	})
}

// TestReloadWorker_ShutdownCancels verifies that stopReloadWorker closes
// workerDone promptly when no load is in flight.
func TestReloadWorker_ShutdownCancels(t *testing.T) {
	s := New("test")
	s.startReloadWorker()

	done := make(chan struct{})
	go func() {
		s.stopReloadWorker()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("stopReloadWorker did not return within 500ms")
	}
}

// TestReloadWorker_ShutdownCancelsInFlight verifies that stopReloadWorker
// cancels an in-flight load by closing its context, and that workerDone
// closes after the worker observes cancellation.
func TestReloadWorker_ShutdownCancelsInFlight(t *testing.T) {
	s := New("test")

	hookObservedCancel := make(chan struct{})
	hookEntered := make(chan struct{})

	s.loadHook = func(ctx context.Context) {
		close(hookEntered)
		// Wait for the context to be cancelled.
		<-ctx.Done()
		if ctx.Err() == context.Canceled {
			close(hookObservedCancel)
		}
	}

	s.startReloadWorker()
	s.requestReload("in-flight")

	// Wait for the hook to be entered.
	select {
	case <-hookEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("load hook did not fire")
	}

	// Stop the worker; this should cancel the context.
	done := make(chan struct{})
	go func() {
		s.stopReloadWorker()
		close(done)
	}()

	select {
	case <-hookObservedCancel:
	case <-time.After(1 * time.Second):
		t.Fatal("load hook did not observe context cancellation")
	}

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("stopReloadWorker did not return after in-flight cancellation")
	}
}

// TestRequestReload_FullChannelDropsEvents verifies that requestReload does
// not block when the channel is full. The worker is not started so the
// channel drains nowhere.
func TestRequestReload_FullChannelDropsEvents(t *testing.T) {
	s := New("test")
	// Manually set up the channel without starting the worker goroutine.
	s.reloadCh = make(chan reloadEvent, reloadChannelCapacity)

	// Fill the channel to capacity.
	for range reloadChannelCapacity {
		s.requestReload("fill")
	}

	// Verify the channel is actually full.
	if len(s.reloadCh) != reloadChannelCapacity {
		t.Fatalf("expected channel len %d, got %d", reloadChannelCapacity, len(s.reloadCh))
	}

	// Additional calls must not block; each one should return promptly.
	done := make(chan struct{})
	go func() {
		for range 20 {
			s.requestReload("overflow")
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("requestReload blocked on full channel")
	}
}

// TestRequestReload_NilChannelIsSafe verifies that requestReload is a no-op
// when the channel has not been initialized (worker never started).
func TestRequestReload_NilChannelIsSafe(t *testing.T) {
	s := New("test")
	// Do not call startReloadWorker; s.reloadCh is nil.

	done := make(chan struct{})
	go func() {
		s.requestReload("noop")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("requestReload blocked on nil channel")
	}
}
