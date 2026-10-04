package render_test

import (
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/cli/render"
)

func TestHumanizeRelativeAt(
	t *testing.T,
) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		timestamp string
		want      string
	}{
		{
			name:      "empty",
			timestamp: "",
			want:      "never",
		},
		{
			name:      "invalid",
			timestamp: "not-a-time",
			want:      "not-a-time",
		},
		{
			name:      "now",
			timestamp: now.Format(time.RFC3339),
			want:      "now",
		},
		{
			name:      "seconds ago",
			timestamp: now.Add(-42 * time.Second).Format(time.RFC3339),
			want:      "42s ago",
		},
		{
			name:      "minutes ago",
			timestamp: now.Add(-12 * time.Minute).Format(time.RFC3339),
			want:      "12m ago",
		},
		{
			name:      "hours ago",
			timestamp: now.Add(-3 * time.Hour).Format(time.RFC3339),
			want:      "3h ago",
		},
		{
			name:      "days ago",
			timestamp: now.Add(-48 * time.Hour).Format(time.RFC3339),
			want:      "2d ago",
		},
		{
			name:      "future",
			timestamp: now.Add(90 * time.Second).Format(time.RFC3339),
			want:      "in 1m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := render.RelativeTime(tt.timestamp, now); got != tt.want {
				t.Fatalf("render.RelativeTime(%q) = %q, want %q", tt.timestamp, got, tt.want)
			}
		})
	}
}
