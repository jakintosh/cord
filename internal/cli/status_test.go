package cli_test

import (
	"bytes"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/cli"
	client "git.studiopollinator.com/pollinator/cord/pkg/admin/client"
	server "git.studiopollinator.com/pollinator/cord/pkg/admin/server"
)

func TestClientStatus_DNSOutcomes(
	t *testing.T,
) {
	for _, tc := range []struct{ name, state, target, health, reason, detail, want string }{
		{
			"applied",
			"running",
			"running",
			"healthy",
			"",
			"",
			"◉ healthy running",
		},
		{
			"unsupported",
			"failed",
			"running",
			"degraded",
			"unsupported",
			"resolvectl unavailable",
			"◉ degraded failed → running",
		},
		{
			"operational failure",
			"failed",
			"running",
			"degraded",
			"",
			"permission denied",
			"◉ degraded failed → running",
		},
		{
			"uninstalled cleanup",
			"failed",
			"stopped",
			"degraded",
			"",
			"permission denied",
			"◉ degraded failed → stopped",
		},
		{
			"unsupported cleanup",
			"failed",
			"stopped",
			"degraded",
			"unsupported",
			"resolvectl unavailable",
			"◉ degraded failed → stopped",
		},
		{
			"inactive",
			"stopped",
			"stopped",
			"inactive",
			"",
			"",
			"◯ inactive stopped",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// No saved record: ownership alone must keep this resolver visible.
			s := client.Status{
				Snapshot: client.ResourceSnapshot{
					Children: []client.ResourceSnapshot{{
						Kind: "network",
						Name: "office",
						Children: []client.ResourceSnapshot{{
							Kind:   "dns-resolver",
							Name:   "office.internal",
							State:  tc.state,
							Target: tc.target,
							Health: tc.health,
							Reason: tc.reason,
							Error:  tc.detail,
						}},
					}},
				},
			}
			var out bytes.Buffer
			if err := cli.New(&out, cli.Options{}).ClientStatus(s); err != nil {
				t.Fatal(err)
			}
			got := strings.Join(strings.Fields(out.String()), " ")
			for _, want := range []string{"office.internal (dns-resolver) " + tc.want, tc.reason, tc.detail} {
				if !strings.Contains(got, want) {
					t.Fatalf("output = %s, want %q", got, want)
				}
			}
		})
	}
}

func TestClientStatus_SavedIntentAndActivityResults(
	t *testing.T,
) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	stamp := func(d time.Duration) *string { s := now.Add(d).Format(time.RFC3339); return &s }
	s := client.Status{
		Version: "dev",
		Health:  "degraded",
		Snapshot: client.ResourceSnapshot{
			Kind:   "runtime",
			State:  "running",
			Target: "running",
			Health: "degraded",
			Activities: map[string]client.ActivityStatus{"dns-cleanup": {
				Error: "resolver directory\nunavailable",
			}},
			Children: []client.ResourceSnapshot{{
				Kind:   "network",
				Name:   "office",
				State:  "running",
				Target: "running",
				Health: "healthy",
				Activities: map[string]client.ActivityStatus{
					"sync": {
						LastAttemptAt:   stamp(-15 * time.Second),
						LastSuccessAt:   stamp(-135 * time.Second),
						Error:           "timeout",
						IntervalSeconds: 120,
						NextAttemptAt:   stamp(105 * time.Second),
					},
					"report": {}, "scan": {
						LastAttemptAt: stamp(-5 * time.Second),
						LastSuccessAt: stamp(-5 * time.Second),
					},
					"close": {},
				},
			}},
		},
		Networks: []client.NetworkStatus{
			{
				Name:   "disabled",
				Health: "inactive",
			},
			{
				Name:    "failed",
				Enabled: true,
				Health:  "degraded",
				Reason:  "start failed: permission denied",
			},
			{
				Name:    "office",
				Enabled: true,
				Health:  "degraded",
			},
		},
		Installs: []client.InstallStatus{{
			Name:  "lab",
			State: "invited",
		}},
	}
	for _, verbose := range []bool{false, true} {
		var out bytes.Buffer
		if err := cli.New(&out, cli.Options{
			Clock:   func() time.Time { return now },
			Verbose: verbose,
		}).ClientStatus(s); err != nil {
			t.Fatal(err)
		}
		got := strings.Join(strings.Fields(out.String()), " ")
		for _, want := range []string{
			"disabled (network) ◯ inactive —", "failed (network) ◉ degraded —",
			"office (network) ◉ degraded running", "↻ sync ◉ error 2m 15s ago",
			"↻ report ◯ not run never", "↻ scan ◉ ok 5s ago",
			"client / dns-cleanup: resolver directory; unavailable",
			"client / failed: start failed: permission denied",
			"client / office / sync: timeout (last attempt 15s ago)", "NAME PHASE lab invited",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("verbose=%v: missing %q in\n%s", verbose, want, out.String())
			}
		}
		if strings.Contains(got, "↻ close") != verbose {
			t.Error("housekeeping visibility differs from verbosity")
		}
		if verbose && !strings.Contains(got, "15s ago 2m in 1m 45s") {
			t.Error("missing explicit schedule")
		}
		if strings.Count(got, "office (network)") != 1 {
			t.Error("saved and owned network duplicated")
		}
	}
	if s.Snapshot.Children[0].Health != "healthy" {
		t.Fatal("render mutated response")
	}
}

func TestServerStatus_RetainedCleanup(
	t *testing.T,
) {
	s := server.Status{
		Health: "degraded",
		Snapshot: server.ResourceSnapshot{
			Kind:   "runtime",
			State:  "running",
			Target: "running",
			Children: []server.ResourceSnapshot{{
				Kind:   "network",
				Name:   "office",
				State:  "failed",
				Target: "stopped",
				Health: "degraded",
				Activities: map[string]server.ActivityStatus{"reconcile": {
					Error: "old failure",
				}, "close": {
					Error: "device close failed",
				}},
				Children: []server.ResourceSnapshot{{
					Kind:   "plane",
					Name:   "main",
					State:  "failed",
					Target: "stopped",
					Health: "degraded",
					Children: []server.ResourceSnapshot{{
						Kind:   "wireguard-device",
						State:  "failed",
						Target: "stopped",
						Health: "degraded",
						Error:  "device close failed\npermission denied",
					}},
				}},
			}},
		},
	}
	var out bytes.Buffer
	if err := cli.New(&out, cli.Options{}).ServerStatus(s); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(strings.Fields(out.String()), " ")
	for _, want := range []string{"office (network) ◉ degraded failed → stopped", "↻ close ◉ error never", "server / office / main / wireguard-device: device close failed; permission denied"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, "old failure") {
		t.Fatal("retired reconciliation obscures cleanup")
	}
}

func TestServerStatus_TreeLayout(
	t *testing.T,
) {
	s := server.Status{
		Version: "dev",
		Health:  "healthy",
		Snapshot: server.ResourceSnapshot{
			Kind:       "runtime",
			State:      "running",
			Target:     "running",
			Activities: map[string]server.ActivityStatus{"reconcile": {}},
			Children: []server.ResourceSnapshot{{
				Kind:   "network",
				Name:   "example",
				State:  "running",
				Target: "running",
				Health: "healthy",
				Children: []server.ResourceSnapshot{{
					Kind:   "plane",
					Name:   "main",
					State:  "running",
					Target: "running",
					Health: "healthy",
					Children: []server.ResourceSnapshot{{
						Kind:   "http",
						State:  "running",
						Target: "running",
						Health: "healthy",
					}},
				}},
			}},
		},
		Networks: []server.NetworkStatus{{
			Name:   "staging",
			Health: "inactive",
		}, {
			Name:    "example",
			Enabled: true,
			Health:  "healthy",
		}},
	}
	var out bytes.Buffer
	if err := cli.New(&out, cli.Options{}).ServerStatus(s); err != nil {
		t.Fatal(err)
	}
	want := `server status
-------------
version: dev

RESOURCE              STATUS      STATE    LAST SUCCESS
server                ◉ healthy   running
├─ ↻ reconcile        ◯ not run            never
├─ example (network)  ◉ healthy   running
│  ├─ ↻ reconcile     ◯ not run            never
│  └─ main (plane)    ◉ healthy   running
│     └─ http         ◉ healthy   running
└─ staging (network)  ◯ inactive  —
`
	if out.String() != want {
		t.Fatalf("output differs:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestServerStatus_StyleAndSchedule(
	t *testing.T,
) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	stamp := now.Add(-8 * time.Second).Format(time.RFC3339)
	s := server.Status{
		Version: "dev",
		Health:  "degraded",
		Snapshot: server.ResourceSnapshot{
			Kind:   "runtime",
			State:  "running",
			Target: "running",
			Error:  "read saved state: unavailable",
			Activities: map[string]server.ActivityStatus{"reconcile": {
				IntervalSeconds: 30,
				LastAttemptAt:   &stamp,
				LastSuccessAt:   &stamp,
			}},
			Children: []server.ResourceSnapshot{{
				Kind:   "network",
				Name:   "retained",
				Health: "degraded",
				State:  "failed",
				Target: "stopped",
			}},
		},
	}
	var plain, styled bytes.Buffer
	for _, c := range []struct {
		out   *bytes.Buffer
		color bool
	}{{
		&plain,
		false,
	}, {
		&styled,
		true,
	}} {
		if err := cli.New(c.out, cli.Options{
			Clock:   func() time.Time { return now },
			Verbose: true,
			Style:   c.color,
		}).ServerStatus(s); err != nil {
			t.Fatal(err)
		}
	}
	ansi := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	if ansi.ReplaceAllString(styled.String(), "") != plain.String() {
		t.Fatal("color changes layout")
	}
	if !strings.Contains(styled.String(), "\x1b[33m◉\x1b[39m degraded") {
		t.Fatal("missing amber health indicator")
	}
	got := strings.Join(strings.Fields(plain.String()), " ")
	for _, want := range []string{"8s ago 8s ago 30s —", "server: read saved state: unavailable", "retained (network) ◉ degraded failed → stopped"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestServerStatus_PropagatesWriterError(
	t *testing.T,
) {
	if err := cli.New(failingWriter{}, cli.Options{}).ServerStatus(server.Status{}); !errors.Is(err, errWrite) {
		t.Fatal("write failure ignored")
	}
}
