package dns_test

import (
	"context"
	"errors"
	"io/fs"
	"os/exec"
	"reflect"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/dns"
)

type resolvedFixture struct {
	manager   *dns.Manager
	commands  [][]string
	index     int
	lookupErr error
	failCall  int
	runErr    error
	output    string
}

func newResolvedFixture(
	t *testing.T,
) *resolvedFixture {
	t.Helper()
	fixture := &resolvedFixture{index: 12}
	manager, err := dns.New(dns.Options{
		Run: fixture.run,
		LookupInterface: func(string) (int, error) {
			return fixture.index, fixture.lookupErr
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.manager = manager
	return fixture
}

func (f *resolvedFixture) run(
	ctx context.Context,
	name string,
	args ...string,
) (
	[]byte,
	error,
) {
	command := append([]string{name}, args...)
	f.commands = append(f.commands, command)
	if len(f.commands) == f.failCall {
		return []byte(f.output), f.runErr
	}
	return nil, nil
}

func TestApply_ConfiguresOnlyOwnedLinkAndReapplies(
	t *testing.T,
) {
	fixture := newResolvedFixture(t)
	config := testConfig()
	want := [][]string{
		{"resolvectl", "default-route", "12", "no"},
		{"resolvectl", "domain", "12", "example.internal"},
		{"resolvectl", "dns", "12", "10.0.0.1"},
	}

	for range 2 {
		fixture.commands = nil
		if err := fixture.manager.Apply(t.Context(), config); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(fixture.commands, want) {
			t.Fatalf("commands = %v, want %v", fixture.commands, want)
		}
	}

	fixture.commands = nil
	for range 2 {
		if err := fixture.manager.Remove(t.Context(), config); err != nil {
			t.Fatal(err)
		}
	}
	wantRemoval := [][]string{{"resolvectl", "revert", "12"}}
	if !reflect.DeepEqual(fixture.commands, wantRemoval) {
		t.Fatalf("removal = %v, want one revert", fixture.commands)
	}
}

func TestApply_PartialFailureCanRetryOrRemove(
	t *testing.T,
) {
	for _, recovery := range []string{"retry", "remove"} {
		t.Run(recovery, func(t *testing.T) {
			fixture := newResolvedFixture(t)
			fixture.failCall = 3
			fixture.runErr = errors.New("permission denied")
			config := testConfig()

			err := fixture.manager.Apply(t.Context(), config)
			if !errors.Is(err, fixture.runErr) || errors.Is(err, dns.ErrUnsupported) {
				t.Fatalf("partial apply = %v, want operational error", err)
			}

			fixture.commands = nil
			fixture.failCall = 0
			if recovery == "retry" {
				if err := fixture.manager.Apply(t.Context(), config); err != nil {
					t.Fatal(err)
				}
				if len(fixture.commands) != 3 {
					t.Fatalf("retry did not reapply full configuration: %v", fixture.commands)
				}
			} else {
				if err := fixture.manager.Remove(t.Context(), config); err != nil {
					t.Fatal(err)
				}
				want := [][]string{{"resolvectl", "revert", "12"}}
				if !reflect.DeepEqual(fixture.commands, want) {
					t.Fatalf("partial cleanup = %v", fixture.commands)
				}
			}
		})
	}
}

func TestRemove_RetainsFailedCleanup(
	t *testing.T,
) {
	fixture := newResolvedFixture(t)
	config := testConfig()
	if err := fixture.manager.Apply(t.Context(), config); err != nil {
		t.Fatal(err)
	}
	fixture.commands = nil
	fixture.failCall = 1
	fixture.runErr = errors.New("bus temporarily unavailable")

	if err := fixture.manager.Remove(t.Context(), config); !errors.Is(err, fixture.runErr) {
		t.Fatalf("removal = %v, want retained failure", err)
	}
	if err := fixture.manager.Remove(t.Context(), config); err != nil {
		t.Fatal(err)
	}
	if len(fixture.commands) != 2 {
		t.Fatalf("cleanup was not retried: %v", fixture.commands)
	}
}

func TestRemove_DoesNotTouchDisappearedOrReplacedInterface(
	t *testing.T,
) {
	for _, state := range []string{"gone", "replaced"} {
		t.Run(state, func(t *testing.T) {
			fixture := newResolvedFixture(t)
			config := testConfig()
			if err := fixture.manager.Apply(t.Context(), config); err != nil {
				t.Fatal(err)
			}
			fixture.commands = nil
			if state == "gone" {
				fixture.lookupErr = fs.ErrNotExist
			} else {
				fixture.index = 99
			}

			if err := fixture.manager.Remove(t.Context(), config); err != nil {
				t.Fatal(err)
			}
			if len(fixture.commands) != 0 {
				t.Fatalf("touched unowned interface: %v", fixture.commands)
			}
		})
	}
}

func TestApply_RecognizesUnavailableIntegration(
	t *testing.T,
) {
	for _, tc := range []struct {
		name   string
		err    error
		output string
	}{
		{"missing command", exec.ErrNotFound, ""},
		{"missing service", errors.New("exit 1"), "org.freedesktop.resolve1 was not provided"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newResolvedFixture(t)
			fixture.failCall = 1
			fixture.runErr = tc.err
			fixture.output = tc.output
			config := testConfig()

			err := fixture.manager.Apply(t.Context(), config)
			if !errors.Is(err, dns.ErrUnsupported) {
				t.Fatalf("apply = %v, want unsupported integration", err)
			}
			fixture.commands = nil
			if err := fixture.manager.Remove(t.Context(), config); err != nil {
				t.Fatal(err)
			}
			if len(fixture.commands) != 0 {
				t.Fatal("unavailable integration acquired cleanup ownership")
			}
		})
	}
}

func TestApply_BoundsCommandDuration(
	t *testing.T,
) {
	manager, err := dns.New(dns.Options{
		Timeout: time.Millisecond,
		LookupInterface: func(string) (int, error) {
			return 12, nil
		},
		Run: func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Apply(t.Context(), testConfig()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Apply = %v, want deadline", err)
	}
}

func TestCleanupStale_DoesNotInferLinkOwnership(
	t *testing.T,
) {
	fixture := newResolvedFixture(t)
	if err := fixture.manager.CleanupStale(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if err := fixture.manager.Remove(t.Context(), testConfig()); err != nil {
		t.Fatal(err)
	}
	if len(fixture.commands) != 0 {
		t.Fatal("fresh manager reverted a link it never owned")
	}
}

func TestApply_NetworksKeepIndependentLinkOwnership(
	t *testing.T,
) {
	fixture := newResolvedFixture(t)
	alpha := testConfig()
	if err := fixture.manager.Apply(t.Context(), alpha); err != nil {
		t.Fatal(err)
	}
	beta := dns.Config{
		Network:    "beta",
		Domain:     "beta.internal",
		Nameserver: alpha.Nameserver,
		Interface:  "cord-beta",
	}

	// A second domain cannot take over the same link.
	fixture.commands = nil
	if err := fixture.manager.Apply(t.Context(), beta); !errors.Is(err, dns.ErrConflict) {
		t.Fatalf("shared link = %v, want ownership conflict", err)
	}
	if len(fixture.commands) != 0 {
		t.Fatal("conflicting domain changed link configuration")
	}

	fixture.index = 13
	if err := fixture.manager.Apply(t.Context(), beta); err != nil {
		t.Fatal(err)
	}
	fixture.commands = nil
	if err := fixture.manager.Remove(t.Context(), beta); err != nil {
		t.Fatal(err)
	}
	fixture.index = 12
	if err := fixture.manager.Remove(t.Context(), alpha); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"resolvectl", "revert", "13"},
		{"resolvectl", "revert", "12"},
	}
	if !reflect.DeepEqual(fixture.commands, want) {
		t.Fatalf("independent removal = %v, want %v", fixture.commands, want)
	}
}
