// Package lifecycle manages lazy creation and idle cleanup of resources
// keyed by an arbitrary comparable type. Resources are created on first
// Acquire and closed after they have been idle (zero in-flight references)
// for longer than the configured TTL.
package lifecycle

import (
	"context"
	"sync"
	"time"
)

// Resource is anything that can be closed when no longer needed.
type Resource interface {
	Close() error
}

// ResourceFunc adapts a plain function to the Resource interface.
type ResourceFunc func() error

// Close calls the underlying function.
func (f ResourceFunc) Close() error {
	return f()
}

// Factory creates a new resource for the given key.
type Factory[K comparable] func(ctx context.Context, key K) (Resource, error)

// entry tracks a single managed resource.
type entry struct {
	resource Resource
	inFlight int
	lastUsed time.Time
}

// Manager lazily creates resources and closes them after an idle period.
// The type parameter K is the key type used to identify resources (typically
// a datasource name or connection string).
type Manager[K comparable] struct {
	mu      sync.Mutex
	idleTTL time.Duration
	factory Factory[K]
	entries map[K]*entry
}

// NewManager returns a Manager that uses factory to create resources and
// considers them idle after idleTTL with zero in-flight references.
func NewManager[K comparable](idleTTL time.Duration, factory Factory[K]) *Manager[K] {
	return &Manager[K]{
		idleTTL: idleTTL,
		factory: factory,
		entries: make(map[K]*entry),
	}
}

// Acquire returns a release function for the resource identified by key.
// The resource is lazily created on first access. The caller MUST call the
// returned release function when done with the resource.
func (m *Manager[K]) Acquire(ctx context.Context, key K) (func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.entries[key]
	if !ok {
		r, err := m.factory(ctx, key)
		if err != nil {
			return nil, err
		}
		e = &entry{resource: r}
		m.entries[key] = e
	}

	e.inFlight++
	e.lastUsed = time.Now()

	var once sync.Once
	release := func() {
		once.Do(func() {
			m.mu.Lock()
			e.inFlight--
			e.lastUsed = time.Now()
			m.mu.Unlock()
		})
	}

	return release, nil
}

// Get returns the managed resource for key if it exists. The resource
// is NOT considered in-flight; the caller should use Acquire for
// operations that need the resource to stay alive during use.
// This method is intended for type-asserting the resource after Acquire.
func (m *Manager[K]) Get(key K) (Resource, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[key]
	if !ok {
		return nil, false
	}
	return e.resource, true
}

// Len returns the number of resources currently tracked by the manager
// (i.e., resources that have been lazily created and not yet closed).
func (m *Manager[K]) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.entries)
}

// CloseIdle closes and removes every resource whose in-flight count is zero
// and whose last-used time is before now (i.e. has been idle for longer
// than the configured TTL). Resources with active references are left alone.
func (m *Manager[K]) CloseIdle(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, e := range m.entries {
		if e.inFlight > 0 {
			continue
		}
		if now.Sub(e.lastUsed) > m.idleTTL {
			e.resource.Close()
			delete(m.entries, key)
		}
	}
}

// Close shuts down the manager, closing all managed resources regardless
// of in-flight state. It is intended for graceful shutdown.
func (m *Manager[K]) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for key, e := range m.entries {
		if err := e.resource.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(m.entries, key)
	}
	return firstErr
}
