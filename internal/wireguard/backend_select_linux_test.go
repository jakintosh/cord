//go:build linux

package wireguard

import "testing"

func TestNewBackend_LinuxAuto(t *testing.T) {
	b, err := newBackend(BackendAuto)
	if err != nil {
		t.Fatalf("newBackend(Auto): %v", err)
	}
	if _, ok := b.(*KernelBackend); !ok {
		t.Errorf("expected *KernelBackend, got %T", b)
	}
}

func TestNewBackend_LinuxKernel(t *testing.T) {
	b, err := newBackend(BackendKernel)
	if err != nil {
		t.Fatalf("newBackend(Kernel): %v", err)
	}
	if _, ok := b.(*KernelBackend); !ok {
		t.Errorf("expected *KernelBackend, got %T", b)
	}
}

func TestNewBackend_LinuxUserspace(t *testing.T) {
	b, err := newBackend(BackendUserspace)
	if err != nil {
		t.Fatalf("newBackend(Userspace): %v", err)
	}
	if _, ok := b.(*UserspaceBackend); !ok {
		t.Errorf("expected *UserspaceBackend, got %T", b)
	}
}

func TestNewBackend_LinuxUnknown(t *testing.T) {
	_, err := newBackend(BackendType("bogus"))
	if err == nil {
		t.Error("expected error for unknown backend")
	}
}

func TestTunName_Linux(t *testing.T) {
	name := "my-wg-interface"
	if tunName(name) != name {
		t.Errorf("tunName = %q, want %q", tunName(name), name)
	}
}
