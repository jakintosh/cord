package testutil

import (
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
// runtime is constructed but not started, so tests can drive it with
// Converge or start it explicitly. No plane APIs are wired, so no
// running network binds a real TCP listener.
type RuntimeEnv struct {
	Database *database.DB
	Service  *service.Service
	Runtime  *runtime.Runtime
	Manager  *wireguard.Manager
	Backend  *wireguardtest.MockBackend
	Wake     chan string
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
		Service:   env.Service,
		WireGuard: mgr,
		Wake:      env.Wake,
		Clock:     clock,
		Logger:    logging.Discard(),
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	t.Cleanup(rt.Stop)

	return &RuntimeEnv{
		Database: env.Database,
		Service:  env.Service,
		Runtime:  rt,
		Manager:  mgr,
		Backend:  backend,
		Wake:     env.Wake,
	}
}

// Enable records the network as enabled and converges the runtime, so a
// test observes the resulting devices synchronously.
func (e *RuntimeEnv) Enable(
	t *testing.T,
	network string,
) {
	t.Helper()

	if err := e.Service.SetNetworkEnabled(network, true); err != nil {
		t.Fatalf("enable network %s: %v", network, err)
	}
	if err := e.Runtime.Converge(network); err != nil {
		t.Fatalf("converge network %s: %v", network, err)
	}
}

// Converge applies the network's current durable state to its devices.
func (e *RuntimeEnv) Converge(
	t *testing.T,
	network string,
) {
	t.Helper()

	if err := e.Runtime.Converge(network); err != nil {
		t.Fatalf("converge network %s: %v", network, err)
	}
}
