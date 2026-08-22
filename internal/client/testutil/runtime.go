package testutil

import (
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
// runtime is constructed but not started, so tests can drive it with
// Converge or start it explicitly.
type RuntimeEnv struct {
	Database *database.DB
	Service  *service.Service
	Runtime  *runtime.Runtime
	Manager  *wireguard.Manager
	Backend  *wireguardtest.MockBackend
	Server   *httptest.Server
	Wake     chan string
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
	// A running network syncs immediately on start; when no test server
	// backs the tunnel address the call must fail fast instead of
	// waiting out the default dial timeout.
	httpClient := &http.Client{Timeout: 100 * time.Millisecond}
	if handlerFactory != nil {
		server = httptest.NewUnstartedServer(nil)
		server.Config.Handler = handlerFactory(server.Listener.Addr().String())
		server.Start()
		httpClient = server.Client()
	}

	rt, err := runtime.New(runtime.Options{
		Service:    env.Service,
		WireGuard:  mgr,
		HTTPClient: httpClient,
		Wake:       env.Wake,
		Clock:      func() time.Time { return FixedTime },
		Logger:     logging.Discard(),

		SyncInterval:   30 * time.Second,
		ScanInterval:   30 * time.Second,
		ReportInterval: 30 * time.Second,
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
		Server:   server,
		Wake:     env.Wake,
	}
}

// Enable records the network as enabled and converges the runtime, so a
// test observes the resulting device synchronously.
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

// Converge applies the network's current durable state to its device.
func (e *RuntimeEnv) Converge(
	t *testing.T,
	network string,
) {
	t.Helper()

	if err := e.Runtime.Converge(network); err != nil {
		t.Fatalf("converge network %s: %v", network, err)
	}
}
