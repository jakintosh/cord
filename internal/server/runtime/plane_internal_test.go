package runtime

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

func TestPlaneOwnsOptionalDNS(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
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
			var config *dnsConfig
			if enabled {
				ln, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				port := uint16(ln.Addr().(*net.TCPAddr).Port)
				ln.Close()
				config = &dnsConfig{network: "test", service: svc, port: port}
			}
			backend := wireguardtest.NewMockBackend()
			p, err := newPlane(
				service.Plane{Name: "test", Cidr: "127.0.0.0/24"},
				config,
				key,
				http.NotFoundHandler(),
				wireguard.NewManagerWithBackend(backend),
				logging.Discard(),
			)
			if err != nil {
				t.Fatal(err)
			}
			want := resource.Disabled
			if enabled {
				want = resource.Stopped
			}
			if status := p.Status(); status.WireGuard.State != resource.Stopped ||
				status.DNS.State != want || status.HTTP.State != resource.Stopped {
				t.Fatalf("initial status = %+v", status)
			}
			defer p.stop()
			if err := p.start(t.Context()); err != nil {
				t.Fatal(err)
			}
			device, httpWrapper, dnsWrapper := p.device, p.http, p.dns
			if err := p.reconcile(); err != nil {
				t.Fatal(err)
			}
			if enabled {
				if p.Status().DNS.State != resource.Running {
					t.Fatal("DNS not running")
				}
				if err := p.dns.Stop(context.Background()); err != nil {
					t.Fatal(err)
				}
				for range 2 {
					if status := p.Status(); status.DNS.State != resource.Stopped || status.HTTP.State != resource.Running {
						t.Fatalf("status read repaired DNS or changed HTTP: %+v", status)
					}
				}
				blocker, err := net.Listen("tcp", p.dns.Address())
				if err != nil {
					t.Fatal(err)
				}
				defer blocker.Close()
				if err := p.device.Stop(); err != nil {
					t.Fatal(err)
				}
				deviceErr := errors.New("device unavailable")
				backend.CreateErr = deviceErr
				repairErr := p.reconcile()
				if !errors.Is(repairErr, deviceErr) {
					t.Fatalf("device failure was lost: %v", repairErr)
				}
				for range 2 {
					status := p.Status().DNS
					if status.State != resource.Failed || status.Error == nil {
						t.Fatalf("failed DNS status = %+v", status)
					}
				}
				if err := p.dns.Stop(context.Background()); err != nil {
					t.Fatal(err)
				}
				if p.Status().DNS.Error == nil {
					t.Fatal("cleanup discarded failed startup error")
				}
				// HTTP still recovers when both WireGuard and DNS cannot restart.
				run := p.http.run
				if err := run.server.Close(); err != nil {
					t.Fatal(err)
				}
				select {
				case <-run.done:
				case <-time.After(time.Second):
					t.Fatal("HTTP did not exit")
				}
				if err := p.reconcile(); !errors.Is(err, deviceErr) || !errors.Is(err, p.dns.Status().Error) {
					t.Fatalf("resource failures were not joined: %v", err)
				}
				if p.Status().HTTP.State != resource.Running {
					t.Fatal("DNS failure prevented HTTP recovery")
				}
				if err := blocker.Close(); err != nil {
					t.Fatal(err)
				}
				if err := p.reconcile(); !errors.Is(err, deviceErr) {
					t.Fatalf("device failure was lost during DNS recovery: %v", err)
				}
				if status := p.Status().DNS; status.State != resource.Running || status.Error != nil {
					t.Fatalf("recovered DNS status = %+v", status)
				}
				backend.CreateErr = nil
				if err := p.reconcile(); err != nil {
					t.Fatal(err)
				}
			}
			if p.device != device || p.http != httpWrapper || p.dns != dnsWrapper {
				t.Fatal("reconciliation replaced a persistent wrapper or healthy device")
			}
			if err := p.stop(); err != nil {
				t.Fatal(err)
			}
			status := p.Status()
			if status.WireGuard.State != resource.Stopped || status.DNS.State != want || status.HTTP.State != resource.Stopped {
				t.Fatalf("stopped status = %+v", status)
			}
			if p.device != device || p.http != httpWrapper || p.dns != dnsWrapper {
				t.Fatal("Stop discarded configured wrappers")
			}
			if backend.Device("test").CloseCalls != 1 {
				t.Fatal("device was not closed exactly once")
			}
		})
	}
}

// Private access is needed to kill a serving instance while leaving its owner
// alive. Normal startup and shutdown remain covered through runtime_test.
func TestPlaneReconcileRepairsFailedHTTP(t *testing.T) {
	key, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	backend := wireguardtest.NewMockBackend()
	p, err := newPlane(
		service.Plane{Name: "httptest", Cidr: "127.0.0.0/24"},
		nil,
		key,
		http.NotFoundHandler(),
		wireguard.NewManagerWithBackend(backend),
		logging.Discard(),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.stop() })
	if err := p.start(t.Context()); err != nil {
		t.Fatal(err)
	}
	wrapper, device, run := p.http, p.device, p.http.run
	if err := backend.Device("httptest").Close(); err != nil {
		t.Fatal(err)
	}
	if status := p.Status().WireGuard; status.State != resource.Failed || status.Error == nil {
		t.Fatalf("failed device status = %+v", status)
	}
	address := run.server.Addr
	p.http.address = address // Reuse the ephemeral port for replacement attempts.
	if err := p.reconcile(); err != nil {
		t.Fatal(err)
	}
	if p.Status().WireGuard.State != resource.Running || p.device != device {
		t.Fatal("WireGuard recovery lost the wrapper or failed to restore service")
	}
	if p.http.run != run {
		t.Fatal("device recovery replaced healthy HTTP")
	}
	if err := run.server.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-run.done:
	case <-time.After(time.Second):
		t.Fatal("HTTP did not exit")
	}
	for range 2 {
		status := p.Status().HTTP
		if status.State != resource.Failed || !errors.Is(status.Error, http.ErrServerClosed) {
			t.Fatalf("failed status = %+v", status)
		}
	}
	if p.http.run != run {
		t.Fatal("status read repaired HTTP")
	}
	blocker, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	if err := p.reconcile(); err == nil {
		t.Fatal("expected blocked restart")
	}
	if status := p.Status().HTTP; status.State != resource.Failed || status.Error == nil {
		t.Fatalf("failed restart status = %+v", status)
	}
	if p.http != wrapper || wrapper.run != nil {
		t.Fatal("failed bind lost wrapper or retained a serving instance")
	}
	if err := blocker.Close(); err != nil {
		t.Fatal(err)
	}
	if err := p.reconcile(); err != nil {
		t.Fatal(err)
	}
	if status := p.Status().HTTP; status.State != resource.Running || status.Error != nil {
		t.Fatalf("recovered status = %+v", status)
	}
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get("http://" + address)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("handler changed: status %d", resp.StatusCode)
	}
	if p.http != wrapper || p.device != device || len(backend.CreateCalls) != 2 {
		t.Fatal("repair replaced wrapper or device")
	}
}
