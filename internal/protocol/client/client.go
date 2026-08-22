// Package client provides typed HTTP clients for the cord server's peer
// and invite APIs, speaking the wire types defined in package protocol.
package client

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
)

// --- Shared helpers ---

// retryMaxAttempts is the maximum number of attempts for retried calls.
const retryMaxAttempts = 3

// retryBackoffs are the delays between retry attempts.
var retryBackoffs = []time.Duration{
	200 * time.Millisecond,
	1 * time.Second,
	5 * time.Second,
}

// withRetry retries a call on transport errors and 5xx responses.
// 4xx responses are not retried — they indicate a client-side problem
// that repeating won't fix. All server endpoints called through this
// client are idempotent by design.
func withRetry(fn func() error) error {
	var lastErr error
	for attempt := range retryMaxAttempts {
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err

		if httpErr, ok := wire.AsHTTPError(err); ok {
			if httpErr.StatusCode >= 400 && httpErr.StatusCode < 500 {
				return err
			}
		}

		if attempt < retryMaxAttempts-1 {
			time.Sleep(retryBackoffs[attempt])
		}
	}
	return lastErr
}

// --- PeerClient (main network API) ---

// PeerClient is a typed HTTP client for the cord server's peer-facing
// API. It communicates over the WireGuard tunnel.
type PeerClient struct {
	client wire.Client
}

// NewPeerClient returns a PeerClient for the given API address
// ("host:port" form). An optional httpClient replaces the default
// transport for testing.
func NewPeerClient(
	apiAddr string,
	httpClient *http.Client,
) (
	*PeerClient,
	error,
) {
	opts := wire.ClientOptions{
		HTTPClient: httpClient,
	}
	c, err := wire.NewClient("http://"+apiAddr, opts)
	if err != nil {
		return nil, err
	}
	return &PeerClient{
		client: c,
	}, nil
}

// GetSnapshot calls GET /snapshot and returns the caller's complete visible
// network snapshot.
func (c *PeerClient) GetSnapshot() (
	protocol.VisibleNetworkSnapshot,
	error,
) {
	var snapshot protocol.VisibleNetworkSnapshot
	err := c.client.Get(context.Background(), "/snapshot", &snapshot)
	return snapshot, err
}

// ConfirmPeer calls POST /confirm, proving WireGuard reachability.
func (c *PeerClient) ConfirmPeer() error {
	var result protocol.StatusResponse
	return withRetry(func() error {
		// TODO: at some point we should figure out a real context to use here
		return c.client.Post(context.Background(), "/confirm", nil, &result)
	})
}

// ReportEndpoints calls POST /endpoints, sending locally-observed
// peer endpoints for gossip distribution.
func (c *PeerClient) ReportEndpoints(
	sightings []protocol.EndpointSighting,
) error {
	body, err := json.Marshal(sightings)
	if err != nil {
		return err
	}

	var result protocol.StatusResponse
	return withRetry(func() error {
		return c.client.Post(context.Background(), "/endpoints", body, &result)
	})
}

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
	err = withRetry(func() error {
		return c.client.Post(context.Background(), "/redeem", body, &result)
	})
	if err != nil {
		return nil, err
	}

	return &result, nil
}
