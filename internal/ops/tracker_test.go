package ops

import (
	"context"
	"errors"
	"testing"
)

func TestTrackerCancelCallsCancelFuncOnce(t *testing.T) {
	tracker := NewTracker()
	ctx, cancel := context.WithCancel(context.Background())
	tracker.Register("op_1", cancel)

	if !tracker.Cancel("op_1") {
		t.Fatalf("first cancel should find operation")
	}
	if tracker.Cancel("op_1") {
		t.Fatalf("second cancel should not find operation")
	}

	select {
	case <-ctx.Done():
	default:
		t.Fatalf("context was not cancelled")
	}
}

func TestTrackerFinishRemovesWithoutCancelling(t *testing.T) {
	tracker := NewTracker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tracker.Register("op_2", cancel)
	tracker.Finish("op_2")

	if tracker.Cancel("op_2") {
		t.Fatalf("cancel after finish should return false")
	}

	select {
	case <-ctx.Done():
		t.Fatalf("context should not be cancelled after Finish")
	default:
		// expected
	}
}

func TestTrackerCancelUnknownOperation(t *testing.T) {
	tracker := NewTracker()
	if tracker.Cancel("nonexistent") {
		t.Fatalf("cancel of unknown operation should return false")
	}
}

func TestTrackerCancelAfterRunsCallbackBeforeCancel(t *testing.T) {
	tracker := NewTracker()
	ctx, cancel := context.WithCancel(context.Background())
	tracker.Register("op_3", cancel)

	callbackRan := false
	found, err := tracker.CancelAfter("op_3", func() error {
		callbackRan = true
		select {
		case <-ctx.Done():
			t.Fatalf("context cancelled before callback")
		default:
			return nil
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !found || !callbackRan {
		t.Fatalf("found=%v callbackRan=%v", found, callbackRan)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatalf("context was not cancelled")
	}
}

func TestTrackerCancelAfterCallbackErrorKeepsOperation(t *testing.T) {
	tracker := NewTracker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tracker.Register("op_4", cancel)

	wantErr := errors.New("persist failed")
	found, err := tracker.CancelAfter("op_4", func() error { return wantErr })
	if !found {
		t.Fatal("operation should be found")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("err=%v, want %v", err, wantErr)
	}
	select {
	case <-ctx.Done():
		t.Fatalf("context should not be cancelled after callback error")
	default:
	}
	if !tracker.Cancel("op_4") {
		t.Fatalf("operation should remain registered after callback error")
	}
}
