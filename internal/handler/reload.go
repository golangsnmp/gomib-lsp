package handler

import (
	"context"
	"log"
	"time"
)

// reloadDebounceWindow is how long the worker waits after the first event
// in a burst before running loadWorkspace.
const reloadDebounceWindow = 250 * time.Millisecond

// reloadChannelCapacity bounds the reload event buffer. If the buffer fills,
// excess events are dropped because the worker is already going to reload.
const reloadChannelCapacity = 64

// reloadEvent carries the reason a reload was requested. The reason is used
// only for logging; the worker always performs a full workspace load.
type reloadEvent struct {
	reason string
}

// startReloadWorker initializes the reload channel and launches the worker
// goroutine. Must be called exactly once per Server instance, before any
// requestReload calls.
func (s *Server) startReloadWorker() {
	s.workerCtx, s.workerStop = context.WithCancel(context.Background())
	s.reloadCh = make(chan reloadEvent, reloadChannelCapacity)
	s.workerDone = make(chan struct{})
	go s.reloadWorker()
}

// stopReloadWorker cancels the worker context and waits for the goroutine
// to exit. Safe to call from shutdown.
func (s *Server) stopReloadWorker() {
	if s.workerStop == nil {
		return
	}
	s.workerStop()
	<-s.workerDone
}

// requestReload queues a reload event for the worker. Non-blocking: if the
// channel is full, the event is dropped because the worker will reload anyway.
func (s *Server) requestReload(reason string) {
	if s.reloadCh == nil {
		return
	}
	select {
	case s.reloadCh <- reloadEvent{reason: reason}:
	default:
		log.Printf("mib-lsp: reload channel full, dropping event (reason=%s)", reason)
	}
}

// reloadWorker is the goroutine body. It waits for reload events, debounces
// bursts by draining additional events for reloadDebounceWindow, then calls
// loadWorkspace and publishAllDiagnostics. Events that arrive during a load
// remain in the channel and are picked up on the next iteration.
func (s *Server) reloadWorker() {
	defer close(s.workerDone)

	for {
		// Wait for the first event or shutdown.
		var first reloadEvent
		select {
		case <-s.workerCtx.Done():
			return
		case ev := <-s.reloadCh:
			first = ev
		}
		log.Printf("mib-lsp: reload requested (reason=%s)", first.reason)

		// Drain additional events during the debounce window.
		timer := time.NewTimer(reloadDebounceWindow)
	drain:
		for {
			select {
			case <-s.workerCtx.Done():
				timer.Stop()
				return
			case <-s.reloadCh:
				// coalesce
			case <-timer.C:
				break drain
			}
		}

		// Run the load. We do not cancel on new events; they remain in the
		// channel for the next iteration.
		s.loadWorkspace(s.workerCtx)
		if s.workerCtx.Err() != nil {
			return
		}
		s.publishAllDiagnostics()
	}
}
