package client_test

import (
	"errors"
	"os"
	"path"
	"strings"
	"testing"

	"git.sr.ht/~jakintosh/cord/internal/client"
	"git.sr.ht/~jakintosh/cord/internal/server"
)

func TestRequireInstalledMissingNetworkErrors(t *testing.T) {
	configDir := path.Join(t.TempDir(), "cfg")

	err := client.RequireInstalled(configDir, "ghost")

	if !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("expected a 'not installed' error, got: %v", err)
	}
}

func TestRequireInstalledMissingNetworkCreatesNoState(t *testing.T) {
	configDir := path.Join(t.TempDir(), "cfg")

	_ = client.RequireInstalled(configDir, "ghost")

	if _, err := os.Stat(configDir); !os.IsNotExist(err) {
		t.Fatalf("expected config dir to not exist, stat err: %v", err)
	}
}

func TestRequireInstalledFindsInstalledNetwork(t *testing.T) {
	configDir := t.TempDir()
	writeClientConfig(t, configDir, "homenet")

	if err := client.RequireInstalled(configDir, "homenet"); err != nil {
		t.Fatalf("expected installed network to be found: %v", err)
	}
}

func TestLoadInviteRejectsTooLongNetworkName(t *testing.T) {
	invitePath := writeInvite(t, "this-name-is-way-too-long")

	_, err := client.LoadInvite(invitePath)

	if err == nil || !strings.Contains(err.Error(), "exceeds 15 bytes") {
		t.Fatalf("expected an 'exceeds 15 bytes' error, got: %v", err)
	}
}

func TestLoadInviteAcceptsValidNetworkName(t *testing.T) {
	invitePath := writeInvite(t, "homenet")

	invite, err := client.LoadInvite(invitePath)
	if err != nil {
		t.Fatalf("failed to load valid invite: %v", err)
	}
	if invite.Interface.NetworkName != "homenet" {
		t.Fatalf("expected network 'homenet', got '%s'", invite.Interface.NetworkName)
	}
}

// helpers

func writeClientConfig(t *testing.T, configDir string, network string) {
	t.Helper()
	configPath := path.Join(configDir, network+".toml")
	if err := os.WriteFile(configPath, []byte("network_name = \""+network+"\"\n"), 0600); err != nil {
		t.Fatalf("failed to write client config: %v", err)
	}
}

func writeInvite(t *testing.T, network string) string {
	t.Helper()
	payload := "[interface]\n" +
		"network_name = \"" + network + "\"\n" +
		"private_key = \"test-key\"\n" +
		"assigned_cidr = \"172.16.10.2/24\"\n"
	invitePath := path.Join(t.TempDir(), "peer.invite")
	if err := os.WriteFile(invitePath, []byte(payload), 0600); err != nil {
		t.Fatalf("failed to write invite: %v", err)
	}
	return invitePath
}
