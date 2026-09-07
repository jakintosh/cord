package runtime

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

func TestPlaneServeReportsUnexpectedFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	results := make(chan error, 2)
	plane := &Plane{
		server: &http.Server{Handler: http.NotFoundHandler()},
		log:    logging.Discard(),
		onServeResult: func(err error) {
			results <- err
		},
	}
	plane.serve(listener, listener.Addr().String())

	if err := waitForServeResult(t, results); err != nil {
		t.Fatalf("initial serve result = %v, want nil", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	if err := waitForServeResult(t, results); err == nil {
		t.Fatal("unexpected listener exit was not reported")
	}
}

func waitForServeResult(
	t *testing.T,
	results <-chan error,
) error {
	t.Helper()
	select {
	case err := <-results:
		return err
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for serve result")
		return nil
	}
}

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
			backend := wireguardtest.NewMockBackend()
			p := &Plane{
				config:     service.Plane{Name: "test", Cidr: "127.0.0.0/24"},
				privateKey: key,
				clock:      time.Now,
				log:        logging.Discard(),
			}
			if enabled {
				ln, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				port := uint16(ln.Addr().(*net.TCPAddr).Port)
				ln.Close()
				p.dnsConfig = &dnsConfig{network: "test", service: svc, port: port}
			}
			if err := p.start(context.Background(), wireguard.NewManagerWithBackend(backend), http.NotFoundHandler()); err != nil {
				p.stop()
				t.Fatal(err)
			}
			defer p.stop()

			device, server := p.device, p.server
			if enabled {
				if p.dns == nil || !p.dns.Status().Serving {
					t.Fatal("DNS is not serving")
				}
				oldDNS := p.dns
				if err := oldDNS.Stop(context.Background()); err != nil {
					t.Fatal(err)
				}
				// A bind conflict must reach the parent as an error,
				// while the healthy HTTP server and device stay owned.
				blocker, err := net.Listen("tcp", oldDNS.Address())
				if err != nil {
					t.Fatal(err)
				}
				defer blocker.Close()
				if err := p.check(); err == nil {
					t.Fatal("DNS bind failure was not returned")
				} else if p.dnsStatus.Error != err.Error() {
					t.Fatalf("status error = %q, returned %v", p.dnsStatus.Error, err)
				}
				if p.device != device || p.server != server {
					t.Fatal("failed DNS repair replaced a healthy sibling")
				}
				if err := blocker.Close(); err != nil {
					t.Fatal(err)
				}
				if err := p.check(); err != nil {
					t.Fatal(err)
				}
				if p.dns == oldDNS || !p.dns.Status().Serving {
					t.Fatal("DNS was not replaced after stopping")
				}
				if p.dnsStatus.Error != "" {
					t.Fatalf("DNS repair error: %s", p.dnsStatus.Error)
				}
				ownedDNS := p.dns
				defer func() {
					if ownedDNS.Status().Serving {
						t.Error("plane stop left DNS serving")
					}
				}()
			} else {
				if err := p.check(); err != nil {
					t.Fatal(err)
				}
				if p.dns != nil {
					t.Fatal("plane without DNS configuration started DNS")
				}
			}
			if p.device != device || p.server != server {
				t.Fatal("DNS maintenance replaced a healthy sibling")
			}
			if err := p.stop(); err != nil {
				t.Fatal(err)
			}
			if backend.Device("test").CloseCalls != 1 {
				t.Fatal("plane did not close its device exactly once")
			}
		})
	}
}
