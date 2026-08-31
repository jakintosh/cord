package server

import "context"

// Association connects two groups.
type Association struct {
	Group1 string `json:"group1"`
	Group2 string `json:"group2"`
}

// CreateAssociationRequest describes an association to create.
type CreateAssociationRequest struct {
	Group1 string `json:"group1"`
	Group2 string `json:"group2"`
}

// ListAssociations lists a network's group associations.
func (c *Client) ListAssociations(
	ctx context.Context,
	network string,
) (
	[]Association,
	error,
) {
	var result []Association
	return result, c.wire.Get(ctx, "/networks/"+segment(network)+"/associations", &result)
}

// AddAssociation creates an association.
func (c *Client) AddAssociation(
	ctx context.Context,
	network, group1, group2 string,
) error {
	body, err := marshalJSON(CreateAssociationRequest{
		Group1: group1,
		Group2: group2,
	})
	if err != nil {
		return err
	}
	return c.wire.Post(ctx, "/networks/"+segment(network)+"/associations", body, nil)
}

// DeleteAssociation removes an association.
func (c *Client) DeleteAssociation(
	ctx context.Context,
	network, group1, group2 string,
) error {
	path := "/networks/" + segment(network) + "/associations/" + segment(group1) + "/" + segment(group2)
	return c.wire.Delete(ctx, path, nil)
}
