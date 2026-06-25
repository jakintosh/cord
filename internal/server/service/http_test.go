package service_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestStartNetwork_WithAPIFactory(t *testing.T) {
	env := testutil.SetupService(t)

	var factoryCalls []string
	env.Service = newServiceWithFactory(t, env, func(network string) service.APIHandlers {
		factoryCalls = append(factoryCalls, network)
		return service.APIHandlers{
			Main:   http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }),
			Invite: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }),
		}
	})

	testutil.SeedNetwork(t, env.Service)

	ctx := context.Background()
	if err := env.Service.StartNetwork(ctx, "testnet"); err != nil {
		t.Fatalf("start network: %v", err)
	}
	defer env.Service.StopNetwork("testnet")

	if len(factoryCalls) != 1 {
		t.Fatalf("expected 1 factory call, got %d", len(factoryCalls))
	}
	if factoryCalls[0] != "testnet" {
		t.Fatalf("factory called with %q, want testnet", factoryCalls[0])
	}

	peers, err := env.Service.ListPeers("testnet")
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	if len(peers) == 0 {
		t.Fatal("expected peers after network start")
	}
}

func TestStartNetwork_NilAPIFactory(t *testing.T) {
	env := testutil.SetupService(t)
	// env.Svc has no APIFactory (nil by default)

	testutil.SeedNetwork(t, env.Service)

	ctx := context.Background()
	if err := env.Service.StartNetwork(ctx, "testnet"); err != nil {
		t.Fatalf("start network without factory: %v", err)
	}
	defer env.Service.StopNetwork("testnet")
}

func TestStartNetwork_PopulatesHTTPServers(t *testing.T) {
	env := testutil.SetupService(t)

	env.Service = newServiceWithFactory(t, env, func(network string) service.APIHandlers {
		return service.APIHandlers{
			Main:   http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }),
			Invite: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }),
		}
	})

	testutil.SeedNetwork(t, env.Service)

	ctx := context.Background()
	if err := env.Service.StartNetwork(ctx, "testnet"); err != nil {
		t.Fatalf("start network: %v", err)
	}
	defer env.Service.StopNetwork("testnet")

	if err := env.Service.StartNetwork(ctx, "testnet"); err != nil {
		t.Fatalf("start network again: %v", err)
	}
}

func TestStopNetwork_ShutsDownHTTPServers(t *testing.T) {
	env := testutil.SetupService(t)

	env.Service = newServiceWithFactory(t, env, func(network string) service.APIHandlers {
		return service.APIHandlers{
			Main:   http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }),
			Invite: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }),
		}
	})

	testutil.SeedNetwork(t, env.Service)

	ctx := context.Background()
	if err := env.Service.StartNetwork(ctx, "testnet"); err != nil {
		t.Fatalf("start network: %v", err)
	}

	if err := env.Service.StopNetwork("testnet"); err != nil {
		t.Fatalf("stop network: %v", err)
	}

	if err := env.Service.StopNetwork("testnet"); err != nil {
		t.Fatalf("stop network again: %v", err)
	}
}

// newServiceWithFactory creates a new Service with an APIFactory, reusing
// the existing test environment's database and mock WG.
func newServiceWithFactory(
	t *testing.T,
	env *testutil.ServiceEnv,
	factory func(network string) service.APIHandlers,
) *service.Service {
	t.Helper()

	svc, err := service.New(service.Options{
		Store:      env.Database,
		WG:         env.WireGuard,
		Clock:      func() time.Time { return testutil.FixedTime },
		APIFactory: factory,
	})
	if err != nil {
		t.Fatalf("new service with factory: %v", err)
	}
	return svc
}
