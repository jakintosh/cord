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
}

// withRetry retries a call on transport errors and 5xx responses.
// 4xx responses are not retried — they indicate a client-side problem
// that repeating won't fix. All server endpoints called through this
// client are idempotent by design.
func withRetry(
	ctx context.Context,
	fn func(context.Context) error,
) error {
	var lastErr error
	for attempt := range retryMaxAttempts {
		err := fn(ctx)
		if err == nil {
			return nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if httpErr, ok := wire.AsHTTPError(err); ok {
			if httpErr.StatusCode >= 400 && httpErr.StatusCode < 500 {
				return err
			}
		}

		if attempt < retryMaxAttempts-1 {
			timer := time.NewTimer(retryBackoffs[attempt])
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
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
func (c *PeerClient) GetSnapshot(
	ctx context.Context,
) (
	protocol.VisibleNetworkSnapshot,
	error,
) {
	var snapshot protocol.VisibleNetworkSnapshot
	err := c.client.Get(ctx, "/snapshot", &snapshot)
	return snapshot, err
}

// ConfirmPeer calls POST /confirm, proving WireGuard reachability.
func (c *PeerClient) ConfirmPeer(
	ctx context.Context,
) error {
	var result protocol.StatusResponse
	return withRetry(ctx, func(ctx context.Context) error {
		return c.client.Post(ctx, "/confirm", nil, &result)
	})
}

// ReportEndpoints calls POST /endpoints, sending locally-observed
// peer endpoints for gossip distribution.
func (c *PeerClient) ReportEndpoints(
	ctx context.Context,
	sightings []protocol.EndpointSighting,
) error {
	body, err := json.Marshal(sightings)
	if err != nil {
		return err
	}

	var result protocol.StatusResponse
	return withRetry(ctx, func(ctx context.Context) error {
		return c.client.Post(ctx, "/endpoints", body, &result)
	})
}
