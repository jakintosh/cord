package server

import "context"

// CreateGroupRequest describes a group to create.
type CreateGroupRequest struct {
	Name string `json:"name"`
}

// Group describes a network group.
type Group struct {
	Name string `json:"name"`
}

// ListGroups lists a network's groups.
func (c *Client) ListGroups(
	ctx context.Context,
	network string,
) (
	[]Group,
	error,
) {
	var result []Group
	return result, c.wire.Get(ctx, "/networks/"+segment(network)+"/groups", &result)
}

// CreateGroup creates a group.
func (c *Client) CreateGroup(
	ctx context.Context,
	network, name string,
) error {
	body, err := marshalJSON(CreateGroupRequest{
		Name: name,
	})
	if err != nil {
		return err
	}
	return c.wire.Post(ctx, "/networks/"+segment(network)+"/groups", body, nil)
}

// DeleteGroup removes a group.
func (c *Client) DeleteGroup(
	ctx context.Context,
	network, name string,
) error {
	path := "/networks/" + segment(network) + "/groups/" + segment(name)
	return c.wire.Delete(ctx, path, nil)
}
