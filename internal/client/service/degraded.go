package service

import "time"

// RotateInterval is the minimum time between endpoint rotation attempts
// for a single degraded peer.
const RotateInterval = 90 * time.Second

// DefaultBackoff is the base duration for exponential backoff when a
// degraded peer has exhausted all candidate endpoints.
const DefaultBackoff = 5 * time.Minute

// MaxBackoff caps the exponential backoff duration.
const MaxBackoff = 1 * time.Hour

// degradedPeer tracks the endpoint rotation state for a peer that has
// lost its handshake. It maintains an ordered list of candidate
// endpoints and cycles through them, with exponential backoff after
// exhausting all candidates.
//
// All methods are called from the owner goroutine (the Network loop);
// no locking is needed.
type degradedPeer struct {
	candidates  []string        // ordered best-first, from the endpoint catalog
	tried       map[string]bool // attempted this cycle
	cycles      int             // completed exhaustion cycles, drives backoff
	idle        bool            // exhausted, waiting out backoff
	nextAttempt time.Time
}

// newDegradedPeer builds a degradedPeer with the given candidate list.
// With no candidates it starts idle, waiting out the initial backoff.
func newDegradedPeer(
	candidates []string,
	now time.Time,
) *degradedPeer {
	dp := &degradedPeer{
		candidates: candidates,
		tried:      make(map[string]bool),
	}
	if len(candidates) == 0 {
		dp.idle = true
		dp.nextAttempt = now.Add(degradedBackoff(0))
	}
	return dp
}

// refresh replaces the candidate list from fresh catalog data.
// An idle peer with untried candidates wakes immediately.
func (dp *degradedPeer) refresh(
	candidates []string,
) {
	dp.candidates = candidates
	if dp.idle {
		for _, c := range candidates {
			if !dp.tried[c] {
				dp.idle = false
				return
			}
		}
	}
}

// rotate advances the rotation by one step. It returns the endpoint to
// apply and true, or "" and false when nothing is due. The nextAttempt
// gate enforces both the per-attempt dwell and the idle backoff.
func (dp *degradedPeer) rotate(
	now time.Time,
) (
	string,
	bool,
) {
	if now.Before(dp.nextAttempt) {
		return "", false
	}
	if dp.idle {
		dp.tried = make(map[string]bool)
		dp.idle = false
	}

	candidate := ""
	for _, c := range dp.candidates {
		if !dp.tried[c] {
			candidate = c
			break
		}
	}
	if candidate == "" {
		dp.exhaust(now)
		return "", false
	}

	dp.tried[candidate] = true
	if dp.hasUntried() {
		dp.nextAttempt = now.Add(RotateInterval)
	} else {
		dp.exhaust(now)
	}
	return candidate, true
}

// hasUntried reports whether any candidate remains untried this cycle.
func (dp *degradedPeer) hasUntried() bool {
	for _, c := range dp.candidates {
		if !dp.tried[c] {
			return true
		}
	}
	return false
}

// exhaust marks the peer idle with exponential backoff after all
// candidates have been exhausted.
func (dp *degradedPeer) exhaust(
	now time.Time,
) {
	dp.idle = true
	dp.cycles++
	dp.nextAttempt = now.Add(degradedBackoff(dp.cycles))
}

// degradedBackoff returns the exponential backoff duration for the
// given cycle count, capped at MaxBackoff.
func degradedBackoff(
	cycles int,
) time.Duration {
	if cycles <= 0 {
		return DefaultBackoff
	}
	exp := time.Duration(1 << (cycles - 1))
	d := min(DefaultBackoff*exp, MaxBackoff)
	return d
}
