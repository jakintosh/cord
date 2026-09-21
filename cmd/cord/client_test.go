package main

import (
	"io"
	"os"
	"strings"
	"testing"

	adminclient "git.studiopollinator.com/pollinator/cord/pkg/admin/client"
)

func TestPrintClientStatus_DNSOutcomes(
	t *testing.T,
) {
	tests := []struct {
		name   string
		state  string
		target string
		reason string
		detail string
		want   string
	}{
		{
			name: "applied", state: "running", target: "running",
			want: "office office.internal applied -",
		},
		{
			name: "unsupported", state: "failed", target: "running",
			reason: "unsupported", detail: "resolvectl unavailable",
			want: "office office.internal unsupported resolvectl unavailable",
		},
		{
			name: "operational failure", state: "failed", target: "running",
			detail: "permission denied",
			want:   "office office.internal degraded permission denied",
		},
		{
			name: "uninstalled cleanup", state: "failed", target: "stopped",
			detail: "permission denied",
			want:   "office office.internal cleanup owed permission denied",
		},
		{
			name: "unsupported cleanup", state: "failed", target: "stopped",
			reason: "unsupported", detail: "resolvectl unavailable",
			want: "office office.internal cleanup owed resolvectl unavailable",
		},
		{
			name: "inactive", state: "stopped", target: "stopped",
			want: "office office.internal inactive -",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolver := adminclient.ResourceSnapshot{
				Kind:   "dns-resolver",
				Name:   "office.internal",
				State:  tc.state,
				Target: tc.target,
				Reason: tc.reason,
				Error:  tc.detail,
			}
			network := adminclient.ResourceSnapshot{
				Kind:     "network",
				Name:     "office",
				Children: []adminclient.ResourceSnapshot{resolver},
			}
			// No saved record: resource ownership alone must keep this visible.
			status := adminclient.Status{
				Snapshot: adminclient.ResourceSnapshot{
					Children: []adminclient.ResourceSnapshot{network},
				},
			}
			output := captureClientStatus(t, status)
			if !strings.Contains(output, tc.want) {
				t.Fatalf("output = %q, want row %q", output, tc.want)
			}
		})
	}
}

func TestPrintClientStatus_ReportsStaleCleanupAndNetworksWithoutResources(
	t *testing.T,
) {
	status := adminclient.Status{
		Networks: []adminclient.NetworkStatus{
			{Name: "disabled"},
			{Name: "failed", Enabled: true},
		},
		Snapshot: adminclient.ResourceSnapshot{
			Activities: map[string]adminclient.ActivityStatus{
				"dns-cleanup": {Error: "resolver directory\nunavailable"},
			},
		},
	}
	output := captureClientStatus(t, status)
	for _, want := range []string{
		"disabled - inactive -",
		"failed - degraded resolver not started",
		"DNS stale cleanup failed: resolver directory; unavailable",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output = %q, want %q", output, want)
		}
	}
}

func captureClientStatus(
	t *testing.T,
	status adminclient.Status,
) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	previous := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = previous }()

	printClientStatus(status)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(strings.Fields(string(output)), " ")
}
