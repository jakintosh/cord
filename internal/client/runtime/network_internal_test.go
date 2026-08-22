package runtime

import (
	"context"
	"testing"
	"time"
)

func TestNetworkStopCancelsBeforeWaitingForActivity(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	network := &Network{
		ctx:         ctx,
		cancel:      cancel,
		syncTimer:   time.NewTimer(time.Hour),
		scanTimer:   time.NewTimer(time.Hour),
		reportTimer: time.NewTimer(time.Hour),
		tunnel:      &Tunnel{},
	}

	// Model an activity in flight. Shutdown must publish cancellation
	// before waiting to acquire this lock.
	network.mu.Lock()
	stopped := make(chan struct{})
	go func() {
		_ = network.stop()
		close(stopped)
	}()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("stop waited for the activity lock before cancelling")
	}

	select {
	case <-stopped:
		t.Fatal("stop returned while an activity still held the lock")
	default:
	}

	network.mu.Unlock()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("stop did not finish after the activity released the lock")
	}
}
