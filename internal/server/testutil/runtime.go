package testutil

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

// RuntimeEnv is a full server environment: a service over an in-memory
// database plus a runtime backed by the mock WireGuard backend. The
// runtime is active. Tests apply intent through public commands.
// No plane APIs are wired, so no
// running network binds a real TCP listener.
type RuntimeEnv struct {
	Database *database.DB
	Service  *service.Service
	Runtime  *runtime.Runtime
	Manager  *wireguard.Manager
	Backend  *wireguardtest.MockBackend
	Stop     func() error
}

func SetupRuntime(
	t *testing.T,
) *RuntimeEnv {
	t.Helper()
	return SetupRuntimeWithClock(t, func() time.Time { return FixedTime })
}

func SetupRuntimeWithClock(
	t *testing.T,
	clock func() time.Time,
) *RuntimeEnv {
	t.Helper()

	env := SetupServiceWithClock(t, clock)

	backend := wireguardtest.NewMockBackend()
	mgr := wireguard.NewManagerWithBackend(backend)

	rt, err := runtime.New(runtime.Options{
		Service:     env.Service,
		WireGuard:   mgr,
		Clock:       clock,
		Logger:      logging.Discard(),
		DNSDisabled: true,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	stop := RunRuntime(t, rt, runtime.Handlers{})

	return &RuntimeEnv{
		Database: env.Database,
		Service:  env.Service,
		Runtime:  rt,
		Manager:  mgr,
		Backend:  backend,
		Stop:     stop,
	}
}

// Enable records the network as enabled and reconciles the runtime, so a
// test observes the resulting devices synchronously.
func (e *RuntimeEnv) Enable(
	t *testing.T,
	network string,
) {
	t.Helper()

	if err := e.Service.SetNetworkEnabled(network, true); err != nil {
		t.Fatalf("enable network %s: %v", network, err)
	}
	if err := ApplyNetwork(t, e.Runtime, network); err != nil {
		t.Fatalf("reconcile network %s: %v", network, err)
	}
}

// ReconcileNetwork applies the network's current durable state to its devices.
func (e *RuntimeEnv) ReconcileNetwork(
	t *testing.T,
	network string,
) {
	t.Helper()

	if err := ApplyNetwork(t, e.Runtime, network); err != nil {
		t.Fatalf("reconcile network %s: %v", network, err)
	}
}

// ApplyNetwork repeats the network intent command and returns its observed application failure.
func ApplyNetwork(t *testing.T, rt *runtime.Runtime, name string) error {
	t.Helper()
	record, err := rt.GetNetwork(t.Context(), name)
	if err != nil {
		return err
	}
	overview, err := rt.SetNetworkEnabled(t.Context(), name, record.Enabled)
	if err != nil {
		return err
	}
	if overview.Reason != "" {
		return errors.New(overview.Reason)
	}
	return nil
}

// RunRuntime runs a single lifetime and joins it before test dependencies close.
// The first status command waits for the initial maintenance pass to finish.
func RunRuntime(
	t *testing.T,
	rt *runtime.Runtime,
	handlers runtime.Handlers,
) func() error {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	var runErr error
	go func() {
		defer close(done)
		runErr = rt.Run(ctx, handlers)
	}()

	stop := func() error {
		cancel()
		<-done
		return runErr
	}
	t.Cleanup(func() { _ = stop() })
	if _, err := rt.Status(t.Context()); err != nil {
		t.Fatalf("run runtime: %v", err)
	}
	return stop
}

// RuntimeStatus reads an owned-state snapshot through the public command API.
func RuntimeStatus(
	t *testing.T,
	rt *runtime.Runtime,
) runtime.RuntimeStatus {
	t.Helper()
	status, err := rt.Status(t.Context())
	if err != nil {
		t.Fatalf("runtime status: %v", err)
	}
	return status
}
