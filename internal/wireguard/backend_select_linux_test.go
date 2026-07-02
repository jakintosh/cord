//go:build linux

package wireguard_test

import (
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

func TestNewManager_LinuxAuto(t *testing.T) {
	mgr, err := wireguard.NewManager(wireguard.Options{Backend: wireguard.BackendAuto})
	if err != nil {
		t.Fatalf("NewManager(Auto): %v", err)
	}
	if mgr == nil {
		t.Fatal("expected non-nil Manager")
	}
}

func TestNewManager_LinuxKernel(t *testing.T) {
	mgr, err := wireguard.NewManager(wireguard.Options{Backend: wireguard.BackendKernel})
	if err != nil {
		t.Fatalf("NewManager(Kernel): %v", err)
	}
	if mgr == nil {
		t.Fatal("expected non-nil Manager")
	}
}

func TestNewManager_LinuxUserspace(t *testing.T) {
	mgr, err := wireguard.NewManager(wireguard.Options{Backend: wireguard.BackendUserspace})
	if err != nil {
		t.Fatalf("NewManager(Userspace): %v", err)
	}
	if mgr == nil {
		t.Fatal("expected non-nil Manager")
	}
}

func TestNewManager_LinuxUnknown(t *testing.T) {
	_, err := wireguard.NewManager(wireguard.Options{Backend: wireguard.BackendType("bogus")})
	if err == nil {
		t.Error("expected error for unknown backend")
	}
}
