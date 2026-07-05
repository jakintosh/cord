package service_test

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/service/serverapi"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
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

// TestBeginInstall_PersistsPermanentKey verifies that BeginInstall
// generates a permanent keypair and persists it in the network record.
func TestBeginInstall_PersistsPermanentKey(t *testing.T) {
	env := testutil.SetupService(t)

	inst, err := env.Service.BeginInstall(service.Invite{
		NetworkName:   "keytest",
		PrivateKey:    "temp-key",
		AssignedRoute: "10.43.0.2/24",
		Server: service.ServerInfo{
			PublicKey: "srv-pub",
			Endpoint:  "1.2.3.4:51821",
			Route:     "10.43.0.1/32",
			APIPort:   8443,
		},
	})
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

	invite := service.Invite{
		NetworkName:   "idempotent",
		PrivateKey:    "temp-key",
		AssignedRoute: "10.43.0.2/24",
		Server: service.ServerInfo{
			PublicKey: "srv-pub",
			Endpoint:  "1.2.3.4:51821",
			Route:     "10.43.0.1/32",
			APIPort:   8443,
		},
	}

	inst1, err := env.Service.BeginInstall(invite)
	if err != nil {
		t.Fatalf("first begin: %v", err)
	}

	inst2, err := env.Service.BeginInstall(invite)
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

	_, err := env.Service.BeginInstall(service.Invite{
		NetworkName:   "already-here",
		PrivateKey:    "temp-key",
		AssignedRoute: "10.43.0.2/24",
		Server: service.ServerInfo{
			PublicKey: "srv-pub",
			Endpoint:  "1.2.3.4:51821",
			Route:     "10.43.0.1/32",
			APIPort:   8443,
		},
	})
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
		var req serverapi.RedeemInvitationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			wire.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		redeemPubKey = req.PermPubKey
		srvHost, srvPortStr, _ := net.SplitHostPort(r.Host)
		srvPort, _ := strconv.Atoi(srvPortStr)
		wire.WriteData(w, http.StatusOK, serverapi.InvitationDTO{
			Network: serverapi.NetworkInfoDTO{
				PublicKey:   srvPubKey,
				Endpoint:    "1.2.3.4:51820",
				ServerRoute: srvHost + "/32",
				APIPort:     uint16(srvPort),
			},
			Peer: serverapi.PeerIdentityDTO{
				Route: "10.42.0.5/32",
			},
		})
	})
	mux.HandleFunc("POST /confirm", func(w http.ResponseWriter, r *http.Request) {
		wire.WriteData(w, http.StatusOK, map[string]string{"status": "confirmed"})
	})
	mux.HandleFunc("GET /peers", func(w http.ResponseWriter, r *http.Request) {
		wire.WriteData(w, http.StatusOK, []serverapi.VisiblePeerDTO{})
	})

	env := testutil.SetupServiceWithServer(t, mux)

	srvHost, srvPortStr, _ := net.SplitHostPort(env.Server.Listener.Addr().String())
	srvPort, _ := strconv.Atoi(srvPortStr)

	invite := service.Invite{
		NetworkName:   "resume-test",
		PrivateKey:    mustGenKey(t),
		AssignedRoute: "10.43.0.2/24",
		Server: service.ServerInfo{
			PublicKey: srvPubKey,
			Endpoint:  "5.6.7.8:51821",
			Route:     srvHost + "/32",
			APIPort:   uint16(srvPort),
		},
	}

	inst, err := env.Service.BeginInstall(invite)
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}

	nc, err := env.Service.InstallNetwork(invite)
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
		wire.WriteData(w, http.StatusOK, serverapi.InvitationDTO{
			Network: serverapi.NetworkInfoDTO{
				PublicKey:   srvPubKey,
				Endpoint:    "1.2.3.4:51820",
				ServerRoute: srvHost + "/32",
				APIPort:     uint16(srvPort),
			},
			Peer: serverapi.PeerIdentityDTO{
				Route: "10.42.0.5/32",
			},
		})
	})
	mux.HandleFunc("POST /confirm", func(w http.ResponseWriter, r *http.Request) {
		confirmCalled = true
		wire.WriteData(w, http.StatusOK, map[string]string{"status": "confirmed"})
	})
	mux.HandleFunc("GET /peers", func(w http.ResponseWriter, r *http.Request) {
		wire.WriteData(w, http.StatusOK, []serverapi.VisiblePeerDTO{})
	})

	env := testutil.SetupServiceWithServer(t, mux)

	srvHost, srvPortStr, _ := net.SplitHostPort(env.Server.Listener.Addr().String())
	srvPort, _ := strconv.Atoi(srvPortStr)

	invite := service.Invite{
		NetworkName:   "res-redeemed",
		PrivateKey:    mustGenKey(t),
		AssignedRoute: "10.43.0.2/24",
		Server: service.ServerInfo{
			PublicKey: srvPubKey,
			Endpoint:  "5.6.7.8:51821",
			Route:     srvHost + "/32",
			APIPort:   uint16(srvPort),
		},
	}

	inst, err := env.Service.BeginInstall(invite)
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}

	if _, err := env.Service.Redeem(inst.Name); err != nil {
		t.Fatalf("manual redeem: %v", err)
	}

	nc, err := env.Service.InstallNetwork(invite)
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
		wire.WriteData(w, http.StatusOK, serverapi.InvitationDTO{
			Network: serverapi.NetworkInfoDTO{
				PublicKey:   srvPubKey,
				Endpoint:    "1.2.3.4:51820",
				ServerRoute: srvHost + "/32",
				APIPort:     uint16(srvPort),
			},
			Peer: serverapi.PeerIdentityDTO{
				Route: "10.42.0.5/32",
			},
		})
	})

	env := testutil.SetupServiceWithServer(t, mux)

	srvHost, srvPortStr, _ := net.SplitHostPort(env.Server.Listener.Addr().String())
	srvPort, _ := strconv.Atoi(srvPortStr)

	invite := service.Invite{
		NetworkName:   "redeemed-idem",
		PrivateKey:    mustGenKey(t),
		AssignedRoute: "10.43.0.2/24",
		Server: service.ServerInfo{
			PublicKey: srvPubKey,
			Endpoint:  "5.6.7.8:51821",
			Route:     srvHost + "/32",
			APIPort:   uint16(srvPort),
		},
	}

	inst, err := env.Service.BeginInstall(invite)
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}
	if _, err := env.Service.Redeem(inst.Name); err != nil {
		t.Fatalf("redeem: %v", err)
	}

	again, err := env.Service.BeginInstall(invite)
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
		wire.WriteData(w, http.StatusOK, serverapi.InvitationDTO{
			Network: serverapi.NetworkInfoDTO{
				PublicKey:   srvPubKey,
				Endpoint:    "1.2.3.4:51820",
				ServerRoute: srvHost + "/32",
				APIPort:     uint16(srvPort),
			},
			Peer: serverapi.PeerIdentityDTO{
				Route: "10.42.0.5/32",
			},
		})
	})
	mux.HandleFunc("POST /confirm", func(w http.ResponseWriter, r *http.Request) {
		wire.WriteData(w, http.StatusOK, map[string]string{"status": "confirmed"})
	})
	mux.HandleFunc("GET /peers", func(w http.ResponseWriter, r *http.Request) {
		wire.WriteData(w, http.StatusOK, []serverapi.VisiblePeerDTO{})
	})

	env := testutil.SetupServiceWithServer(t, mux)

	srvHost, srvPortStr, _ := net.SplitHostPort(env.Server.Listener.Addr().String())
	srvPort, _ := strconv.Atoi(srvPortStr)

	invite := service.Invite{
		NetworkName:   "clear-test",
		PrivateKey:    mustGenKey(t),
		AssignedRoute: "10.43.0.2/24",
		Server: service.ServerInfo{
			PublicKey: srvPubKey,
			Endpoint:  "5.6.7.8:51821",
			Route:     srvHost + "/32",
			APIPort:   uint16(srvPort),
		},
	}

	nc, err := env.Service.InstallNetwork(invite)
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

	_, err := env.Service.BeginInstall(service.Invite{
		NetworkName:   "not-confirmed",
		PrivateKey:    "temp-key",
		AssignedRoute: "10.43.0.2/24",
		Server: service.ServerInfo{
			PublicKey: "srv-pub",
			Endpoint:  "1.2.3.4:51821",
			Route:     "10.43.0.1/32",
			APIPort:   8443,
		},
	})
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}

	err = env.Service.EnableNetwork("not-confirmed")
	if err == nil {
		t.Fatal("expected error enabling unconfirmed network")
	}
}
