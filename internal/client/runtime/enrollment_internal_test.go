package runtime

import (
	"errors"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
)

func TestEnrollmentInterfaceOwnership(t *testing.T) {
	tunnel, backend := newTestTunnel(t)
	if err := tunnel.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	device := backend.Device("test")
	cleanupErr := errors.New("cleanup failed")
	device.CloseErr = cleanupErr
	t.Cleanup(func() { device.CloseErr = nil })

	enrollment := &Enrollment{
		clock:   time.Now,
		install: &service.Install{Name: "old", InviteIfaceName: "test"},
		tunnel:  tunnel,
	}
	rt := &Runtime{
		clock:       time.Now,
		enrollments: map[string]*Enrollment{"old": enrollment},
		running:     make(map[string]*Network),
		reasons:     make(map[string]string),
	}

	if err := rt.startNetwork(&service.Network{Name: "new", InterfaceName: "test"}); !errors.Is(err, cleanupErr) {
		t.Fatalf("permanent startup = %v, want retained cleanup error", err)
	}
	if _, err := rt.acquireEnrollment(&service.Install{Name: "new", MainIfaceName: "test"}); !errors.Is(err, cleanupErr) {
		t.Fatalf("enrollment startup = %v, want retained cleanup error", err)
	}
	if enrollment.tunnel != tunnel || rt.enrollments["old"] != enrollment || len(backend.CreateCalls) != 1 {
		t.Fatal("failed cleanup lost ownership or allowed interface reuse")
	}

	device.CloseErr = nil
	rt.running["permanent"] = &Network{
		record: &service.Network{InterfaceName: "occupied"},
	}
	attempts := device.CloseCalls
	if _, err := rt.acquireEnrollment(&service.Install{
		Name:            "other",
		InviteIfaceName: "test",
		MainIfaceName:   "occupied",
	}); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("second interface conflict = %v, want conflict", err)
	}
	if device.CloseCalls != attempts || rt.enrollments["old"] != enrollment {
		t.Fatal("network conflict triggered cleanup of another enrollment")
	}

	if _, err := rt.acquireEnrollment(&service.Install{Name: "new", MainIfaceName: "test"}); err != nil {
		t.Fatal(err)
	}
	if enrollment.tunnel != nil || rt.enrollments["old"] != nil || device.Check() == nil {
		t.Fatal("interface reuse did not finish old cleanup")
	}

	rt.running["permanent"] = &Network{
		record:  &service.Network{InterfaceName: "occupied"},
		closing: true,
	}
	if _, err := rt.acquireEnrollment(&service.Install{Name: "other", InviteIfaceName: "occupied"}); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("network-owned interface = %v, want conflict even during cleanup", err)
	}
}
