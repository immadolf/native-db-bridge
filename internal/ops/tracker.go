package ops

import (
	"context"
	"sync"
)

// Tracker manages in-flight operation cancel functions. Each operation
// registers a context.CancelFunc at start and removes it on completion
// or cancellation.
type Tracker struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

// NewTracker returns an empty Tracker.
func NewTracker() *Tracker {
	return &Tracker{cancels: map[string]context.CancelFunc{}}
}

// Register stores a cancel function for the given operation ID.
func (t *Tracker) Register(operationID string, cancel context.CancelFunc) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cancels[operationID] = cancel
}

// Cancel invokes and removes the cancel function for the given operation ID.
// Returns true if the operation was found and cancelled, false if it was
// already completed or unknown.
func (t *Tracker) Cancel(operationID string) bool {
	t.mu.Lock()
	cancel, ok := t.cancels[operationID]
	if ok {
		delete(t.cancels, operationID)
	}
	t.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

// Finish removes the operation from the tracker without calling its cancel
// function. Used when an operation completes normally.
func (t *Tracker) Finish(operationID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.cancels, operationID)
}
