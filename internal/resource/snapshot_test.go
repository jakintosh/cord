package resource_test

import (
	"errors"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/resource"
)

func TestActivityStatus_RecordPreservesScheduleAndLastSuccess(
	t *testing.T,
) {
	start := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	due := start.Add(time.Hour)
	status := resource.ActivityStatus{Interval: time.Hour, NextAttemptAt: due}
	status.Record(start, nil)
	failedAt := start.Add(time.Minute)
	status.Record(failedAt, errors.New("attempt failed"))
	if status.LastAttemptAt != failedAt || status.LastSuccessAt != start || status.Error != "attempt failed" {
		t.Fatalf("failed attempt = %+v", status)
	}

	recoveredAt := start.Add(2 * time.Minute)
	status.Record(recoveredAt, nil)
	if status.LastAttemptAt != recoveredAt || status.LastSuccessAt != recoveredAt || status.Error != "" {
		t.Fatalf("recovered attempt = %+v", status)
	}
	if status.Interval != time.Hour || status.NextAttemptAt != due {
		t.Fatalf("recording changed the owner's schedule: %+v", status)
	}
}

func TestSnapshot_UpdateHealth(
	t *testing.T,
) {
	tests := []struct {
		name     string
		snapshot resource.Snapshot
		want     resource.Health
	}{
		{
			name:     "running",
			snapshot: resource.Snapshot{State: resource.Running, Target: resource.Running},
			want:     resource.HealthHealthy,
		},
		{
			name:     "stopped intentionally",
			snapshot: resource.Snapshot{State: resource.Stopped, Target: resource.Stopped},
			want:     resource.HealthInactive,
		},
		{
			name:     "stopped unexpectedly",
			snapshot: resource.Snapshot{State: resource.Stopped, Target: resource.Running},
			want:     resource.HealthDegraded,
		},
		{
			name:     "still running during retirement",
			snapshot: resource.Snapshot{State: resource.Running, Target: resource.Stopped},
			want:     resource.HealthDegraded,
		},
		{
			name:     "failed cleanup",
			snapshot: resource.Snapshot{State: resource.Failed, Target: resource.Stopped},
			want:     resource.HealthDegraded,
		},
		{
			name:     "cleanup error despite stopped state",
			snapshot: resource.Snapshot{State: resource.Stopped, Target: resource.Stopped, Error: "cleanup failed"},
			want:     resource.HealthDegraded,
		},
		{
			name: "failed child during retirement",
			snapshot: resource.Snapshot{
				State: resource.Stopped, Target: resource.Stopped,
				Children: []resource.Snapshot{{Health: resource.HealthDegraded}},
			},
			want: resource.HealthDegraded,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.snapshot.UpdateHealth()
			if tc.snapshot.Health != tc.want {
				t.Fatalf("health = %q, want %q", tc.snapshot.Health, tc.want)
			}
		})
	}
}

func TestSnapshot_ActivityNamesDoNotDetermineHealth(
	t *testing.T,
) {
	for _, name := range []string{"sync", "close", "arbitrary-name"} {
		t.Run(name, func(t *testing.T) {
			status := resource.Snapshot{
				State:      resource.Running,
				Target:     resource.Running,
				Activities: map[string]resource.ActivityStatus{name: {Error: "previous attempt failed"}},
			}
			status.UpdateHealth()
			if status.Health != resource.HealthDegraded {
				t.Fatal("active resource ignored an activity failure")
			}

			// Successful retirement makes all historical activity failures irrelevant.
			status.State = resource.Stopped
			status.Target = resource.Stopped
			status.UpdateHealth()
			if status.Health != resource.HealthInactive {
				t.Fatal("activity history prevented successful retirement")
			}
		})
	}
}
