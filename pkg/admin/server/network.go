package server

import "context"

// Network describes a server-managed Cord network.
type Network struct {
	Name          string `json:"name"`
	ExternalIP    string `json:"external_ip"`
	MainName      string `json:"main_name"`
	MainCidr      string `json:"main_cidr"`
	MainWgPort    uint16 `json:"main_wg_port"`
	MainApiPort   uint16 `json:"main_api_port"`
	InviteName    string `json:"invite_name"`
	InviteCidr    string `json:"invite_cidr"`
	InviteWgPort  uint16 `json:"invite_wg_port"`
	InviteApiPort uint16 `json:"invite_api_port"`
	Enabled       bool   `json:"enabled"`
}

// CreateNetworkRequest describes a network to create.
type CreateNetworkRequest struct {
	Name          string  `json:"name"`
	ExternalIP    string  `json:"external_ip"`
	MainName      *string `json:"main_name,omitempty"`
	MainCidr      string  `json:"main_cidr"`
	MainWgPort    *uint16 `json:"main_wg_port,omitempty"`
	MainApiPort   *uint16 `json:"main_api_port,omitempty"`
	InviteName    *string `json:"invite_name,omitempty"`
	InviteCidr    *string `json:"invite_cidr,omitempty"`
	InviteWgPort  *uint16 `json:"invite_wg_port,omitempty"`
	InviteApiPort *uint16 `json:"invite_api_port,omitempty"`
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

// ShowNetwork returns one managed network.
func (c *Client) ShowNetwork(
	ctx context.Context,
	name string,
) (
	Network,
	error,
) {
	var result Network
	return result, c.wire.Get(ctx, "/networks/"+segment(name), &result)
}

// AddNetwork creates a network.
func (c *Client) AddNetwork(
	ctx context.Context,
	req CreateNetworkRequest,
) (
	Network,
	error,
) {
	body, err := marshalJSON(req)
	if err != nil {
		return Network{}, err
	}
	var result Network
	return result, c.wire.Post(ctx, "/networks", body, &result)
}

// DeleteNetwork removes a network.
func (c *Client) DeleteNetwork(
	ctx context.Context,
	name string,
) error {
	return c.wire.Delete(ctx, "/networks/"+segment(name), nil)
}

// EnableNetwork enables a network.
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

// DisableNetwork disables a network.
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
