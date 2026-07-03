//go:build darwin

package wireguard_test

import (
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

func TestNewManager_DarwinAuto(t *testing.T) {
	mgr, err := wireguard.NewManager(wireguard.Options{Backend: wireguard.BackendAuto})
	if err != nil {
		t.Fatalf("NewManager(Auto): %v", err)
	}
	if mgr == nil {
		t.Fatal("expected non-nil Manager")
	}
}

func TestNewManager_DarwinUserspace(t *testing.T) {
	mgr, err := wireguard.NewManager(wireguard.Options{Backend: wireguard.BackendUserspace})
	if err != nil {
		t.Fatalf("NewManager(Userspace): %v", err)
	}
	if mgr == nil {
		t.Fatal("expected non-nil Manager")
	}
}

func TestNewManager_DarwinKernel(t *testing.T) {
	_, err := wireguard.NewManager(wireguard.Options{Backend: wireguard.BackendKernel})
	if err == nil {
		t.Error("expected error for kernel backend on macOS")
	}
}

func TestNewManager_DarwinUnknown(t *testing.T) {
	_, err := wireguard.NewManager(wireguard.Options{Backend: wireguard.BackendType("bogus")})
	if err == nil {
		t.Error("expected error for unknown backend")
	}
}
