package runtime

import (
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

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
