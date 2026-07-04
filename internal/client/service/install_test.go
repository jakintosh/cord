package service_test

import (
	"context"
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

	nw, err := env.Service.BeginInstall(service.Invite{
		NetworkName:           "keytest",
		TempPeerPrivKey:       "temp-key",
		TempPeerAssignedRoute: "10.43.0.2/24",
		InviteServerPubkey:    "srv-pub",
		InviteServerEndpoint:  "1.2.3.4:51821",
		InviteServerRoute:     "10.43.0.1/32",
		InviteServerPort:      8443,
	})
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}
	if nw.State != service.StateInvited {
		t.Errorf("state = %q, want %q", nw.State, service.StateInvited)
	}
	if nw.PrivateKey == "" {
		t.Error("private key should not be empty")
	}
	if nw.PublicKey == "" {
		t.Error("public key should not be empty")
	}
	if nw.AssignedRoute != "" {
		t.Errorf("assigned_cidr = %q, want empty before redeem", nw.AssignedRoute)
	}
}

// TestBeginInstall_Idempotent verifies that calling BeginInstall twice
// with the same invite returns the same network record with the same
// permanent key — the key is never regenerated.
func TestBeginInstall_Idempotent(t *testing.T) {
	env := testutil.SetupService(t)

	invite := service.Invite{
		NetworkName:           "idempotent",
		TempPeerPrivKey:       "temp-key",
		TempPeerAssignedRoute: "10.43.0.2/24",
		InviteServerPubkey:    "srv-pub",
		InviteServerEndpoint:  "1.2.3.4:51821",
		InviteServerRoute:     "10.43.0.1/32",
		InviteServerPort:      8443,
	}

	nw1, err := env.Service.BeginInstall(invite)
	if err != nil {
		t.Fatalf("first begin: %v", err)
	}

	nw2, err := env.Service.BeginInstall(invite)
	if err != nil {
		t.Fatalf("second begin: %v", err)
	}

	if nw1.PublicKey != nw2.PublicKey {
		t.Errorf("public key changed: %q vs %q", nw1.PublicKey, nw2.PublicKey)
	}
	if nw1.PrivateKey != nw2.PrivateKey {
		t.Errorf("private key changed: %q vs %q", nw1.PrivateKey, nw2.PrivateKey)
	}
}

// TestBeginInstall_ExistingConfirmedNetwork verifies that BeginInstall
// refuses if a network already exists in a non-invited state.
func TestBeginInstall_ExistingConfirmedNetwork(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Service, "already-here")

	_, err := env.Service.BeginInstall(service.Invite{
		NetworkName:           "already-here",
		TempPeerPrivKey:       "temp-key",
		TempPeerAssignedRoute: "10.43.0.2/24",
		InviteServerPubkey:    "srv-pub",
		InviteServerEndpoint:  "1.2.3.4:51821",
		InviteServerRoute:     "10.43.0.1/32",
		InviteServerPort:      8443,
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

	env := testutil.SetupServiceWithServer(t, mux)

	srvHost, srvPortStr, _ := net.SplitHostPort(env.Server.Listener.Addr().String())
	srvPort, _ := strconv.Atoi(srvPortStr)

	invite := service.Invite{
		NetworkName:           "resume-test",
		TempPeerPrivKey:       mustGenKey(t),
		TempPeerAssignedRoute: "10.43.0.2/24",
		InviteServerPubkey:    srvPubKey,
		InviteServerEndpoint:  "5.6.7.8:51821",
		InviteServerRoute:     srvHost + "/32",
		InviteServerPort:      uint16(srvPort),
	}

	nw1, err := env.Service.BeginInstall(invite)
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}

	nw2, err := env.Service.Install(invite)
	if err != nil {
		t.Fatalf("install (resume): %v", err)
	}

	if nw2.State != service.StateConfirmed {
		t.Errorf("state = %q, want %q", nw2.State, service.StateConfirmed)
	}

	if nw1.PublicKey != nw2.PublicKey {
		t.Errorf("public key changed: %q vs %q", nw1.PublicKey, nw2.PublicKey)
	}

	if redeemPubKey != nw1.PublicKey {
		t.Errorf("redeem was called with %q, expected %q (persisted key)",
			redeemPubKey, nw1.PublicKey)
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

	env := testutil.SetupServiceWithServer(t, mux)

	srvHost, srvPortStr, _ := net.SplitHostPort(env.Server.Listener.Addr().String())
	srvPort, _ := strconv.Atoi(srvPortStr)

	invite := service.Invite{
		NetworkName:           "res-redeemed",
		TempPeerPrivKey:       mustGenKey(t),
		TempPeerAssignedRoute: "10.43.0.2/24",
		InviteServerPubkey:    srvPubKey,
		InviteServerEndpoint:  "5.6.7.8:51821",
		InviteServerRoute:     srvHost + "/32",
		InviteServerPort:      uint16(srvPort),
	}

	nw1, err := env.Service.BeginInstall(invite)
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}

	if _, err := env.Service.Redeem(nw1.Name); err != nil {
		t.Fatalf("manual redeem: %v", err)
	}

	nw2, err := env.Service.Install(invite)
	if err != nil {
		t.Fatalf("install (resume from redeemed): %v", err)
	}

	if nw2.State != service.StateConfirmed {
		t.Errorf("state = %q, want %q", nw2.State, service.StateConfirmed)
	}
	if !confirmCalled {
		t.Error("confirm was not called during resume")
	}
	if nw1.PublicKey != nw2.PublicKey {
		t.Errorf("public key changed: %q vs %q", nw1.PublicKey, nw2.PublicKey)
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

	env := testutil.SetupServiceWithServer(t, mux)

	srvHost, srvPortStr, _ := net.SplitHostPort(env.Server.Listener.Addr().String())
	srvPort, _ := strconv.Atoi(srvPortStr)

	invite := service.Invite{
		NetworkName:           "clear-test",
		TempPeerPrivKey:       mustGenKey(t),
		TempPeerAssignedRoute: "10.43.0.2/24",
		InviteServerPubkey:    srvPubKey,
		InviteServerEndpoint:  "5.6.7.8:51821",
		InviteServerRoute:     srvHost + "/32",
		InviteServerPort:      uint16(srvPort),
	}

	nw, err := env.Service.Install(invite)
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	if nw.State != service.StateConfirmed {
		t.Fatalf("state = %q, want confirmed", nw.State)
	}
	if nw.TempPeerPrivKey != "" {
		t.Errorf("temp_priv_key = %q, want empty after confirm", nw.TempPeerPrivKey)
	}
	if nw.TempPeerAssignedRoute != "" {
		t.Errorf("temp_cidr = %q, want empty after confirm", nw.TempPeerAssignedRoute)
	}
	if nw.InviteServerPubkey != "" {
		t.Errorf("invite_server_pubkey = %q, want empty after confirm", nw.InviteServerPubkey)
	}
	if nw.InviteServerEndpoint != "" {
		t.Errorf("invite_server_endpoint = %q, want empty after confirm", nw.InviteServerEndpoint)
	}
	if nw.InviteServerRoute != "" {
		t.Errorf("invite_server_route = %q, want empty after confirm", nw.InviteServerRoute)
	}
	if nw.AssignedRoute == "" {
		t.Error("assigned_cidr should not be empty after confirm")
	}
	if nw.ServerPubkey == "" {
		t.Error("server_pubkey should not be empty after confirm")
	}
}

// TestEnableNetwork_RefusesUnconfirmed verifies that EnableNetwork
// refuses to bring up a network that has not completed onboarding.
func TestEnableNetwork_RefusesUnconfirmed(t *testing.T) {
	env := testutil.SetupService(t)

	_, err := env.Service.BeginInstall(service.Invite{
		NetworkName:           "not-confirmed",
		TempPeerPrivKey:       "temp-key",
		TempPeerAssignedRoute: "10.43.0.2/24",
		InviteServerPubkey:    "srv-pub",
		InviteServerEndpoint:  "1.2.3.4:51821",
		InviteServerRoute:     "10.43.0.1/32",
		InviteServerPort:      8443,
	})
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}

	err = env.Service.EnableNetwork(context.Background(), "not-confirmed")
	if err == nil {
		t.Fatal("expected error enabling unconfirmed network")
	}
}
