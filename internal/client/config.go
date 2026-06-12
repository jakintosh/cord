package client

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path"

	"github.com/BurntSushi/toml"

	"git.sr.ht/~jakintosh/cord/internal/server"
)

// ClientConfig is the persistent identity of an installed network:
// the permanent keypair, the assigned address, and how to reach the
// server. Written during install, read by every other command.
type ClientConfig struct {
	NetworkName  string            `toml:"network_name"`
	PrivateKey   string            `toml:"private_key"`
	PublicKey    string            `toml:"public_key"`
	AssignedCidr string            `toml:"assigned_cidr"`
	Server       server.ServerInfo `toml:"server"`
}

// NetworkCidr derives the full network range from the assigned cidr.
func (cfg *ClientConfig) NetworkCidr() (*net.IPNet, error) {
	_, network, err := net.ParseCIDR(cfg.AssignedCidr)
	if err != nil {
		return nil, fmt.Errorf("invalid assigned cidr '%s': %w", cfg.AssignedCidr, err)
	}
	return network, nil
}

// Address returns the interface address: the assigned IP with the
// network's mask.
func (cfg *ClientConfig) Address() (*net.IPNet, error) {
	ip, network, err := net.ParseCIDR(cfg.AssignedCidr)
	if err != nil {
		return nil, fmt.Errorf("invalid assigned cidr '%s': %w", cfg.AssignedCidr, err)
	}
	return &net.IPNet{IP: ip, Mask: network.Mask}, nil
}

func (ctx *Context) configPath() string {
	return path.Join(ctx.ConfigDir, ctx.Name+".toml")
}

// HasConfig reports whether a client config exists for this network.
func (ctx *Context) HasConfig() bool {
	_, err := os.Stat(ctx.configPath())
	return err == nil
}

// SaveConfig writes the client config with key-file permissions.
func (ctx *Context) SaveConfig(cfg *ClientConfig) error {
	if err := os.MkdirAll(ctx.ConfigDir, 0755); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}

	payload := &bytes.Buffer{}
	if err := toml.NewEncoder(payload).Encode(cfg); err != nil {
		return fmt.Errorf("failed to encode client config: %w", err)
	}

	// the config holds the private key: owner read/write only
	if err := os.WriteFile(ctx.configPath(), payload.Bytes(), 0600); err != nil {
		return fmt.Errorf("failed to write client config: %w", err)
	}
	return nil
}

// LoadConfig reads the client config for this network.
func (ctx *Context) LoadConfig() (*ClientConfig, error) {
	payload, err := os.ReadFile(ctx.configPath())
	if err != nil {
		return nil, fmt.Errorf(
			"no installed network '%s' (missing %s): %w",
			ctx.Name, ctx.configPath(), err,
		)
	}

	cfg := &ClientConfig{}
	if err := toml.Unmarshal(payload, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse client config: %w", err)
	}
	return cfg, nil
}

// DeleteConfig removes the client config; missing files are fine.
func (ctx *Context) DeleteConfig() error {
	err := os.Remove(ctx.configPath())
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete client config: %w", err)
	}
	return nil
}
