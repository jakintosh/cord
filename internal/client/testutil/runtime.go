package testutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/database"
	"git.studiopollinator.com/pollinator/cord/internal/client/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

// RuntimeEnv is a full client environment: a service over an in-memory
// database plus a runtime backed by the mock WireGuard backend. The
// runtime is active; tests drive changes through public commands.
type RuntimeEnv struct {
	Database *database.DB
	Service  *service.Service
	Runtime  *runtime.Runtime
	Manager  *wireguard.Manager
	Backend  *wireguardtest.MockBackend
	Server   *httptest.Server
	Stop     func() error
}

func SetupRuntime(
	t *testing.T,
) *RuntimeEnv {
	t.Helper()
	return SetupRuntimeWithServer(t, nil)
}

// SetupRuntimeWithServer stands up a server the runtime's tunnels can
// actually reach. The factory receives the test server's address, so an
// invitation can point a network's server route at it.
func SetupRuntimeWithServer(
	t *testing.T,
	handlerFactory func(apiAddr string) http.Handler,
) *RuntimeEnv {
	t.Helper()

	env := SetupService(t)
	backend := wireguardtest.NewMockBackend()
	mgr := wireguard.NewManagerWithBackend(backend)

	var server *httptest.Server
	// A running network syncs immediately on start. Without a test server,
	// keep that sync offline and fail without waiting for a network timeout.
	httpClient := NewOfflineHTTPClient()
	if handlerFactory != nil {
		server = httptest.NewUnstartedServer(nil)
		server.Config.Handler = handlerFactory(server.Listener.Addr().String())
		server.Start()
		t.Cleanup(server.Close)
		httpClient = server.Client()
	}

	rt, err := runtime.New(runtime.Options{
		Service:    env.Service,
		WireGuard:  mgr,
		DNS:        NewDNSManager(t),
		HTTPClient: httpClient,
		Clock:      func() time.Time { return FixedTime },
		Logger:     logging.Discard(),

		SyncInterval:   30 * time.Second,
		ScanInterval:   30 * time.Second,
		ReportInterval: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	stop := RunRuntime(t, rt)
	if _, err := rt.GetStatus(t.Context()); err != nil {
		t.Fatalf("runtime startup: %v", err)
	}

	return &RuntimeEnv{
		Database: env.Database,
		Service:  env.Service,
		Runtime:  rt,
		Manager:  mgr,
		Backend:  backend,
		Server:   server,
		Stop:     stop,
	}
}

// RunRuntime runs a single lifetime and joins it before dependencies close.
// Commands wait for startup; tests can assert the returned shutdown error.
func RunRuntime(
	t *testing.T,
	rt *runtime.Runtime,
) func() error {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	var runErr error
	go func() {
		defer close(done)
		runErr = rt.Run(ctx)
	}()
	stop := func() error {
		cancel()
		select {
		case <-done:
			return runErr
		case <-time.After(5 * time.Second):
			t.Fatal("runtime did not finish shutdown")
			return nil
		}
	}
	t.Cleanup(func() { _ = stop() })
	return stop
}

// Enable records the network as enabled and converges the runtime, so a
// test observes the resulting device synchronously.
func (e *RuntimeEnv) Enable(
	t *testing.T,
	network string,
) {
	t.Helper()

	if _, err := e.Runtime.SetNetworkEnabled(t.Context(), network, true); err != nil {
		t.Fatalf("enable network %s: %v", network, err)
	}
}

// Converge applies the network's current durable state to its device.
func (e *RuntimeEnv) Converge(
	t *testing.T,
	network string,
) {
	t.Helper()

	if err := e.Runtime.Converge(t.Context(), network); err != nil {
		t.Fatalf("converge network %s: %v", network, err)
	}
}
