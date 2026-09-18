package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/database"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

func TestNetworkConstructionAndPartialStartupCleanup(t *testing.T) {
	db, err := database.Open(database.Options{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc, err := service.New(service.Options{Store: db})
	if err != nil {
		t.Fatal(err)
	}
	key, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := wireguard.PublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	backend := wireguardtest.NewMockBackend()
	network, err := newNetwork(networkOptions{
		record: &service.Network{
			Name:          "test",
			InterfaceName: "test",
			PrivateKey:    key,
			AssignedRoute: "10.42.0.2/32",
			Server: service.ServerInfo{
				PublicKey:   publicKey,
				Route:       "10.42.0.1/32",
				NetworkCidr: "10.42.0.0/24",
				APIPort:     8080,
			},
		},
		service:        svc,
		wireguard:      wireguard.NewManagerWithBackend(backend),
		clock:          time.Now,
		log:            logging.Discard(),
		syncInterval:   time.Hour,
		scanInterval:   time.Hour,
		reportInterval: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer network.stop()
	if len(backend.CreateCalls) != 0 || network.ctx != nil {
		t.Fatal("construction started the network")
	}
	if err := network.stop(); err != nil {
		t.Fatalf("stop before startup: %v", err)
	}

	// The record is absent from storage, so applying cached peers fails after
	// the interface starts but before any activities can be scheduled.
	if err := network.start(t.Context()); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("start = %v, want missing network", err)
	}
	if !network.nextSync.IsZero() || !network.nextScan.IsZero() || !network.nextReport.IsZero() {
		t.Fatal("failed startup scheduled activities")
	}
	if network.reconcileStatus.Error == "" {
		t.Fatal("failed startup did not record its reconciliation error")
	}
	if err := network.stop(); err != nil {
		t.Fatalf("stop after failed startup: %v", err)
	}
	device := backend.Device("test")
	if device == nil || device.CloseCalls != 1 {
		t.Fatal("partial startup did not release its interface exactly once")
	}
}

func TestNetworkStopRetiresScheduledWork(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	network := &Network{
		ctx:        ctx,
		cancel:     cancel,
		nextSync:   time.Now(),
		nextScan:   time.Now(),
		nextReport: time.Now(),
		tunnel:     &Tunnel{},
	}
	if err := network.stop(); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() == nil || !network.nextActivity().IsZero() {
		t.Fatal("stopped network still has an active lifetime or scheduled work")
	}
	// Retired work must not access the absent service or device.
	network.runDueActivities()
}
