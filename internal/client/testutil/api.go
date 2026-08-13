package testutil

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/api"
	"git.studiopollinator.com/pollinator/cord/internal/client/database"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

var FixedTime = time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)

type APIEnv struct {
	Database *database.DB
	Service  *service.Service
	Router   http.Handler
	Server   *httptest.Server
}

func Setup(
	t *testing.T,
) *APIEnv {
	t.Helper()
	return SetupWithServer(t, nil)
}

func SetupWithServer(
	t *testing.T,
	handlerFactory func(apiAddr string) http.Handler,
) *APIEnv {
	t.Helper()

	db := SetupDB(t)
	backend := wireguardtest.NewMockBackend()
	mgr := wireguard.NewManagerWithBackend(backend)

	var server *httptest.Server
	// The runtime loop issues an immediate sync on start; when no test
	// server backs the tunnel address the call must fail fast instead of
	// waiting out the default dial timeout.
	httpClient := &http.Client{Timeout: 100 * time.Millisecond}
	if handlerFactory != nil {
		server = httptest.NewUnstartedServer(nil)
		handler := handlerFactory(server.Listener.Addr().String())
		server.Config.Handler = handler
		server.Start()
		httpClient = server.Client()
	}

	svc, err := service.New(service.Options{
		Store:      db,
		WireGuard:  mgr,
		Clock:      func() time.Time { return FixedTime },
		Logger:     logging.Discard(),
		HTTPClient: httpClient,

		SyncInterval:   30 * time.Second,
		ScanInterval:   30 * time.Second,
		ReportInterval: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	apiServer, err := api.New(api.Options{
		Service: svc,
		Logger:  logging.Discard(),
	})
	if err != nil {
		t.Fatalf("new api: %v", err)
	}

	return &APIEnv{
		Database: db,
		Service:  svc,
		Router:   apiServer.Router(),
		Server:   server,
	}
}

func (e *APIEnv) SeedNetwork(
	t *testing.T,
	name string,
) *service.NetworkConfig {
	return SeedNetworkDirect(t, e.Database, name)
}

func (e *APIEnv) SeedInstall(
	t *testing.T,
	name string,
) *service.Install {
	return SeedInstall(t, e.Service, name)
}

func (e *APIEnv) SeedEnabledNetwork(
	t *testing.T,
	name string,
) *service.NetworkConfig {
	t.Helper()

	nc := e.SeedNetwork(t, name)
	if err := e.Service.EnableNetwork(name); err != nil {
		t.Fatalf("enable network %q: %v", name, err)
	}
	return nc
}

// NewInstallServer creates an httptest server handler that responds to
// the invite and peer API endpoints needed by InstallNetwork.
func NewInstallServer(apiAddr string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /redeem", func(w http.ResponseWriter, r *http.Request) {
		wire.WriteData(w, http.StatusOK, protocol.Invitation{
			Network: protocol.NetworkInfo{
				Name:        "testnet",
				PublicKey:   "server-pub-key",
				Endpoint:    "1.2.3.4:51820",
				ServerRoute: "10.42.0.1/32",
				NetworkCidr: "10.42.0.0/16",
				APIPort:     8443,
			},
			Peer: protocol.PeerIdentity{
				Route: "10.42.0.5/32",
			},
		})
	})

	mux.HandleFunc("POST /confirm", func(w http.ResponseWriter, r *http.Request) {
		wire.WriteData(w, http.StatusOK, map[string]string{"status": "confirmed"})
	})

	return mux
}
