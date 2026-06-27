//go:build darwin

package wireguard

import "testing"

func TestNewBackend_DarwinAuto(t *testing.T) {
	b, err := newBackend(BackendAuto)
	if err != nil {
		t.Fatalf("newBackend(Auto): %v", err)
	}
	if _, ok := b.(*UserspaceBackend); !ok {
		t.Errorf("expected *UserspaceBackend, got %T", b)
	}
}

func TestNewBackend_DarwinUserspace(t *testing.T) {
	b, err := newBackend(BackendUserspace)
	if err != nil {
		t.Fatalf("newBackend(Userspace): %v", err)
	}
	if _, ok := b.(*UserspaceBackend); !ok {
		t.Errorf("expected *UserspaceBackend, got %T", b)
	}
}

func TestNewBackend_DarwinKernel(t *testing.T) {
	_, err := newBackend(BackendKernel)
	if err == nil {
		t.Error("expected error for kernel backend on macOS")
	}
}

func TestNewBackend_DarwinUnknown(t *testing.T) {
	_, err := newBackend(BackendType("bogus"))
	if err == nil {
		t.Error("expected error for unknown backend")
	}
}

func TestTunName_Darwin(t *testing.T) {
	if tunName("anything") != "utun" {
		t.Errorf("tunName = %q, want utun", tunName("anything"))
	}
}
