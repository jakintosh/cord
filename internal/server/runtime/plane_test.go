package runtime_test

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	peerapi "git.studiopollinator.com/pollinator/cord/internal/server/api/peer"
	"git.studiopollinator.com/pollinator/cord/internal/server/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

func TestHTTPStartupRetriesAfterPortReleased(t *testing.T) {
	env := testutil.SetupService(t)
	rt, address, _ := newHTTPRuntime(t, env.Service)
	blocker, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	status, err := rt.SetNetworkEnabled("httptest", true)
	if err != nil {
		t.Fatal(err)
	}
	if status.Running || status.Health != runtime.HealthDegraded || status.Reason == "" {
		t.Fatalf("status = %+v, want failed startup", status)
	}
	if err := blocker.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rt.Converge("httptest"); err != nil {
		t.Fatal(err)
	}
	assertHTTPResponds(t, address)
	assertHTTPHealthy(t, rt)
}

func TestHTTPStopReleasesListener(t *testing.T) {
	env := testutil.SetupService(t)
	rt, address, _ := newHTTPRuntime(t, env.Service)
	if err := env.Service.SetNetworkEnabled("httptest", true); err != nil {
		t.Fatal(err)
	}
	if err := rt.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertHTTPResponds(t, address)
	rt.Stop()
	rt.Stop()
	assertHTTPPortReleased(t, address)
	if status := networkStatus(t, rt, "httptest"); status.Running {
		t.Fatal("network still running after Stop")
	}
}

func TestHTTPStopWithRequestInFlight(t *testing.T) {
	for _, graceful := range []bool{true, false} {
		t.Run(fmt.Sprintf("graceful=%t", graceful), func(t *testing.T) {
			env := testutil.SetupService(t)
			store := &blockedHTTPStore{
				Store:   env.Database,
				entered: make(chan struct{}),
				release: make(chan struct{}),
				exited:  make(chan struct{}),
			}
			svc, err := service.New(service.Options{Store: store})
			if err != nil {
				t.Fatal(err)
			}
			rt, address, _ := newHTTPRuntime(t, svc)
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(store.release) }) }
			// Release before runtime/database cleanup, including on test failure.
			t.Cleanup(release)
			if _, err := rt.SetNetworkEnabled("httptest", true); err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() { result <- requestHTTP(address) }()
			select {
			case <-store.entered:
			case <-time.After(2 * time.Second):
				t.Fatal("API request did not reach the store")
			}
			stopped := make(chan struct{})
			go func() {
				rt.Stop()
				close(stopped)
			}()
			waitFor(t, func() bool {
				conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
				if err != nil {
					return true
				}
				conn.Close()
				return false
			}, "HTTP listener to stop accepting connections")
			select {
			case <-stopped:
				t.Fatal("Stop returned without allowing the in-flight request to finish")
			default:
			}
			if graceful {
				release()
			}
			select {
			case <-stopped:
			case <-time.After(8 * time.Second):
				t.Fatal("Stop did not finish within the shutdown allowance")
			}
			select {
			case err := <-result:
				if graceful && err != nil {
					t.Fatalf("graceful shutdown interrupted the response: %v", err)
				}
				if !graceful && err == nil {
					t.Fatal("blocked request was not disconnected")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("client remained connected after Stop")
			}
			assertHTTPPortReleased(t, address)
			release()
			select {
			case <-store.exited:
			case <-time.After(time.Second):
				t.Fatal("store operation did not finish after release")
			}
		})
	}
}

// The real /confirm endpoint waits on this public service dependency.
// Other operations use the normal database, including runtime reconciliation.
type blockedHTTPStore struct {
	service.Store
	entered chan struct{}
	release chan struct{}
	exited  chan struct{}
}

func (s *blockedHTTPStore) GetProvisionalPeerByIP(string, net.IP) (*service.Peer, error) {
	defer close(s.exited)
	close(s.entered)
	<-s.release
	return nil, service.ErrNotFound
}

func newHTTPRuntime(t *testing.T, svc *service.Service) (*runtime.Runtime, string, *wireguardtest.MockBackend) {
	t.Helper()
	port := freeDNSPort(t)
	_, err := svc.CreateNetwork("httptest", "192.168.1.1",
		service.Plane{Cidr: "127.0.0.0/24", ApiPort: port},
		service.Plane{Cidr: "172.16.10.0/24"},
	)
	if err != nil {
		t.Fatal(err)
	}
	backend := wireguardtest.NewMockBackend()
	rt, err := runtime.New(runtime.Options{
		Service:     svc,
		WireGuard:   wireguard.NewManagerWithBackend(backend),
		Peer:        peerapi.New(svc, nil),
		DNSDisabled: true,
		Interval:    time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Stop)
	return rt, fmt.Sprintf("127.0.0.1:%d", port), backend
}

func requestHTTP(address string) error {
	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Post("http://"+address+"/confirm", "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// No provisional peer was seeded: the real API should reject this identity.
	if resp.StatusCode != http.StatusForbidden {
		return fmt.Errorf("HTTP status = %d, want 403", resp.StatusCode)
	}
	return nil
}

func assertHTTPResponds(t *testing.T, address string) {
	t.Helper()
	if err := requestHTTP(address); err != nil {
		t.Fatal(err)
	}
}

func assertHTTPHealthy(t *testing.T, rt *runtime.Runtime) {
	t.Helper()
	status := networkStatus(t, rt, "httptest")
	if !status.Running || status.Health != runtime.HealthHealthy || status.Status == nil || status.Status.Main.HTTP.Error != nil {
		t.Fatalf("status = %+v, want healthy HTTP", status)
	}
}

func assertHTTPPortReleased(t *testing.T, address string) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", address, time.Second)
	if err == nil {
		conn.Close()
		t.Fatal("HTTP still accepts connections after Stop")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("HTTP port was not released: %v", err)
	}
	listener.Close()
}
