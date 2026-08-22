package testutil

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/api"
	"git.studiopollinator.com/pollinator/cord/internal/client/database"
	"git.studiopollinator.com/pollinator/cord/internal/client/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

var FixedTime = time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)

type APIEnv struct {
	Database *database.DB
	Service  *service.Service
	Runtime  *runtime.Runtime
	Backend  *wireguardtest.MockBackend
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

	env := SetupRuntimeWithServer(t, handlerFactory)

	apiServer, err := api.New(api.Options{
		Service: env.Service,
		Runtime: env.Runtime,
		Logger:  logging.Discard(),
	})
	if err != nil {
		t.Fatalf("new api: %v", err)
	}

	return &APIEnv{
		Database: env.Database,
		Service:  env.Service,
		Runtime:  env.Runtime,
		Backend:  env.Backend,
		Router:   apiServer.Router(),
		Server:   env.Server,
	}
}

func (e *APIEnv) SeedNetwork(
	t *testing.T,
	name string,
) *service.Network {
	return SeedNetworkDirect(t, e.Database, name)
}

func (e *APIEnv) SeedInstall(
	t *testing.T,
	name string,
) *service.Install {
	return SeedInstall(t, e.Service, name)
}

// SeedEnabledNetwork seeds a network, records it as enabled, and
// converges the runtime, so its device exists before the test runs.
func (e *APIEnv) SeedEnabledNetwork(
	t *testing.T,
	name string,
) *service.Network {
	t.Helper()

	network := e.SeedNetwork(t, name)
	if err := e.Service.SetNetworkEnabled(name, true); err != nil {
		t.Fatalf("enable network %q: %v", name, err)
	}
	if err := e.Runtime.Converge(name); err != nil {
		t.Fatalf("converge network %q: %v", name, err)
	}
	network.Enabled = true
	return network
}

// NewInstallServer creates a handler answering every call an install and
// its first sync make: /redeem hands back a main network reachable at
// apiAddr, /confirm succeeds, and /snapshot returns an empty view.
func NewInstallServer(apiAddr string) http.Handler {
	mux := http.NewServeMux()

	network := servedNetwork("testnet", apiAddr)
	mux.HandleFunc("POST /redeem", func(w http.ResponseWriter, r *http.Request) {
		wire.WriteData(w, http.StatusOK, protocol.Invitation{
			Network: network,
			Peer: protocol.PeerIdentity{
				Route: "10.42.0.5/32",
			},
		})
	})

	mux.HandleFunc("POST /confirm", func(w http.ResponseWriter, r *http.Request) {
		wire.WriteData(w, http.StatusOK, map[string]string{"status": "confirmed"})
	})

	mux.HandleFunc("GET /snapshot", func(w http.ResponseWriter, r *http.Request) {
		wire.WriteData(w, http.StatusOK, NetworkSnapshot())
	})

	return mux
}
