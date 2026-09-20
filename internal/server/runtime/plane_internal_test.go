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

// Direct access lets this test fail each owned resource independently and assert
// wrapper identity. An external harness cannot stop only DNS or the HTTP server.
// Local service wiring avoids the runtime -> testutil -> runtime import cycle.
func TestPlane_ReconcilesIndependentResourceFailures(
	t *testing.T,
) {
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
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := uint16(ln.Addr().(*net.TCPAddr).Port)
	ln.Close()
	config := &dns.Options{
		Network: "test",
		Zone:    "test.internal",
		Address: fmt.Sprintf("127.0.0.1:%d", port),
		Service: svc,
		Logger:  logging.Discard(),
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
	want := resource.Stopped

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
}

// Wrapper identity and the absence of optional resources are mechanical plane
// ownership guarantees; public runtime tests cover enabling and disabling intent.
func TestPlane_DisabledDNSHasNoResource(
	t *testing.T,
) {
	key, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	backend := wireguardtest.NewMockBackend()
	p, err := newPlane(service.Plane{Name: "test", Cidr: "127.0.0.0/24"}, nil,
		key, nil, wireguard.NewManagerWithBackend(backend), 0, logging.Discard())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.close() })
	for range 2 {
		if err := p.reconcile(t.Context()); err != nil {
			t.Fatal(err)
		}
		if p.dns != nil || p.status().DNS.State != resource.Disabled {
			t.Fatalf("disabled DNS = %+v", p.status().DNS)
		}
	}
	if err := p.close(); err != nil {
		t.Fatal(err)
	}
	if p.status().DNS.State != resource.Disabled || backend.Device("test").CloseCalls != 1 {
		t.Fatalf("closed plane = %+v", p.status())
	}
}

// Direct access stops the HTTP instance while preserving its configured owner.
func TestPlaneReconcile_RepairsFailedHTTP(
	t *testing.T,
) {
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
		0,
		logging.Discard(),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.close() })
	if err := p.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	wrapper, device, run := p.http, p.device, p.http.run
	if err := backend.Device("httptest").Close(); err != nil {
		t.Fatal(err)
	}
	if status := p.status().WireGuard; status.State != resource.Failed || status.Error == nil {
		t.Fatalf("failed device status = %+v", status)
	}
	address := run.server.Addr
	p.http.address = address // Reuse the ephemeral port for replacement attempts.
	if err := p.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if p.status().WireGuard.State != resource.Running || p.device != device {
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
		status := p.status().HTTP
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
	if err := p.reconcile(t.Context()); err == nil {
		t.Fatal("expected blocked restart")
	}
	if status := p.status().HTTP; status.State != resource.Failed || status.Error == nil {
		t.Fatalf("failed restart status = %+v", status)
	}
	if p.http != wrapper || wrapper.run != nil {
		t.Fatal("failed bind lost wrapper or retained a serving instance")
	}
	if err := blocker.Close(); err != nil {
		t.Fatal(err)
	}
	if err := p.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if status := p.status().HTTP; status.State != resource.Running || status.Error != nil {
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
