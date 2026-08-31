package server

import "context"

// Cidr describes a named CIDR in a network.
type Cidr struct {
	Name string `json:"name"`
	Cidr string `json:"cidr"`
}

// CreateCidrRequest describes a CIDR to create.
type CreateCidrRequest struct {
	Name string `json:"name"`
	Cidr string `json:"cidr"`
}

// UpdateCidrRequest changes a CIDR's name.
type UpdateCidrRequest struct {
	Name string `json:"name"`
}

// CidrGroupRequest names a group to assign.
type CidrGroupRequest struct {
	Group string `json:"group"`
}

// ListCidrs lists a network's CIDRs.
func (c *Client) ListCidrs(
	ctx context.Context,
	network string,
) (
	[]Cidr,
	error,
) {
	var result []Cidr
	return result, c.wire.Get(ctx, "/networks/"+segment(network)+"/cidrs", &result)
}

// AddCidr creates a CIDR.
func (c *Client) AddCidr(
	ctx context.Context,
	network, name, cidr string,
) error {
	body, err := marshalJSON(CreateCidrRequest{
		Name: name,
		Cidr: cidr,
	})
	if err != nil {
		return err
	}
	return c.wire.Post(ctx, "/networks/"+segment(network)+"/cidrs", body, nil)
}

// RenameCidr changes a CIDR's name.
func (c *Client) RenameCidr(
	ctx context.Context,
	network, cidr, newName string,
) error {
	body, err := marshalJSON(UpdateCidrRequest{
		Name: newName,
	})
	if err != nil {
		return err
	}
	path := "/networks/" + segment(network) + "/cidrs/" + segment(cidr)
	return c.wire.Patch(ctx, path, body, nil)
}

// DeleteCidr removes a CIDR.
func (c *Client) DeleteCidr(
	ctx context.Context,
	network, cidr string,
) error {
	path := "/networks/" + segment(network) + "/cidrs/" + segment(cidr)
	return c.wire.Delete(ctx, path, nil)
}

// ListCidrGroups lists groups assigned to a CIDR.
func (c *Client) ListCidrGroups(
	ctx context.Context,
	network, cidr string,
) (
	[]Group,
	error,
) {
	var result []Group
	path := "/networks/" + segment(network) + "/cidrs/" + segment(cidr) + "/groups"
	return result, c.wire.Get(ctx, path, &result)
}

// AssignCidrGroup assigns a group to a CIDR.
func (c *Client) AssignCidrGroup(
	ctx context.Context,
	network, cidr, group string,
) error {
	body, err := marshalJSON(CidrGroupRequest{
		Group: group,
	})
	if err != nil {
		return err
	}
	path := "/networks/" + segment(network) + "/cidrs/" + segment(cidr) + "/groups"
	return c.wire.Post(ctx, path, body, nil)
}

// RemoveCidrGroup removes a group from a CIDR.
func (c *Client) RemoveCidrGroup(
	ctx context.Context,
	network, cidr, group string,
) error {
	path := "/networks/" + segment(network) + "/cidrs/" + segment(cidr) + "/groups/" + segment(group)
	return c.wire.Delete(ctx, path, nil)
}
