package service

import (
	"testing"
	"time"
)

var degradedBase = time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)

// TestDegraded_DwellEnforced verifies that after a successful rotation a
// second rotation is refused until RotateInterval has elapsed, then
// succeeds.
func TestDegraded_DwellEnforced(t *testing.T) {
	dp := newDegradedPeer([]string{"a:1", "b:2"}, degradedBase)

	got, ok := dp.rotate(degradedBase)
	if !ok || got != "a:1" {
		t.Fatalf("first rotate = (%q, %v), want (a:1, true)", got, ok)
	}

	if _, ok := dp.rotate(degradedBase.Add(RotateInterval - time.Second)); ok {
		t.Fatal("rotate before dwell elapsed should return false")
	}

	got, ok = dp.rotate(degradedBase.Add(RotateInterval))
	if !ok || got != "b:2" {
		t.Fatalf("rotate after dwell = (%q, %v), want (b:2, true)", got, ok)
	}
}

// TestDegraded_ExhaustionBackoff verifies that exhausting all candidates
// goes idle with DefaultBackoff, the next cycle doubles, and the backoff
// is capped at MaxBackoff.
func TestDegraded_ExhaustionBackoff(t *testing.T) {
	dp := newDegradedPeer([]string{"a:1"}, degradedBase)

	// Single candidate: rotating it exhausts the cycle immediately.
	if _, ok := dp.rotate(degradedBase); !ok {
		t.Fatal("first rotate should succeed")
	}
	if !dp.idle {
		t.Fatal("peer should be idle after exhausting only candidate")
	}
	if got := dp.nextAttempt.Sub(degradedBase); got != DefaultBackoff {
		t.Fatalf("first backoff = %v, want %v", got, DefaultBackoff)
	}

	// Second cycle: wake at backoff expiry, exhaust again.
	wake := dp.nextAttempt
	if _, ok := dp.rotate(wake); !ok {
		t.Fatal("second cycle rotate should succeed")
	}
	if got := dp.nextAttempt.Sub(wake); got != 2*DefaultBackoff {
		t.Fatalf("second backoff = %v, want %v", got, 2*DefaultBackoff)
	}

	// Drive enough cycles that the doubling would exceed MaxBackoff.
	for i := 0; i < 10; i++ {
		wake = dp.nextAttempt
		if _, ok := dp.rotate(wake); !ok {
			t.Fatalf("cycle %d rotate should succeed", i)
		}
	}
	if got := dp.nextAttempt.Sub(wake); got != MaxBackoff {
		t.Fatalf("capped backoff = %v, want %v", got, MaxBackoff)
	}
}

// TestDegraded_RefreshWakes verifies that refreshing an idle peer with a
// new untried candidate wakes it and the candidate is returned on the
// next rotate.
func TestDegraded_RefreshWakes(t *testing.T) {
	dp := newDegradedPeer([]string{"a:1"}, degradedBase)

	if _, ok := dp.rotate(degradedBase); !ok {
		t.Fatal("first rotate should succeed")
	}
	if !dp.idle {
		t.Fatal("peer should be idle after exhaustion")
	}

	wake := dp.nextAttempt
	dp.refresh([]string{"a:1", "b:2"})
	if dp.idle {
		t.Fatal("refresh with untried candidate should wake the peer")
	}

	got, ok := dp.rotate(wake)
	if !ok || got != "b:2" {
		t.Fatalf("rotate after refresh = (%q, %v), want (b:2, true)", got, ok)
	}
}

// TestDegraded_EmptyStartsIdle verifies that constructing with no
// candidates starts idle and rotate returns false until backoff expires.
func TestDegraded_EmptyStartsIdle(t *testing.T) {
	dp := newDegradedPeer(nil, degradedBase)

	if !dp.idle {
		t.Fatal("empty-candidate peer should start idle")
	}
	if _, ok := dp.rotate(degradedBase); ok {
		t.Fatal("rotate during initial backoff should return false")
	}

	// After backoff expires with still no candidates, rotate re-exhausts
	// and stays false.
	if _, ok := dp.rotate(dp.nextAttempt); ok {
		t.Fatal("rotate with no candidates should return false")
	}
}
