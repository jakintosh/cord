package server

import (
	"context"

	"git.studiopollinator.com/pollinator/cord/pkg/invite"
)

// CreateRegistrationRequest describes a registration to create.
type CreateRegistrationRequest struct {
	Name  string `json:"name"`
	IP    string `json:"ip"`
	Admin bool   `json:"admin"`
}

// RegistrationGroupRequest names a group to assign.
type RegistrationGroupRequest struct {
	Group string `json:"group"`
}

// Registration describes a pending or redeemed registration.
type Registration struct {
	Name      string `json:"name"`
	Route     string `json:"route"`
	Admin     bool   `json:"admin"`
	Redeemed  bool   `json:"redeemed"`
	ExpiresAt string `json:"expires_at"`
}

// ListRegistrations lists registrations for a network.
func (c *Client) ListRegistrations(
	ctx context.Context,
	network string,
) (
	[]Registration,
	error,
) {
	var result []Registration
	return result, c.wire.Get(ctx, "/networks/"+segment(network)+"/registrations", &result)
}

// CreateInvite creates a registration and returns its invitation.
func (c *Client) CreateInvite(
	ctx context.Context,
	network, name, ip string,
	admin bool,
) (
	*invite.Invitation,
	error,
) {
	body, err := marshalJSON(CreateRegistrationRequest{
		Name:  name,
		IP:    ip,
		Admin: admin,
	})
	if err != nil {
		return nil, err
	}
	var result *invite.Invitation
	return result, c.wire.Post(ctx, "/networks/"+segment(network)+"/registrations", body, &result)
}

// RevokeRegistration revokes a registration.
func (c *Client) RevokeRegistration(
	ctx context.Context,
	network, registration string,
) error {
	path := "/networks/" + segment(network) + "/registrations/" + segment(registration)
	return c.wire.Delete(ctx, path, nil)
}

// ListRegistrationGroups lists groups assigned to a registration.
func (c *Client) ListRegistrationGroups(
	ctx context.Context,
	network, registration string,
) (
	[]Group,
	error,
) {
	var result []Group
	path := "/networks/" + segment(network) + "/registrations/" + segment(registration) + "/groups"
	return result, c.wire.Get(ctx, path, &result)
}

// AssignRegistrationGroup assigns a group to a registration.
func (c *Client) AssignRegistrationGroup(
	ctx context.Context,
	network, registration, group string,
) error {
	body, err := marshalJSON(RegistrationGroupRequest{
		Group: group,
	})
	if err != nil {
		return err
	}
	path := "/networks/" + segment(network) + "/registrations/" + segment(registration) + "/groups"
	return c.wire.Post(ctx, path, body, nil)
}

// RemoveRegistrationGroup removes a group from a registration.
func (c *Client) RemoveRegistrationGroup(
	ctx context.Context,
	network, registration, group string,
) error {
	path := "/networks/" + segment(network) + "/registrations/" + segment(registration) + "/groups/" + segment(group)
	return c.wire.Delete(ctx, path, nil)
}
