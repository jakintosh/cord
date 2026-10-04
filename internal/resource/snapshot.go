package resource

import "time"

// Health describes whether a resource and its descendants satisfy their targets.
type Health string

const (
	HealthHealthy  Health = "healthy"
	HealthDegraded Health = "degraded"
	HealthInactive Health = "inactive"
)

// Snapshot is a detached ownership tree collected by an owner.
// State describes availability; Target describes the intended lifetime.
// Reason is an optional machine-readable explanation of the current condition.
type Snapshot struct {
	Kind       string
	Name       string
	State      State
	Target     State
	Reason     string
	Error      string
	Health     Health
	Activities map[string]ActivityStatus
	Children   []Snapshot
}

// ActivityStatus holds the schedule and latest result of one owned activity.
// Interval is zero for work without a periodic schedule. NextAttemptAt is zero
// when no next attempt is recorded; the owner controls scheduling.
type ActivityStatus struct {
	NextAttemptAt time.Time
	Interval      time.Duration
	LastAttemptAt time.Time
	LastSuccessAt time.Time
	Error         string
}

// UpdateHealth combines local state and activity with already-collected child health.
// Once the target is stopped, activity history does not affect health: owners
// must represent unfinished cleanup in resource state, errors, or child health.
func (s *Snapshot) UpdateHealth() {
	s.Health = HealthHealthy
	if s.State == Stopped && s.Target == Stopped {
		s.Health = HealthInactive
	}

	if s.Error != "" || s.State == Failed || s.State != s.Target {
		s.Health = HealthDegraded
	}

	if s.Target != Stopped {
		for _, activity := range s.Activities {
			if activity.Error != "" {
				s.Health = HealthDegraded
			}
		}
	}

	for _, child := range s.Children {
		if child.Health == HealthDegraded {
			s.Health = HealthDegraded
		}
	}
}

// Record records a completed attempt without changing its schedule.
// A failed attempt preserves the previous successful attempt's timestamp.
func (s *ActivityStatus) Record(
	at time.Time,
	err error,
) {
	s.LastAttemptAt = at
	if err != nil {
		s.Error = err.Error()
		return
	}
	s.LastSuccessAt = at
	s.Error = ""
}
