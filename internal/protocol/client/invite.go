package client

import (
	"context"
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
)

// --- InviteClient (invite network API) ---

// InviteClient is a typed HTTP client for the cord server's invite API.
type InviteClient struct {
	client wire.Client
}

// NewInviteClient returns an InviteClient for the given API address.
func NewInviteClient(
	apiAddr string,
	httpClient *http.Client,
) (
	*InviteClient,
	error,
) {
	c, err := wire.NewClient("http://"+apiAddr, wire.ClientOptions{HTTPClient: httpClient})
	if err != nil {
		return nil, err
	}
	return &InviteClient{client: c}, nil
}

// RedeemInvitation calls POST /redeem, exchanging a temporary invite
// key for a permanent peer identity and the main network server details.
func (c *InviteClient) RedeemInvitation(
	ctx context.Context,
	permPubKey string,
) (
	*protocol.Invitation,
	error,
) {
	req := protocol.RedeemRequest{
		PermPubKey: permPubKey,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var result protocol.Invitation
	err = withRetry(ctx, func(ctx context.Context) error {
		return c.client.Post(ctx, "/redeem", body, &result)
	})
	if err != nil {
		return nil, err
	}

	return &result, nil
}
