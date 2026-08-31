package client

import (
	"context"
	"encoding/json"

	"git.studiopollinator.com/pollinator/cord/pkg/invite"
)

// InstallRequest installs a server-issued invitation with local settings.
type InstallRequest struct {
	Invitation invite.Invitation `json:"invitation"`
	ListenPort *uint16           `json:"listen_port"`
}

// UpdateNetworkRequest changes local network settings.
type UpdateNetworkRequest struct {
	ListenPort *uint16 `json:"listen_port,omitempty"`
}

// Network describes an installed or in-progress client network.
type Network struct {
	Name           string `json:"name"`
	State          string `json:"state"`
	Enabled        bool   `json:"enabled"`
	Address        string `json:"address,omitempty"`
	Interface      string `json:"interface,omitempty"`
	ListenPort     uint16 `json:"listen_port,omitempty"`
	ServerEndpoint string `json:"server_endpoint,omitempty"`
}

// ListNetworks lists managed network names.
func (c *Client) ListNetworks(
	ctx context.Context,
) (
	[]string,
	error,
) {
	var result []string
	return result, c.wire.Get(ctx, "/networks", &result)
}

// GetNetwork returns one managed network.
func (c *Client) GetNetwork(
	ctx context.Context,
	name string,
) (
	Network,
	error,
) {
	var result Network
	return result, c.wire.Get(ctx, "/networks/"+segment(name), &result)
}

// InstallNetwork begins installing an invitation.
func (c *Client) InstallNetwork(
	ctx context.Context,
	invitation invite.Invitation,
	listenPort *uint16,
) (
	Network,
	error,
) {
	body, err := json.Marshal(InstallRequest{
		Invitation: invitation,
		ListenPort: listenPort,
	})
	if err != nil {
		return Network{}, err
	}

	var result Network
	return result, c.wire.Post(ctx, "/networks", body, &result)
}

// UpdateNetwork changes local network settings.
func (c *Client) UpdateNetwork(
	ctx context.Context,
	name string,
	listenPort *uint16,
) (
	Network,
	error,
) {
	body, err := json.Marshal(UpdateNetworkRequest{
		ListenPort: listenPort,
	})
	if err != nil {
		return Network{}, err
	}
	var result Network
	return result, c.wire.Patch(ctx, "/networks/"+segment(name), body, &result)
}

// RedeemNetwork redeems an installed invitation.
func (c *Client) RedeemNetwork(
	ctx context.Context,
	name string,
) (
	Network,
	error,
) {
	var result Network
	return result, c.wire.Post(ctx, "/networks/"+segment(name)+"/redeem", nil, &result)
}

// ConfirmNetwork confirms an installed network over its permanent identity.
func (c *Client) ConfirmNetwork(
	ctx context.Context,
	name string,
) error {
	return c.wire.Post(ctx, "/networks/"+segment(name)+"/confirm", nil, nil)
}

// UninstallNetwork removes a managed network.
func (c *Client) UninstallNetwork(
	ctx context.Context,
	name string,
) error {
	return c.wire.Delete(ctx, "/networks/"+segment(name), nil)
}

// EnableNetwork enables a managed network.
func (c *Client) EnableNetwork(
	ctx context.Context,
	name string,
) (
	NetworkStatus,
	error,
) {
	var result NetworkStatus
	return result, c.wire.Post(ctx, "/networks/"+segment(name)+"/enable", nil, &result)
}

// DisableNetwork disables a managed network.
func (c *Client) DisableNetwork(
	ctx context.Context,
	name string,
) (
	NetworkStatus,
	error,
) {
	var result NetworkStatus
	return result, c.wire.Post(ctx, "/networks/"+segment(name)+"/disable", nil, &result)
}

// SyncNetwork synchronizes a managed network with its server.
func (c *Client) SyncNetwork(
	ctx context.Context,
	name string,
) (
	Network,
	error,
) {
	var result Network
	return result, c.wire.Post(ctx, "/networks/"+segment(name)+"/sync", nil, &result)
}
