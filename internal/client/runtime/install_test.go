package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

func TestInstall_Success(t *testing.T) {
	env := testutil.SetupService(t)
	backend := wireguardtest.NewMockBackend()
	rt := newRuntime(t, env, backend, runtime.Options{Interval: time.Hour})
	server := newInstallServer(t)

	network, err := rt.Install(t.Context(), server.invitation("install-me"), service.NetworkOptions{})
	if err != nil {
		t.Fatalf("install network: %v", err)
	}
	if network.Name != "install-me" {
		t.Fatalf("name = %q, want install-me", network.Name)
	}
	if network.AssignedRoute != "10.42.0.5/32" {
		t.Fatalf("assigned route = %q, want 10.42.0.5/32", network.AssignedRoute)
	}
	if !network.Enabled {
		t.Fatal("a confirmed network should be enabled")
	}

	// The invite tunnel is a one-shot: up for the redemption, down again.
	invite := backend.Device("install-me-i")
	if invite == nil {
		t.Fatal("expected the invite device")
	}
	if invite.CloseCalls != 1 {
		t.Fatalf("invite device close calls = %d, want 1", invite.CloseCalls)
	}
	if ops := invite.AppliedOps(); len(ops) != 1 {
		t.Fatalf("invite peer ops = %d, want 1 (the server)", len(ops))
	} else if ops[0].Target.PersistentKeepalive != 0 {
		t.Errorf("invite keepalive = %v, want disabled", ops[0].Target.PersistentKeepalive)
	}

	// The network the runtime owns is the one left running.
	status := networkStatus(t, rt, "install-me")
	if !status.Running {
		t.Fatalf("status = %+v, want running", status)
	}
	main := backend.Device("install-me")
	if main == nil {
		t.Fatal("expected the main device")
	}
	if main.CloseCalls != 0 {
		t.Fatalf("main device close calls = %d, want 0 (the network stays up)", main.CloseCalls)
	}
	ops := main.AppliedOps()
	if len(ops) != 1 {
		t.Fatalf("main peer ops = %d, want 1 (the server)", len(ops))
	}
	if ops[0].Target.PersistentKeepalive != runtime.PersistentKeepaliveInterval {
		t.Errorf(
			"main server keepalive = %v, want %v",
			ops[0].Target.PersistentKeepalive,
			runtime.PersistentKeepaliveInterval,
		)
	}
}

// TestInstall_ResumesFromInvited verifies that an install interrupted
// after BeginInstall resumes with the permanent key already persisted.
func TestInstall_ResumesFromInvited(t *testing.T) {
	env := testutil.SetupService(t)
	backend := wireguardtest.NewMockBackend()
	rt := newRuntime(t, env, backend, runtime.Options{Interval: time.Hour})
	server := newInstallServer(t)

	invitation := server.invitation("resume-inv")
	install, err := env.Service.BeginInstall(invitation, service.NetworkOptions{})
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}

	network, err := rt.Install(t.Context(), invitation, service.NetworkOptions{})
	if err != nil {
		t.Fatalf("install (resume): %v", err)
	}
	if network.PrivateKey != install.MainPrivateKey {
		t.Error("the permanent key should survive a resumed install")
	}

	permPubKey, err := wireguard.PublicKey(install.MainPrivateKey)
	if err != nil {
		t.Fatalf("derive public key: %v", err)
	}
	if got := server.redeemedKey(); got != permPubKey {
		t.Errorf("redeemed with %q, want the persisted key %q", got, permPubKey)
	}
}

// TestInstall_ResumesFromRedeemed verifies that an install interrupted
// after redemption confirms without redeeming a second time.
func TestInstall_ResumesFromRedeemed(t *testing.T) {
	env := testutil.SetupService(t)
	backend := wireguardtest.NewMockBackend()
	rt := newRuntime(t, env, backend, runtime.Options{Interval: time.Hour})
	server := newInstallServer(t)

	invitation := server.invitation("resume-red")
	install, err := env.Service.BeginInstall(invitation, service.NetworkOptions{})
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}
	if _, err := rt.RedeemInstall(t.Context(), install.Name); err != nil {
		t.Fatalf("redeem: %v", err)
	}

	if _, err := rt.Install(t.Context(), invitation, service.NetworkOptions{}); err != nil {
		t.Fatalf("install (resume from redeemed): %v", err)
	}

	if got := server.calls("redeem"); got != 1 {
		t.Errorf("redeem calls = %d, want 1", got)
	}
	if got := server.calls("confirm"); got != 1 {
		t.Errorf("confirm calls = %d, want 1", got)
	}
}

// TestRedeem_Idempotent verifies that a redeemed install is returned
// unchanged without contacting the server again.
func TestRedeem_Idempotent(t *testing.T) {
	env := testutil.SetupService(t)
	backend := wireguardtest.NewMockBackend()
	rt := newRuntime(t, env, backend, runtime.Options{Interval: time.Hour})
	server := newInstallServer(t)

	install, err := env.Service.BeginInstall(
		server.invitation("redeem-once"),
		service.NetworkOptions{},
	)
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}

	first, err := rt.RedeemInstall(t.Context(), install.Name)
	if err != nil {
		t.Fatalf("first redeem: %v", err)
	}
	again, err := rt.RedeemInstall(t.Context(), install.Name)
	if err != nil {
		t.Fatalf("second redeem: %v", err)
	}

	if again.Phase != service.PhaseRedeemed {
		t.Errorf("phase = %q, want %q", again.Phase, service.PhaseRedeemed)
	}
	if again.MainAssignedRoute != first.MainAssignedRoute {
		t.Error("a repeated redemption should not change the assignment")
	}
	if got := server.calls("redeem"); got != 1 {
		t.Errorf("redeem calls = %d, want 1", got)
	}
}

func TestRedeem_CancellationStopsRemoteCall(t *testing.T) {
	env := testutil.SetupService(t)
	backend := wireguardtest.NewMockBackend()
	rt := newRuntime(t, env, backend, runtime.Options{Interval: time.Hour})
	server := newInstallServer(t)
	started := server.blockRedemption()

	install, err := env.Service.BeginInstall(
		server.invitation("cancel-redeem"),
		service.NetworkOptions{},
	)
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	errCh := make(chan error, 1)
	go func() {
		_, err := rt.RedeemInstall(ctx, install.Name)
		errCh <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("redeem request did not reach the server")
	}
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("redeem: got %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("redeem did not stop when its context was cancelled")
	}

	stored, err := env.Service.GetInstall(install.Name)
	if err != nil {
		t.Fatalf("get install: %v", err)
	}
	if stored.Phase != service.PhaseInvited {
		t.Fatalf("phase = %q, want %q", stored.Phase, service.PhaseInvited)
	}
	if device := backend.Device(install.InviteIfaceName); device == nil || device.CloseCalls != 1 {
		t.Fatalf("invite device = %+v, want one closed device", device)
	}
}

func TestConfirm_RequiresRedeemedInstall(t *testing.T) {
	env := testutil.SetupService(t)
	backend := wireguardtest.NewMockBackend()
	rt := newRuntime(t, env, backend, runtime.Options{Interval: time.Hour})
	server := newInstallServer(t)

	if _, err := env.Service.BeginInstall(
		server.invitation("still-invited"),
		service.NetworkOptions{},
	); err != nil {
		t.Fatalf("begin install: %v", err)
	}

	_, err := rt.ConfirmInstall(t.Context(), "still-invited")
	if !errors.Is(err, service.ErrInstallState) {
		t.Errorf("err = %v, want ErrInstallState", err)
	}
	if got := server.calls("confirm"); got != 0 {
		t.Errorf("confirm calls = %d, want 0", got)
	}
}

// installServer is a stand-in for the cord server's invite and peer
// APIs, recording what the runtime asked of it.
type installServer struct {
	server *httptest.Server

	mu           sync.Mutex
	counts       map[string]int
	permKey      string
	peers        []protocol.VisiblePeer
	failSnapshot bool

	blockRedeem   bool
	redeemStarted chan struct{}
}

// newInstallServer starts a server whose redeemed network points back at
// itself, so both the invite and the main tunnel reach it.
func newInstallServer(
	t *testing.T,
) *installServer {
	t.Helper()

	s := &installServer{counts: make(map[string]int)}
	s.server = httptest.NewUnstartedServer(nil)

	redeemed := testutil.Invitation("testnet", s.server.Listener.Addr().String())
	redeemed.Peer = protocol.PeerIdentity{Route: "10.42.0.5/32"}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /redeem", func(w http.ResponseWriter, r *http.Request) {
		var request protocol.RedeemRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			wire.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.mu.Lock()
		s.counts["redeem"]++
		s.permKey = request.PermPubKey
		block := s.blockRedeem
		started := s.redeemStarted
		s.mu.Unlock()
		if block {
			started <- struct{}{}
			<-r.Context().Done()
			return
		}

		wire.WriteData(w, http.StatusOK, redeemed)
	})
	mux.HandleFunc("POST /confirm", func(w http.ResponseWriter, r *http.Request) {
		s.record("confirm")
		wire.WriteData(w, http.StatusOK, map[string]string{"status": "confirmed"})
	})
	mux.HandleFunc("GET /snapshot", func(w http.ResponseWriter, r *http.Request) {
		s.record("snapshot")

		s.mu.Lock()
		peers := s.peers
		fail := s.failSnapshot
		s.mu.Unlock()
		if fail {
			wire.WriteError(w, http.StatusInternalServerError, "snapshot failed")
			return
		}
		wire.WriteData(w, http.StatusOK, testutil.NetworkSnapshot(peers...))
	})

	s.server.Config.Handler = mux
	s.server.Start()
	t.Cleanup(s.server.Close)

	return s
}

func (s *installServer) blockRedemption() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.blockRedeem = true
	s.redeemStarted = make(chan struct{}, 1)
	return s.redeemStarted
}

// invitation returns an invite for the named network whose invite server
// is this one.
func (s *installServer) invitation(
	name string,
) protocol.Invitation {
	return testutil.Invitation(name, s.server.Listener.Addr().String())
}

// serve makes peers the answer to every later snapshot request.
func (s *installServer) serve(
	peers ...protocol.VisiblePeer,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peers = peers
}

func (s *installServer) setSnapshotFailure(
	fail bool,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failSnapshot = fail
}

func (s *installServer) record(
	call string,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counts[call]++
}

func (s *installServer) calls(
	call string,
) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.counts[call]
}

// redeemedKey returns the permanent public key presented at /redeem.
func (s *installServer) redeemedKey() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.permKey
}
