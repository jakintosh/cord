package runtime

import (
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
)

// The selector's clock boundaries and catalog tie-breaking are easier to
// specify directly than through asynchronous scans and device handshakes.
func TestNextCandidate_RespectsDwellAndCatalogOrder(
	t *testing.T,
) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		endpoints []service.PeerEndpoint
		want      string
	}{
		{
			name: "empty catalog",
		},
		{
			name: "fresh catalog order",
			endpoints: []service.PeerEndpoint{
				{Endpoint: "first"},
				{Endpoint: "second"},
			},
			want: "first",
		},
		{
			name: "recent attempt blocks fresh candidate",
			endpoints: []service.PeerEndpoint{
				{Endpoint: "attempted", LastAttemptedAt: now.Add(-30 * time.Second)},
				{Endpoint: "fresh"},
			},
		},
		{
			name: "least recently attempted",
			endpoints: []service.PeerEndpoint{
				{Endpoint: "first", LastAttemptedAt: now.Add(-3 * RotateInterval)},
				{Endpoint: "second", LastAttemptedAt: now.Add(-2 * RotateInterval)},
				{Endpoint: "oldest", LastAttemptedAt: now.Add(-5 * RotateInterval)},
			},
			want: "oldest",
		},
		{
			name: "fresh candidate precedes attempted",
			endpoints: []service.PeerEndpoint{
				{Endpoint: "attempted", LastAttemptedAt: now.Add(-2 * RotateInterval)},
				{Endpoint: "fresh"},
			},
			want: "fresh",
		},
		{
			name: "exact dwell boundary",
			endpoints: []service.PeerEndpoint{
				{Endpoint: "only", LastAttemptedAt: now.Add(-RotateInterval)},
			},
			want: "only",
		},
		{
			name: "attempt ties preserve catalog order",
			endpoints: []service.PeerEndpoint{
				{Endpoint: "first", LastAttemptedAt: now.Add(-2 * RotateInterval)},
				{Endpoint: "second", LastAttemptedAt: now.Add(-2 * RotateInterval)},
			},
			want: "first",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate, ok := nextCandidate(tt.endpoints, now)
			if candidate != tt.want || ok != (tt.want != "") {
				t.Fatalf("candidate = %q, %v; want %q, %v", candidate, ok, tt.want, tt.want != "")
			}
		})
	}
}
