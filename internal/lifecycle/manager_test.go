package lifecycle

import (
	"context"
	"testing"
	"time"
)

func TestManagerLazyLoadsAndClosesIdle(t *testing.T) {
	created := 0
	closed := 0
	m := NewManager[string](time.Minute, func(ctx context.Context, key string) (Resource, error) {
		created++
		return ResourceFunc(func() error { closed++; return nil }), nil
	})

	ctx := context.Background()
	release, err := m.Acquire(ctx, "saas_support")
	if err != nil {
		t.Fatal(err)
	}
	if created != 1 {
		t.Fatalf("created=%d", created)
	}
	release()

	m.CloseIdle(time.Now().Add(2 * time.Minute))
	if closed != 1 {
		t.Fatalf("closed=%d", closed)
	}
}

func TestManagerDoesNotCloseInFlightResource(t *testing.T) {
	closed := 0
	m := NewManager[string](time.Minute, func(ctx context.Context, key string) (Resource, error) {
		return ResourceFunc(func() error { closed++; return nil }), nil
	})

	release, err := m.Acquire(context.Background(), "saas_support")
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	m.CloseIdle(time.Now().Add(2 * time.Minute))
	if closed != 0 {
		t.Fatalf("closed in-flight resource")
	}
}

func TestManagerReusesExistingResource(t *testing.T) {
	created := 0
	m := NewManager[string](time.Minute, func(ctx context.Context, key string) (Resource, error) {
		created++
		return ResourceFunc(func() error { return nil }), nil
	})

	ctx := context.Background()
	release1, err := m.Acquire(ctx, "db1")
	if err != nil {
		t.Fatal(err)
	}
	release2, err := m.Acquire(ctx, "db1")
	if err != nil {
		t.Fatal(err)
	}

	if created != 1 {
		t.Fatalf("expected 1 creation, got %d", created)
	}

	release1()
	release2()
}

func TestManagerCloseIdleDoesNotAffectFreshResource(t *testing.T) {
	closed := 0
	m := NewManager[string](time.Minute, func(ctx context.Context, key string) (Resource, error) {
		return ResourceFunc(func() error { closed++; return nil }), nil
	})

	release, err := m.Acquire(context.Background(), "db1")
	if err != nil {
		t.Fatal(err)
	}
	release()

	// Now is before the idle TTL -- should NOT close.
	m.CloseIdle(time.Now())
	if closed != 0 {
		t.Fatalf("closed a resource that is not yet idle")
	}
}

func TestManagerCloseShutsDownAllResources(t *testing.T) {
	closed := 0
	m := NewManager[string](time.Minute, func(ctx context.Context, key string) (Resource, error) {
		return ResourceFunc(func() error { closed++; return nil }), nil
	})

	_, _ = m.Acquire(context.Background(), "a")
	_, _ = m.Acquire(context.Background(), "b")

	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if closed != 2 {
		t.Fatalf("expected 2 closed, got %d", closed)
	}
}

func TestManagerAcquireReturnsFactoryError(t *testing.T) {
	expected := context.Canceled
	m := NewManager[string](time.Minute, func(ctx context.Context, key string) (Resource, error) {
		return nil, expected
	})

	_, err := m.Acquire(context.Background(), "bad")
	if err != expected {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}

func TestManagerConcurrentAcquireAndRelease(t *testing.T) {
	m := NewManager[string](time.Minute, func(ctx context.Context, key string) (Resource, error) {
		return ResourceFunc(func() error { return nil }), nil
	})

	ctx := context.Background()
	for i := 0; i < 100; i++ {
		go func() {
			release, err := m.Acquire(ctx, "shared")
			if err != nil {
				t.Errorf("acquire error: %v", err)
				return
			}
			release()
		}()
	}

	// Allow goroutines to finish (best-effort; the -race detector is the
	// real validator here).
	time.Sleep(50 * time.Millisecond)
}
