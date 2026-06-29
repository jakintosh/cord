package serverapi

import (
	"encoding/json"
	"net/http"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
)

// Client is a typed HTTP client for the cord server's peer-facing and
// invite-facing APIs. It communicates over the WireGuard tunnel using
// the wire envelope protocol ({data, error}).
type Client struct {
	main   wire.Client
	invite wire.Client
}

// NewClient returns a Client configured to reach the server's main and
// invite API listeners. Both addresses are in "host:port" form (plain
// HTTP, no TLS over the tunnel). An optional httpClient replaces the
// default transport for testing.
func NewClient(
	mainAddr string,
	inviteAddr string,
	httpClient *http.Client,
) *Client {
	return &Client{
		main: wire.Client{
			BaseURL:    "http://" + mainAddr,
			HTTPClient: httpClient,
		},
		invite: wire.Client{
			BaseURL:    "http://" + inviteAddr,
			HTTPClient: httpClient,
		},
	}
}

// ListPeers calls GET /peers on the main peer API and returns the
// visible peer list for the authenticated peer.
func (c *Client) ListPeers() (
	[]VisiblePeerDTO,
	error,
) {
	var peers []VisiblePeerDTO
	err := c.main.Get("/peers", &peers)
	return peers, err
}

// ConfirmPeer calls POST /confirm on the main peer API, proving
// WireGuard reachability from the assigned IP.
func (c *Client) ConfirmPeer() error {
	var result map[string]string
	return withRetry(func() error {
		return c.main.Post("/confirm", nil, &result)
	})
}

// ReportEndpoints calls POST /endpoints on the main peer API, sending
// locally-observed peer endpoints for gossip distribution.
func (c *Client) ReportEndpoints(
	sightings []EndpointSightingDTO,
) error {
	body, err := json.Marshal(sightings)
	if err != nil {
		return err
	}
	var result map[string]string
	return withRetry(func() error {
		return c.main.Post("/endpoints", body, &result)
	})
}

// RedeemInvite calls POST /redeem on the invite API, exchanging a
// temporary invite key for a permanent peer identity and the main
// network server details.
func (c *Client) RedeemInvite(
	req RedeemInviteRequest,
) (
	*RedeemResultDTO,
	error,
) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	var result RedeemResultDTO
	err = withRetry(func() error {
		return c.invite.Post("/redeem", body, &result)
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

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
