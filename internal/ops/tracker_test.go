package ops

import (
	"context"
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
