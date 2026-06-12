package server

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strconv"
	"syscall"

	"github.com/BurntSushi/toml"

	"git.sr.ht/~jakintosh/cord/internal/utils"
)

type Config interface {
	GetConfigWriter(name string) (io.Writer, error)
	GetConfigReader(name string) (io.Reader, error)
	DeleteConfig(name string) error
}

// NetworkConfig is the persistent identity of a cord network, written
// once at network creation and read by serve/invite operations.
type NetworkConfig struct {
	Name             string `toml:"name"`
	PrivateKey       string `toml:"private_key"`
	PublicKey        string `toml:"public_key"`
	RootCidr         string `toml:"root_cidr"`
	InviteCidr       string `toml:"invite_cidr"`
	ExternalIP       string `toml:"external_ip"`
	ListenPort       uint16 `toml:"listen_port"`
	InviteListenPort uint16 `toml:"invite_listen_port"`
	ApiPort          uint16 `toml:"api_port"`
}

// RootNet parses the main network CIDR.
func (cfg *NetworkConfig) RootNet() (*net.IPNet, error) {
	_, cidr, err := net.ParseCIDR(cfg.RootCidr)
	if err != nil {
		return nil, fmt.Errorf("invalid root cidr in config: %w", err)
	}
	return cidr, nil
}

// InviteNet parses the invite network CIDR.
func (cfg *NetworkConfig) InviteNet() (*net.IPNet, error) {
	_, cidr, err := net.ParseCIDR(cfg.InviteCidr)
	if err != nil {
		return nil, fmt.Errorf("invalid invite cidr in config: %w", err)
	}
	return cidr, nil
}

// ServerIP is the server's address on the main network (first assignable).
func (cfg *NetworkConfig) ServerIP() (net.IP, error) {
	cidr, err := cfg.RootNet()
	if err != nil {
		return nil, err
	}
	return utils.GetFirstAssignableIpFromCidr(cidr), nil
}

// InviteServerIP is the server's address on the invite network.
func (cfg *NetworkConfig) InviteServerIP() (net.IP, error) {
	cidr, err := cfg.InviteNet()
	if err != nil {
		return nil, err
	}
	return utils.GetFirstAssignableIpFromCidr(cidr), nil
}

// ExternalEndpoint is the public WireGuard endpoint of the main network.
func (cfg *NetworkConfig) ExternalEndpoint() string {
	return net.JoinHostPort(cfg.ExternalIP, strconv.Itoa(int(cfg.ListenPort)))
}

// ExternalInviteEndpoint is the public WireGuard endpoint of the invite network.
func (cfg *NetworkConfig) ExternalInviteEndpoint() string {
	return net.JoinHostPort(cfg.ExternalIP, strconv.Itoa(int(cfg.InviteListenPort)))
}

// InternalApiEndpoint is the HTTP API address reachable over the main network.
func (cfg *NetworkConfig) InternalApiEndpoint() (string, error) {
	ip, err := cfg.ServerIP()
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(ip.String(), strconv.Itoa(int(cfg.ApiPort))), nil
}

// InviteApiEndpoint is the HTTP API address reachable over the invite network.
func (cfg *NetworkConfig) InviteApiEndpoint() (string, error) {
	ip, err := cfg.InviteServerIP()
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(ip.String(), strconv.Itoa(int(cfg.ApiPort))), nil
}

func configFileName(network string) string {
	return network + ".toml"
}

// SaveConfig persists the network config through the server's config store.
func (srv *Server) SaveConfig(cfg *NetworkConfig) error {
	w, err := srv.Config.GetConfigWriter(configFileName(srv.Network))
	if err != nil {
		return fmt.Errorf("failed to open config for writing: %w", err)
	}

	if err := toml.NewEncoder(w).Encode(cfg); err != nil {
		return fmt.Errorf("failed to write network config: %w", err)
	}
	return nil
}

// LoadConfig reads the network config from the server's config store.
func (srv *Server) LoadConfig() (*NetworkConfig, error) {
	r, err := srv.Config.GetConfigReader(configFileName(srv.Network))
	if err != nil {
		return nil, fmt.Errorf("failed to open config for reading: %w", err)
	}

	cfg := &NetworkConfig{}
	if _, err := toml.NewDecoder(r).Decode(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse network config: %w", err)
	}
	return cfg, nil
}

// FsConfig
// Uses the filesystem to manage the configuration

type FsConfig struct {
	Directory string
}

func NewFsConfig(dir string) *FsConfig {

	return &FsConfig{dir}
}

func (cfg *FsConfig) GetConfigWriter(name string) (io.Writer, error) {

	if err := os.MkdirAll(cfg.Directory, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory '%s': %w", cfg.Directory, err)
	}
	filepath := path.Join(cfg.Directory, name)
	w, err := os.Create(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file '%s': %w", filepath, err)
	}
	return w, nil
}

func (cfg *FsConfig) GetConfigReader(name string) (io.Reader, error) {

	filepath := path.Join(cfg.Directory, name)
	r, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file '%s': %w", filepath, err)
	}
	return r, nil
}

func (cfg *FsConfig) DeleteConfig(name string) error {

	filepath := path.Join(cfg.Directory, name)
	if err := os.Remove(filepath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete config '%s': %w", filepath, err)
	}
	if err := os.Remove(cfg.Directory); err != nil &&
		!os.IsNotExist(err) &&
		!errors.Is(err, syscall.ENOTEMPTY) &&
		!errors.Is(err, syscall.EEXIST) {
		return fmt.Errorf("failed to delete empty config directory '%s': %w", cfg.Directory, err)
	}
	return nil
}

// MemConfig
// Uses memory to manage the configuration

type MemConfig struct {
	Buffers map[string]*bytes.Buffer
}

func NewMemConfig() *MemConfig {
	return &MemConfig{
		Buffers: map[string]*bytes.Buffer{},
	}
}

func (cfg *MemConfig) GetConfigWriter(name string) (io.Writer, error) {

	if buf, ok := cfg.Buffers[name]; ok {
		buf.Reset()
		return buf, nil
	}
	cfg.Buffers[name] = &bytes.Buffer{}
	return cfg.Buffers[name], nil
}

func (cfg *MemConfig) GetConfigReader(name string) (io.Reader, error) {

	buf, ok := cfg.Buffers[name]
	if !ok {
		return nil, fmt.Errorf("no config named '%s'", name)
	}
	return bytes.NewReader(buf.Bytes()), nil
}

func (cfg *MemConfig) DeleteConfig(name string) error {

	delete(cfg.Buffers, name)
	return nil
}
