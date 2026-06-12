package main

import (
	"net"
	"os"
	"path"
	"strings"
	"testing"

	"git.sr.ht/~jakintosh/cord/internal/server"
)

func TestOpenServerReadMissingNetworkErrors(t *testing.T) {
	configDir := path.Join(t.TempDir(), "cfg")
	dataDir := path.Join(t.TempDir(), "data")

	_, err := openServerRead(configDir, dataDir, "ghost")

	if err == nil {
		t.Fatal("expected an error for a missing network")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected a 'not found' error, got: %v", err)
	}
}

func TestOpenServerReadCreatesNoState(t *testing.T) {
	configDir := path.Join(t.TempDir(), "cfg")
	dataDir := path.Join(t.TempDir(), "data")

	_, _ = openServerRead(configDir, dataDir, "ghost")

	expectMissing(t, configDir)
	expectMissing(t, dataDir)
}

func TestOpenServerReadOpensExistingNetwork(t *testing.T) {
	configDir := path.Join(t.TempDir(), "cfg")
	dataDir := path.Join(t.TempDir(), "data")
	seedNetwork(t, configDir, dataDir, "homenet")

	srv, err := openServerRead(configDir, dataDir, "homenet")
	if err != nil {
		t.Fatalf("failed to open existing network: %v", err)
	}

	overview, err := srv.GetNetworkOverview()
	if err != nil {
		t.Fatalf("failed to read network overview: %v", err)
	}
	if overview.Name != "homenet" {
		t.Fatalf("expected network 'homenet', got '%s'", overview.Name)
	}
}

// helpers

func expectMissing(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected '%s' to not exist, stat err: %v", dir, err)
	}
}

func seedNetwork(t *testing.T, configDir string, dataDir string, name string) {
	t.Helper()

	srv, err := openServerWrite(configDir, dataDir, name)
	if err != nil {
		t.Fatalf("failed to open server for seeding: %v", err)
	}

	_, rootCidr, _ := net.ParseCIDR("10.42.0.0/16")
	_, inviteCidr, _ := net.ParseCIDR("172.16.10.0/24")
	err = srv.CreateNetwork(server.CreateNetworkRequest{
		RootCidr:   rootCidr,
		InviteCidr: inviteCidr,
		ExternalIP: net.IPv4(203, 0, 113, 1),
		ListenPort: 51820,
		InvitePort: 51821,
		ApiPort:    51820,
	})
	if err != nil {
		t.Fatalf("failed to create network: %v", err)
	}
}
