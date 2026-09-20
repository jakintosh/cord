package runtime_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

func TestHTTP_StartupRetriesAfterPortReleased(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	rt, address, _, _ := newHTTPRuntime(t, env.Service, 0, nil)
	blocker, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	status, err := rt.SetNetworkEnabled(t.Context(), "httptest", true)
	if err != nil {
		t.Fatal(err)
	}
	if status.Running || status.Health != runtime.HealthDegraded || status.Reason == "" {
		t.Fatalf("status = %+v, want failed startup", status)
	}
	if err := blocker.Close(); err != nil {
		t.Fatal(err)
	}
	if err := testutil.ApplyNetwork(t, rt, "httptest"); err != nil {
		t.Fatal(err)
	}
	assertHTTPResponds(t, address)
	assertHTTPHealthy(t, rt)
}

func TestHTTP_StopReleasesListener(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	rt, address, _, stop := newHTTPRuntime(t, env.Service, 0, nil)
	if err := env.Service.SetNetworkEnabled("httptest", true); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.SetNetworkEnabled(t.Context(), "httptest", true); err != nil {
		t.Fatal(err)
	}
	assertHTTPResponds(t, address)
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	assertHTTPPortReleased(t, address)
	if _, err := rt.GetOverview(t.Context()); !errors.Is(err, runtime.ErrNotRunning) {
		t.Fatalf("stopped runtime accepted a read: %v", err)
	}
}

func TestHTTP_StopWithRequestInFlight(
	t *testing.T,
) {
	for _, graceful := range []bool{true, false} {
		t.Run(fmt.Sprintf("graceful=%t", graceful), func(t *testing.T) {
			env := testutil.SetupService(t)
			entered := make(chan struct{})
			release := make(chan struct{})
			exited := make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }

			handlers := func(*runtime.Runtime) runtime.Handlers {
				return runtime.Handlers{Peer: func(string) http.Handler {
					return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						defer close(exited)
						close(entered)
						if graceful {
							<-req.Context().Done()
						} else {
							<-release
						}
						w.WriteHeader(http.StatusNoContent)
					})
				}}
			}

			rt, address, _, stop := newHTTPRuntime(t, env.Service, 50*time.Millisecond, handlers)
			t.Cleanup(unblock)
			if _, err := rt.SetNetworkEnabled(t.Context(), "httptest", true); err != nil {
				t.Fatal(err)
			}

			result := make(chan error, 1)
			go func() { result <- requestHTTP(address) }()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("handler did not enter")
			}

			err := stop()
			if graceful && err != nil {
				t.Fatalf("graceful shutdown: %v", err)
			}
			if !graceful && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("forced shutdown = %v, want timeout", err)
			}
			if err := <-result; graceful && err != nil {
				t.Fatalf("graceful response: %v", err)
			}
			assertHTTPPortReleased(t, address)
			unblock()
			<-exited
		})
	}
}

func newHTTPRuntime(
	t *testing.T,
	svc *service.Service,
	shutdownTimeout time.Duration,
	buildHandlers func(*runtime.Runtime) runtime.Handlers,
) (
	*runtime.Runtime,
	string,
	*wireguardtest.MockBackend,
	func() error,
) {
	t.Helper()
	port := reserveTCPPort(t)
	_, err := svc.CreateNetwork("httptest", "192.168.1.1",
		service.Plane{Cidr: "127.0.0.0/24", ApiPort: port},
		service.Plane{Cidr: "172.16.10.0/24"},
	)
	if err != nil {
		t.Fatal(err)
	}
	backend := wireguardtest.NewMockBackend()
	rt, err := runtime.New(runtime.Options{
		Service:         svc,
		WireGuard:       wireguard.NewManagerWithBackend(backend),
		DNSDisabled:     true,
		Interval:        time.Hour,
		ShutdownTimeout: shutdownTimeout,
	})
	if err != nil {
		t.Fatal(err)
	}
	handlers := runtime.Handlers{Peer: func(string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
	}}
	if buildHandlers != nil {
		handlers = buildHandlers(rt)
	}
	stop := testutil.RunRuntime(t, rt, handlers)
	return rt, fmt.Sprintf("127.0.0.1:%d", port), backend, stop
}

func requestHTTP(
	address string,
) error {
	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Post("http://"+address+"/confirm", "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("HTTP status = %d, want 204", resp.StatusCode)
	}
	return nil
}

func assertHTTPResponds(
	t *testing.T,
	address string,
) {
	t.Helper()
	if err := requestHTTP(address); err != nil {
		t.Fatal(err)
	}
}

func assertHTTPHealthy(
	t *testing.T,
	rt *runtime.Runtime,
) {
	t.Helper()
	status := networkStatus(t, rt, "httptest")
	if !status.Running || status.Health != runtime.HealthHealthy || status.Status == nil || status.Status.Main.HTTP.Error != nil {
		t.Fatalf("status = %+v, want healthy HTTP", status)
	}
}

func assertHTTPPortReleased(
	t *testing.T,
	address string,
) {
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

// A network-disable command occupies the loop while closing HTTP. Canceled
// request contexts must release handlers without another command-loop turn.
func TestHTTP_ShutdownAllowsHandlerToReturnThroughRuntime(
	t *testing.T,
) {
	for _, wholeRuntime := range []bool{false, true} {
		t.Run(fmt.Sprint(wholeRuntime), func(t *testing.T) {
			env := testutil.SetupService(t)
			entered := make(chan struct{})
			result := make(chan error, 1)
			build := func(rt *runtime.Runtime) runtime.Handlers {
				return runtime.Handlers{Peer: func(string) http.Handler {
					return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						close(entered)
						<-req.Context().Done()
						_, err := rt.CreateGroup(req.Context(), "httptest", "late")
						result <- err
						w.WriteHeader(http.StatusNoContent)
					})
				}}
			}
			rt, address, _, stop := newHTTPRuntime(t, env.Service, time.Second, build)
			if status, err := rt.SetNetworkEnabled(t.Context(), "httptest", true); err != nil || !status.Running {
				t.Fatalf("enable = %+v, %v", status, err)
			}

			clientDone := make(chan error, 1)
			go func() { clientDone <- requestHTTP(address) }()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("handler did not enter")
			}

			if wholeRuntime {
				if err := stop(); err != nil {
					t.Fatal(err)
				}
			} else {
				status, err := rt.SetNetworkEnabled(t.Context(), "httptest", false)
				if err != nil || status.Running || status.Reason != "" {
					t.Fatalf("disable = %+v, %v", status, err)
				}
			}

			if err := <-result; !errors.Is(err, context.Canceled) {
				t.Fatalf("late command = %v", err)
			}
			if err := <-clientDone; err != nil {
				t.Fatal(err)
			}
			groups, err := env.Service.ListGroups("httptest")
			if err != nil || len(groups) != 0 {
				t.Fatalf("canceled request persisted: %+v, %v", groups, err)
			}
		})
	}
}

func TestHTTP_DisableReleasesHandlerWaitingForRead(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	store := &blockedDisableStore{
		Store:   env.Database,
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	svc, err := service.New(service.Options{Store: store})
	if err != nil {
		t.Fatal(err)
	}

	entered := make(chan struct{})
	read := make(chan error, 1)
	build := func(rt *runtime.Runtime) runtime.Handlers {
		return runtime.Handlers{Peer: func(string) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				close(entered)
				_, err := rt.GetNetwork(req.Context(), "httptest")
				read <- err
				w.WriteHeader(http.StatusNoContent)
			})
		}}
	}
	rt, address, _, _ := newHTTPRuntime(t, svc, time.Second, build)
	if _, err := rt.SetNetworkEnabled(t.Context(), "httptest", true); err != nil {
		t.Fatal(err)
	}

	var once sync.Once
	release := func() { once.Do(func() { close(store.release) }) }
	t.Cleanup(release)
	disabled := make(chan error, 1)
	go func() {
		status, err := rt.SetNetworkEnabled(t.Context(), "httptest", false)
		if err == nil && status.Reason != "" {
			err = errors.New(status.Reason)
		}
		disabled <- err
	}()
	<-store.entered

	response := make(chan error, 1)
	go func() { response <- requestHTTP(address) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("handler did not enter")
	}
	release()

	if err := <-disabled; err != nil {
		t.Fatalf("disable failed to drain a waiting reader: %v", err)
	}
	if err := <-read; !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting read = %v", err)
	}
	if err := <-response; err != nil {
		t.Fatal(err)
	}
	if _, err := rt.ListNetworks(t.Context()); err != nil {
		t.Fatalf("runtime stopped processing commands: %v", err)
	}
}

type blockedDisableStore struct {
	service.Store
	entered chan struct{}
	release chan struct{}
}

func (s *blockedDisableStore) SetNetworkEnabled(
	name string,
	enabled bool,
) error {
	if !enabled {
		close(s.entered)
		<-s.release
	}
	return s.Store.SetNetworkEnabled(name, enabled)
}
