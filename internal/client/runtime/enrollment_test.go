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

func TestConfirm_ClosesTemporaryTunnelBeforePublishingNetwork(t *testing.T) {
	for _, failClose := range []bool{false, true} {
		name := "success"
		if failClose {
			name = "cleanup failure"
		}
		t.Run(name, func(t *testing.T) {
			env := testutil.SetupService(t)
			server := newInstallServer(t)
			backend := &confirmationBackend{
				MockBackend: wireguardtest.NewMockBackend(),
			}
			if failClose {
				backend.closeErr = errors.New("main tunnel cleanup failed")
			}
			rt, err := runtime.New(runtime.Options{
				Service:    env.Service,
				WireGuard:  wireguard.NewManagerWithBackend(backend),
				HTTPClient: server.server.Client(),
			})
			if err != nil {
				t.Fatal(err)
			}
			testutil.RunRuntime(t, rt)
			t.Cleanup(func() {
				if device := backend.Device("testnet"); device != nil {
					device.CloseErr = nil
					_ = device.Close()
				}
			})

			_, err = rt.Install(t.Context(), server.invitation("testnet"), service.NetworkOptions{})
			if !failClose {
				if err != nil {
					t.Fatal(err)
				}
				if !networkStatus(t, rt, "testnet").Running {
					t.Fatal("permanent network did not start")
				}
				return
			}

			if !errors.Is(err, backend.closeErr) {
				t.Fatalf("install = %v, want cleanup failure", err)
			}
			if server.calls("confirm") != 1 {
				t.Fatal("expected remote confirmation before cleanup failure")
			}
			if _, err := env.Service.GetNetwork("testnet"); !errors.Is(err, service.ErrNotFound) {
				t.Fatalf("network lookup = %v, want no published network", err)
			}
			install, err := env.Service.GetInstall("testnet")
			if err != nil {
				t.Fatal(err)
			}
			if install.Phase != service.PhaseRedeemed {
				t.Fatalf("install phase = %q, want redeemed", install.Phase)
			}
			if len(backend.CreateCalls) != 2 {
				t.Fatal("cleanup failure allowed permanent interface creation")
			}
		})
	}
}

func TestEnrollmentRetainsCleanup(t *testing.T) {
	for _, action := range []string{"retry", "maintenance after uninstall", "shutdown"} {
		t.Run(action, func(t *testing.T) {
			env := testutil.SetupService(t)
			server := newInstallServer(t)
			cleanupErr := errors.New("temporary tunnel cleanup failed")
			backend := &confirmationBackend{
				MockBackend: wireguardtest.NewMockBackend(),
				closeErr:    cleanupErr,
			}
			rt, err := runtime.New(runtime.Options{
				Service:    env.Service,
				WireGuard:  wireguard.NewManagerWithBackend(backend),
				HTTPClient: server.server.Client(),
				Interval:   time.Hour,
			})
			if err != nil {
				t.Fatal(err)
			}
			finish := testutil.RunRuntime(t, rt)
			if _, err := rt.Install(t.Context(), server.invitation("testnet"), service.NetworkOptions{}); !errors.Is(err, cleanupErr) {
				t.Fatalf("install = %v, want cleanup error", err)
			}
			device := backend.Device("testnet")
			t.Cleanup(func() {
				device.CloseErr = nil
				_ = device.Close()
			})
			attempts := device.CloseCalls
			creates := len(backend.CreateCalls)

			// Retry must close the old child before creating another interface.
			if _, err := rt.ConfirmInstall(t.Context(), "testnet"); !errors.Is(err, cleanupErr) {
				t.Fatalf("confirm retry = %v, want cleanup error", err)
			}
			if device.CloseCalls <= attempts || len(backend.CreateCalls) != creates {
				t.Fatal("retry did not retain the old child exclusively")
			}

			if action == "shutdown" {
				attempts = device.CloseCalls
				if err := finish(); !errors.Is(err, cleanupErr) {
					t.Fatalf("shutdown = %v, want cleanup error", err)
				}
				if device.CloseCalls <= attempts {
					t.Fatal("shutdown did not retry enrollment cleanup")
				}
				return
			}

			device.CloseErr = nil
			backend.closeErr = nil
			if action == "maintenance after uninstall" {
				if err := rt.UninstallNetwork(t.Context(), "testnet"); err != nil {
					t.Fatal(err)
				}
				if err := rt.ConvergeAll(t.Context()); err != nil {
					t.Fatal(err)
				}
				if device.Check() == nil {
					t.Fatal("maintenance forgot cleanup after deleting the saved install")
				}
				attempts = device.CloseCalls
				if err := rt.ConvergeAll(t.Context()); err != nil {
					t.Fatal(err)
				}
				if device.CloseCalls != attempts {
					t.Fatal("successful cleanup remained registered")
				}
				return
			}

			if _, err := rt.ConfirmInstall(t.Context(), "testnet"); err != nil {
				t.Fatal(err)
			}
			if server.calls("redeem") != 1 || !networkStatus(t, rt, "testnet").Running {
				t.Fatal("retry did not resume saved progress into a permanent network")
			}
		})
	}
}

func TestEnrollmentRetainsTunnelAfterPeerSetupFailure(t *testing.T) {
	env := testutil.SetupService(t)
	server := newInstallServer(t)
	setupErr := errors.New("peer setup failed")
	cleanupErr := errors.New("cleanup failed")
	backend := &confirmationBackend{
		MockBackend:   wireguardtest.NewMockBackend(),
		closeErr:      cleanupErr,
		invitePeerErr: setupErr,
	}
	rt, err := runtime.New(runtime.Options{
		Service:    env.Service,
		WireGuard:  wireguard.NewManagerWithBackend(backend),
		HTTPClient: server.server.Client(),
		Interval:   time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	testutil.RunRuntime(t, rt)
	_, err = rt.Install(t.Context(), server.invitation("testnet"), service.NetworkOptions{})
	if !errors.Is(err, setupErr) || !errors.Is(err, cleanupErr) {
		t.Fatalf("install = %v, want setup and cleanup errors", err)
	}
	device := backend.Device("testnet-i")
	t.Cleanup(func() {
		device.CloseErr = nil
		_ = device.Close()
	})
	if server.calls("redeem") != 0 {
		t.Fatal("redemption ran after peer setup failed")
	}
	device.CloseErr = nil
	if err := rt.ConvergeAll(t.Context()); err != nil {
		t.Fatal(err)
	}
	if device.Check() == nil {
		t.Fatal("partial startup lost its tunnel before cleanup succeeded")
	}
}

// confirmationBackend rejects creation while the previous interface is live.
type confirmationBackend struct {
	*wireguardtest.MockBackend
	closeErr      error
	invitePeerErr error
}

func (b *confirmationBackend) CreateDevice(
	config wireguard.DeviceConfig,
) (
	wireguard.WgDevice,
	error,
) {
	if previous := b.Device(config.Name); previous != nil && previous.Check() == nil {
		return nil, errors.New("previous interface is still live")
	}
	device, err := b.MockBackend.CreateDevice(config)
	if err != nil {
		return nil, err
	}
	if config.Name == "testnet" {
		b.Device(config.Name).CloseErr = b.closeErr
	}
	if config.Name == "testnet-i" && b.invitePeerErr != nil {
		b.Device(config.Name).ApplyPeerOpsErr = b.invitePeerErr
		b.Device(config.Name).CloseErr = b.closeErr
	}
	return device, nil
}

func TestInstall_Success(t *testing.T) {
	env := testutil.SetupService(t)
	backend := wireguardtest.NewMockBackend()
	rt := newRuntime(t, env, backend, runtime.Options{Interval: time.Hour})
	testutil.RunRuntime(t, rt)
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
	testutil.RunRuntime(t, rt)
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
	testutil.RunRuntime(t, rt)
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
	testutil.RunRuntime(t, rt)
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
	testutil.RunRuntime(t, rt)
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

	// Cancellation releases the caller before the owner finishes cleanup.
	// A subsequent command waits for that operation to finish.
	if _, err := rt.GetStatus(t.Context()); err != nil {
		t.Fatal(err)
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
	testutil.RunRuntime(t, rt)
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

func TestOnboardingSerializesCommandsAndStopsWithRuntime(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "other")
	server := newInstallServer(t)
	started := server.blockRedemption()
	backend := wireguardtest.NewMockBackend()
	rt := newRuntime(t, env, backend, runtime.Options{
		Interval:   time.Hour,
		HTTPClient: server.server.Client(),
	})
	finish := testutil.RunRuntime(t, rt)
	install, err := env.Service.BeginInstall(server.invitation("onboarding"), service.NetworkOptions{})
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := rt.RedeemInstall(t.Context(), install.Name)
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("redemption did not start")
	}

	// A second command cannot mutate intent while the owner is in redemption.
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := rt.SetNetworkEnabled(ctx, "other", true); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued command = %v", err)
	}
	if err := finish(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) && !errors.Is(err, runtime.ErrNotRunning) {
			t.Fatalf("redemption after shutdown = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not release the onboarding caller")
	}
	if backend.Device(install.InviteIfaceName).CloseCalls != 1 {
		t.Fatal("shutdown did not close the onboarding tunnel")
	}
	record, err := env.Service.GetNetwork("other")
	if err != nil || record.Enabled {
		t.Fatalf("queued command changed intent: %+v, %v", record, err)
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
