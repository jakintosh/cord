package topology

import (
	"strings"
	"testing"
)

func makeCidr(
	name string,
	cidr string,
	terminal bool,
) Cidr {
	result, err := CidrFromString(name, cidr, terminal)
	if err != nil {
		panic(err)
	}
	return result
}

func TestNew_RejectsInvalidSnapshotReferences(t *testing.T) {
	tests := []struct {
		name     string
		snapshot *Snapshot
		want     string
	}{
		{
			name: "unknown assignment CIDR",
			snapshot: &Snapshot{
				Assignments: map[string][]string{"missing": {"group"}},
			},
			want: "assignment CIDR",
		},
		{
			name: "unknown peer CIDR",
			snapshot: &Snapshot{
				PeerCidr: map[string]string{"peer": "missing"},
				PeerInfo: map[string]Peer{"peer": {Name: "peer"}},
			},
			want: "CIDR \"missing\" not found",
		},
		{
			name: "missing peer information",
			snapshot: &Snapshot{
				Cidrs:    []Cidr{makeCidr("peer-cidr", "10.0.0.1/32", true)},
				PeerCidr: map[string]string{"peer": "peer-cidr"},
			},
			want: "information not found",
		},
		{
			name: "peer name mismatch",
			snapshot: &Snapshot{
				Cidrs:    []Cidr{makeCidr("peer-cidr", "10.0.0.1/32", true)},
				PeerCidr: map[string]string{"peer": "peer-cidr"},
				PeerInfo: map[string]Peer{"peer": {Name: "other"}},
			},
			want: "has name \"other\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.snapshot)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("New() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestNew_RejectsNilSnapshot(t *testing.T) {
	_, err := New(nil)
	if err == nil || !strings.Contains(err.Error(), "snapshot is required") {
		t.Fatalf("New() error = %v, want required snapshot", err)
	}
}
