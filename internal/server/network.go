package server

import (
	"fmt"
	"net"

	"git.sr.ht/~jakintosh/cord/internal/utils"
	wg "git.sr.ht/~jakintosh/cord/internal/wireguard"
)

// CreateNetworkRequest carries everything needed to initialize a network.
type CreateNetworkRequest struct {
	RootCidr   *net.IPNet
	InviteCidr *net.IPNet
	ExternalIP net.IP
	ListenPort uint16
	InvitePort uint16
	ApiPort    uint16
}

// CreateNetwork initializes a new cord network: it generates the
// server's WireGuard identity, creates the root CIDR and server peer in
// the database, and persists the network config file.
func (ctx *Context) CreateNetwork(
	req CreateNetworkRequest,
) error {

	if err := utils.ValidateHostName(ctx.Name); err != nil {
		return fmt.Errorf("failed to validate network name: %w", err)
	}

	if req.RootCidr.Contains(req.InviteCidr.IP) || req.InviteCidr.Contains(req.RootCidr.IP) {
		return fmt.Errorf(
			"invite cidr '%s' must not overlap root cidr '%s'",
			req.InviteCidr.String(), req.RootCidr.String(),
		)
	}

	// make sure we have a config writer before we do all the db work
	if _, err := ctx.Config.GetConfigWriter(configFileName(ctx.Name)); err != nil {
		return fmt.Errorf("failed to create config writer: %w", err)
	}

	// Generate the server's WireGuard keypair
	privKey, err := wg.GeneratePrivateKey()
	if err != nil {
		return fmt.Errorf("failed to generate wireguard keypair: %w", err)
	}
	pubKey := privKey.PublicKey()

	// Create root CIDR and initial server peer atomically in the store
	if err := ctx.Store.Create(ctx.Name, req.RootCidr, pubKey.String()); err != nil {
		return fmt.Errorf("failed to create network and server peer: %w", err)
	}

	// Persist the network identity
	cfg := &NetworkConfig{
		Name:             ctx.Name,
		PrivateKey:       privKey.String(),
		PublicKey:        pubKey.String(),
		RootCidr:         req.RootCidr.String(),
		InviteCidr:       req.InviteCidr.String(),
		ExternalIP:       req.ExternalIP.String(),
		ListenPort:       req.ListenPort,
		InviteListenPort: req.InvitePort,
		ApiPort:          req.ApiPort,
	}
	if err := ctx.SaveConfig(cfg); err != nil {
		return fmt.Errorf("failed to write network config: %w", err)
	}

	return nil
}

func (ctx *Context) DeleteNetwork() error {

	return ctx.Store.Delete(ctx.Name)
}
