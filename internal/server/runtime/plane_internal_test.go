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
	"git.studiopollinator.com/pollinator/cord/internal/server/dns"
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
			var config *dns.Options
			if enabled {
				ln, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				port := uint16(ln.Addr().(*net.TCPAddr).Port)
				ln.Close()
				config = &dns.Options{
					Network: "test",
					Zone:    "test.internal",
					Address: fmt.Sprintf("127.0.0.1:%d", port),
					Service: svc,
					Logger:  logging.Discard(),
				}
			}
			backend := wireguardtest.NewMockBackend()
			p, err := newPlane(
				service.Plane{Name: "test", Cidr: "127.0.0.0/24"},
				config,
				key,
				http.NotFoundHandler(),
				wireguard.NewManagerWithBackend(backend),
				0,
				logging.Discard(),
			)
			if err != nil {
				t.Fatal(err)
			}
			want := resource.Disabled
			if enabled {
				want = resource.Stopped
			}
			if status := p.status(); status.WireGuard.State != resource.Stopped ||
				status.DNS.State != want || status.HTTP.State != resource.Stopped {
				t.Fatalf("initial status = %+v", status)
			}
			defer p.close()
			if err := p.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			device, httpWrapper, dnsWrapper := p.device, p.http, p.dns
			if err := p.reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			if enabled {
				if p.status().DNS.State != resource.Running {
					t.Fatal("DNS not running")
				}
				if err := p.dns.Close(context.Background()); err != nil {
					t.Fatal(err)
				}
				for range 2 {
					if status := p.status(); status.DNS.State != resource.Stopped || status.HTTP.State != resource.Running {
						t.Fatalf("status read repaired DNS or changed HTTP: %+v", status)
					}
				}
				blocker, err := net.Listen("tcp", p.dns.Address())
				if err != nil {
					t.Fatal(err)
				}
				defer blocker.Close()
				if err := p.device.Close(); err != nil {
					t.Fatal(err)
				}
				deviceErr := errors.New("device unavailable")
				backend.CreateErr = deviceErr
				repairErr := p.reconcile(t.Context())
				if !errors.Is(repairErr, deviceErr) {
					t.Fatalf("device failure was lost: %v", repairErr)
				}
				for range 2 {
					status := p.status().DNS
					if status.State != resource.Failed || status.Error == nil {
						t.Fatalf("failed DNS status = %+v", status)
					}
				}
				if err := p.dns.Close(context.Background()); err != nil {
					t.Fatal(err)
				}
				if p.status().DNS.Error == nil {
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
				if err := p.reconcile(t.Context()); !errors.Is(err, deviceErr) || !errors.Is(err, p.dns.Status().Error) {
					t.Fatalf("resource failures were not joined: %v", err)
				}
				if p.status().HTTP.State != resource.Running {
					t.Fatal("DNS failure prevented HTTP recovery")
				}
				if err := blocker.Close(); err != nil {
					t.Fatal(err)
				}
				if err := p.reconcile(t.Context()); !errors.Is(err, deviceErr) {
					t.Fatalf("device failure was lost during DNS recovery: %v", err)
				}
				if status := p.status().DNS; status.State != resource.Running || status.Error != nil {
					t.Fatalf("recovered DNS status = %+v", status)
				}
				backend.CreateErr = nil
				if err := p.reconcile(t.Context()); err != nil {
					t.Fatal(err)
				}
			}
			if p.device != device || p.http != httpWrapper || p.dns != dnsWrapper {
				t.Fatal("reconciliation replaced a persistent wrapper or healthy device")
			}
			if err := p.close(); err != nil {
				t.Fatal(err)
			}
			status := p.status()
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
