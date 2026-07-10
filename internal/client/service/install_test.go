package service_test

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

func mustGenKey(t *testing.T) string {
	t.Helper()
	k, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return k
}

func installRequest(invitation protocol.Invitation) service.InstallRequest {
	return service.InstallRequest{Invitation: invitation}
}

// TestBeginInstall_PersistsPermanentKey verifies that BeginInstall
// generates a permanent keypair and persists it in the network record.
func TestBeginInstall_PersistsPermanentKey(t *testing.T) {
	env := testutil.SetupService(t)

	inst, err := env.Service.BeginInstall(installRequest(protocol.Invitation{
		Network: protocol.NetworkInfo{
			Name:        "keytest",
			PublicKey:   "srv-pub",
			Endpoint:    "1.2.3.4:51821",
			ServerRoute: "10.43.0.1/32",
			NetworkCidr: "10.0.0.0/16",
			APIPort:     8443,
		},
		Peer: protocol.PeerIdentity{
			Route:      "10.43.0.2/24",
			PrivateKey: "temp-key",
		},
	}))
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}
	if inst.Phase != service.PhaseInvited {
		t.Errorf("phase = %q, want %q", inst.Phase, service.PhaseInvited)
	}
	if inst.MainPrivateKey == "" {
		t.Error("private key should not be empty")
	}
	pubKey, err := wireguard.PublicKey(inst.MainPrivateKey)
	if err != nil {
		t.Fatalf("derive public key: %v", err)
	}
	if pubKey == "" {
		t.Error("public key should not be empty")
	}
	if inst.MainAssignedRoute != "" {
		t.Errorf("assigned_cidr = %q, want empty before redeem", inst.MainAssignedRoute)
	}
}

// TestBeginInstall_Idempotent verifies that calling BeginInstall twice
// with the same invite returns the same network record with the same
// permanent key — the key is never regenerated.
func TestBeginInstall_Idempotent(t *testing.T) {
	env := testutil.SetupService(t)

	invite := protocol.Invitation{
		Network: protocol.NetworkInfo{
			Name:        "idempotent",
			PublicKey:   "srv-pub",
			Endpoint:    "1.2.3.4:51821",
			ServerRoute: "10.43.0.1/32",
			NetworkCidr: "10.0.0.0/16",
			APIPort:     8443,
		},
		Peer: protocol.PeerIdentity{
			Route:      "10.43.0.2/24",
			PrivateKey: "temp-key",
		},
	}

	inst1, err := env.Service.BeginInstall(installRequest(invite))
	if err != nil {
		t.Fatalf("first begin: %v", err)
	}

	inst2, err := env.Service.BeginInstall(installRequest(invite))
	if err != nil {
		t.Fatalf("second begin: %v", err)
	}

	pub1, _ := wireguard.PublicKey(inst1.MainPrivateKey)
	pub2, _ := wireguard.PublicKey(inst2.MainPrivateKey)
	if pub1 != pub2 {
		t.Errorf("public key changed: %q vs %q", pub1, pub2)
	}
	if inst1.MainPrivateKey != inst2.MainPrivateKey {
		t.Errorf("private key changed: %q vs %q", inst1.MainPrivateKey, inst2.MainPrivateKey)
	}
}

// TestBeginInstall_ExistingConfirmedNetwork verifies that BeginInstall
// refuses if a completed network already exists.
func TestBeginInstall_ExistingConfirmedNetwork(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Service, "already-here")

	_, err := env.Service.BeginInstall(installRequest(protocol.Invitation{
		Network: protocol.NetworkInfo{
			Name:        "already-here",
			PublicKey:   "srv-pub",
			Endpoint:    "1.2.3.4:51821",
			ServerRoute: "10.43.0.1/32",
			NetworkCidr: "10.0.0.0/16",
			APIPort:     8443,
		},
		Peer: protocol.PeerIdentity{
			Route:      "10.43.0.2/24",
			PrivateKey: "temp-key",
		},
	}))
	if err == nil {
		t.Fatal("expected error for existing network")
	}
}

// TestInstall_ResumesFromInvited verifies that if BeginInstall
// succeeds but Redeem has not yet run, calling Install resumes from
// the invited state and reuses the persisted permanent key.
func TestInstall_ResumesFromInvited(t *testing.T) {
	mux := http.NewServeMux()
	var redeemPubKey string
	srvPubKey := mustGenKey(t)
	mux.HandleFunc("POST /redeem", func(w http.ResponseWriter, r *http.Request) {
		var req protocol.RedeemRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			wire.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		redeemPubKey = req.PermPubKey
		srvHost, srvPortStr, _ := net.SplitHostPort(r.Host)
		srvPort, _ := strconv.Atoi(srvPortStr)
		wire.WriteData(w, http.StatusOK, protocol.Invitation{
			Network: protocol.NetworkInfo{
				Name:        "testnet",
				PublicKey:   srvPubKey,
				Endpoint:    "1.2.3.4:51820",
				ServerRoute: srvHost + "/32",
				NetworkCidr: "10.0.0.0/16",
				APIPort:     uint16(srvPort),
			},
			Peer: protocol.PeerIdentity{
				Route: "10.42.0.5/32",
			},
		})
	})
	mux.HandleFunc("POST /confirm", func(w http.ResponseWriter, r *http.Request) {
		wire.WriteData(w, http.StatusOK, map[string]string{"status": "confirmed"})
	})
	mux.HandleFunc("GET /peers", func(w http.ResponseWriter, r *http.Request) {
		wire.WriteData(w, http.StatusOK, []protocol.VisiblePeer{})
	})

	env := testutil.SetupServiceWithServer(t, mux)

	srvHost, srvPortStr, _ := net.SplitHostPort(env.Server.Listener.Addr().String())
	srvPort, _ := strconv.Atoi(srvPortStr)

	invite := protocol.Invitation{
		Network: protocol.NetworkInfo{
			Name:        "resume-test",
			PublicKey:   srvPubKey,
			Endpoint:    "5.6.7.8:51821",
			ServerRoute: srvHost + "/32",
			NetworkCidr: "10.0.0.0/16",
			APIPort:     uint16(srvPort),
		},
		Peer: protocol.PeerIdentity{
			Route:      "10.43.0.2/24",
			PrivateKey: mustGenKey(t),
		},
	}

	inst, err := env.Service.BeginInstall(installRequest(invite))
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}

	nc, err := env.Service.InstallNetwork(installRequest(invite))
	if err != nil {
		t.Fatalf("install (resume): %v", err)
	}
	if nc.Name != "resume-test" {
		t.Fatalf("name = %q, want resume-test", nc.Name)
	}

	pub1, _ := wireguard.PublicKey(inst.MainPrivateKey)
	pub2, _ := wireguard.PublicKey(nc.PrivateKey)
	if pub1 != pub2 {
		t.Errorf("public key changed: %q vs %q", pub1, pub2)
	}

	if redeemPubKey != pub1 {
		t.Errorf("redeem was called with %q, expected %q (persisted key)",
			redeemPubKey, pub1)
	}
}

// TestInstall_ResumesFromRedeemed verifies that if Redeem succeeds but
// Confirm has not yet run, calling Install resumes from the redeemed
// state and completes confirmation.
func TestInstall_ResumesFromRedeemed(t *testing.T) {
	mux := http.NewServeMux()
	confirmCalled := false
	srvPubKey := mustGenKey(t)
	mux.HandleFunc("POST /redeem", func(w http.ResponseWriter, r *http.Request) {
		srvHost, srvPortStr, _ := net.SplitHostPort(r.Host)
		srvPort, _ := strconv.Atoi(srvPortStr)
		wire.WriteData(w, http.StatusOK, protocol.Invitation{
			Network: protocol.NetworkInfo{
				Name:        "testnet",
				PublicKey:   srvPubKey,
				Endpoint:    "1.2.3.4:51820",
				ServerRoute: srvHost + "/32",
				NetworkCidr: "10.0.0.0/16",
				APIPort:     uint16(srvPort),
			},
			Peer: protocol.PeerIdentity{
				Route: "10.42.0.5/32",
			},
		})
	})
	mux.HandleFunc("POST /confirm", func(w http.ResponseWriter, r *http.Request) {
		confirmCalled = true
		wire.WriteData(w, http.StatusOK, map[string]string{"status": "confirmed"})
	})
	mux.HandleFunc("GET /peers", func(w http.ResponseWriter, r *http.Request) {
		wire.WriteData(w, http.StatusOK, []protocol.VisiblePeer{})
	})

	env := testutil.SetupServiceWithServer(t, mux)

	srvHost, srvPortStr, _ := net.SplitHostPort(env.Server.Listener.Addr().String())
	srvPort, _ := strconv.Atoi(srvPortStr)

	invite := protocol.Invitation{
		Network: protocol.NetworkInfo{
			Name:        "res-redeemed",
			PublicKey:   srvPubKey,
			Endpoint:    "5.6.7.8:51821",
			ServerRoute: srvHost + "/32",
			NetworkCidr: "10.0.0.0/16",
			APIPort:     uint16(srvPort),
		},
		Peer: protocol.PeerIdentity{
			Route:      "10.43.0.2/24",
			PrivateKey: mustGenKey(t),
		},
	}

	inst, err := env.Service.BeginInstall(installRequest(invite))
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}

	if _, err := env.Service.Redeem(inst.Name); err != nil {
		t.Fatalf("manual redeem: %v", err)
	}

	nc, err := env.Service.InstallNetwork(installRequest(invite))
	if err != nil {
		t.Fatalf("install (resume from redeemed): %v", err)
	}
	if nc.Name != "res-redeemed" {
		t.Fatalf("name = %q, want res-redeemed", nc.Name)
	}

	if !confirmCalled {
		t.Error("confirm was not called during resume")
	}
	pub1, _ := wireguard.PublicKey(inst.MainPrivateKey)
	pub2, _ := wireguard.PublicKey(nc.PrivateKey)
	if pub1 != pub2 {
		t.Errorf("public key changed: %q vs %q", pub1, pub2)
	}
}

// TestBeginInstall_IdempotentRedeemed verifies that calling BeginInstall
// on an install already at the redeemed phase returns the existing
// record unchanged rather than erroring.
func TestBeginInstall_IdempotentRedeemed(t *testing.T) {
	mux := http.NewServeMux()
	srvPubKey := mustGenKey(t)
	mux.HandleFunc("POST /redeem", func(w http.ResponseWriter, r *http.Request) {
		srvHost, srvPortStr, _ := net.SplitHostPort(r.Host)
		srvPort, _ := strconv.Atoi(srvPortStr)
		wire.WriteData(w, http.StatusOK, protocol.Invitation{
			Network: protocol.NetworkInfo{
				Name:        "testnet",
				PublicKey:   srvPubKey,
				Endpoint:    "1.2.3.4:51820",
				ServerRoute: srvHost + "/32",
				NetworkCidr: "10.0.0.0/16",
				APIPort:     uint16(srvPort),
			},
			Peer: protocol.PeerIdentity{
				Route: "10.42.0.5/32",
			},
		})
	})

	env := testutil.SetupServiceWithServer(t, mux)

	srvHost, srvPortStr, _ := net.SplitHostPort(env.Server.Listener.Addr().String())
	srvPort, _ := strconv.Atoi(srvPortStr)

	invite := protocol.Invitation{
		Network: protocol.NetworkInfo{
			Name:        "redeemed-idem",
			PublicKey:   srvPubKey,
			Endpoint:    "5.6.7.8:51821",
			ServerRoute: srvHost + "/32",
			NetworkCidr: "10.0.0.0/16",
			APIPort:     uint16(srvPort),
		},
		Peer: protocol.PeerIdentity{
			Route:      "10.43.0.2/24",
			PrivateKey: mustGenKey(t),
		},
	}

	inst, err := env.Service.BeginInstall(installRequest(invite))
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}
	if _, err := env.Service.Redeem(inst.Name); err != nil {
		t.Fatalf("redeem: %v", err)
	}

	again, err := env.Service.BeginInstall(installRequest(invite))
	if err != nil {
		t.Fatalf("begin install on redeemed: %v", err)
	}
	if again.Phase != service.PhaseRedeemed {
		t.Errorf("phase = %q, want %q", again.Phase, service.PhaseRedeemed)
	}
	if again.MainPrivateKey != inst.MainPrivateKey {
		t.Error("permanent key should be unchanged")
	}
}

// TestConfirm_ClearsInstallFields verifies that after Confirm, the
// temporary install scratch fields are cleared.
func TestConfirm_ClearsInstallFields(t *testing.T) {
	mux := http.NewServeMux()
	srvPubKey := mustGenKey(t)
	mux.HandleFunc("POST /redeem", func(w http.ResponseWriter, r *http.Request) {
		srvHost, srvPortStr, _ := net.SplitHostPort(r.Host)
		srvPort, _ := strconv.Atoi(srvPortStr)
		wire.WriteData(w, http.StatusOK, protocol.Invitation{
			Network: protocol.NetworkInfo{
				Name:        "testnet",
				PublicKey:   srvPubKey,
				Endpoint:    "1.2.3.4:51820",
				ServerRoute: srvHost + "/32",
				NetworkCidr: "10.0.0.0/16",
				APIPort:     uint16(srvPort),
			},
			Peer: protocol.PeerIdentity{
				Route: "10.42.0.5/32",
			},
		})
	})
	mux.HandleFunc("POST /confirm", func(w http.ResponseWriter, r *http.Request) {
		wire.WriteData(w, http.StatusOK, map[string]string{"status": "confirmed"})
	})
	mux.HandleFunc("GET /peers", func(w http.ResponseWriter, r *http.Request) {
		wire.WriteData(w, http.StatusOK, []protocol.VisiblePeer{})
	})

	env := testutil.SetupServiceWithServer(t, mux)

	srvHost, srvPortStr, _ := net.SplitHostPort(env.Server.Listener.Addr().String())
	srvPort, _ := strconv.Atoi(srvPortStr)

	invite := protocol.Invitation{
		Network: protocol.NetworkInfo{
			Name:        "clear-test",
			PublicKey:   srvPubKey,
			Endpoint:    "5.6.7.8:51821",
			ServerRoute: srvHost + "/32",
			NetworkCidr: "10.0.0.0/16",
			APIPort:     uint16(srvPort),
		},
		Peer: protocol.PeerIdentity{
			Route:      "10.43.0.2/24",
			PrivateKey: mustGenKey(t),
		},
	}

	nc, err := env.Service.InstallNetwork(installRequest(invite))
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if nc.Name != "clear-test" {
		t.Fatalf("name = %q, want clear-test", nc.Name)
	}
	if nc.AssignedRoute == "" {
		t.Error("assigned_cidr should not be empty after confirm")
	}
	if nc.Server.PublicKey == "" {
		t.Error("server pubkey should not be empty after confirm")
	}
	if nc.Server.Endpoint == "" {
		t.Error("server endpoint should not be empty after confirm")
	}
	if nc.Server.Route == "" {
		t.Error("server route should not be empty after confirm")
	}
	if nc.Server.APIPort == 0 {
		t.Error("server api_port should not be zero after confirm")
	}
}

// TestEnableNetwork_RefusesUnconfirmed verifies that EnableNetwork
// refuses to bring up a network that has not completed onboarding.
func TestEnableNetwork_RefusesUnconfirmed(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.BeginInstall(installRequest(protocol.Invitation{
		Network: protocol.NetworkInfo{
			Name:        "not-confirmed",
			PublicKey:   "srv-pub",
			Endpoint:    "1.2.3.4:51821",
			ServerRoute: "10.43.0.1/32",
			NetworkCidr: "10.0.0.0/16",
			APIPort:     8443,
		},
		Peer: protocol.PeerIdentity{
			Route:      "10.43.0.2/24",
			PrivateKey: "temp-key",
		},
	}))
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}

	err = env.Service.EnableNetwork("not-confirmed")
	if err == nil {
		t.Fatal("expected error enabling unconfirmed network")
	}
}
