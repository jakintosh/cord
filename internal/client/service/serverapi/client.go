package serverapi

import (
	"encoding/json"
	"net/http"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
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
) *PeerClient {
	return &PeerClient{
		client: wire.Client{
			BaseURL:    "http://" + apiAddr,
			HTTPClient: httpClient,
		},
	}
}

// ListPeers calls GET /peers and returns the visible peer list.
func (c *PeerClient) ListPeers() (
	[]VisiblePeerDTO,
	error,
) {
	var peers []VisiblePeerDTO
	err := c.client.Get("/peers", &peers)
	return peers, err
}

// ConfirmPeer calls POST /confirm, proving WireGuard reachability.
func (c *PeerClient) ConfirmPeer() error {
	var result map[string]string
	return withRetry(func() error {
		return c.client.Post("/confirm", nil, &result)
	})
}

// ReportEndpoints calls POST /endpoints, sending locally-observed
// peer endpoints for gossip distribution.
func (c *PeerClient) ReportEndpoints(
	sightings []EndpointSightingDTO,
) error {
	body, err := json.Marshal(sightings)
	if err != nil {
		return err
	}

	var result map[string]string
	return withRetry(func() error {
		return c.client.Post("/endpoints", body, &result)
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
) *InviteClient {
	return &InviteClient{
		client: wire.Client{
			BaseURL:    "http://" + apiAddr,
			HTTPClient: httpClient,
		},
	}
}

// RedeemInvitation calls POST /redeem, exchanging a temporary invite
// key for a permanent peer identity and the main network server details.
func (c *InviteClient) RedeemInvitation(
	permPubKey string,
) (
	*InvitationDTO,
	error,
) {
	req := RedeemInvitationRequest{
		PermPubKey: permPubKey,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var result InvitationDTO
	err = withRetry(func() error {
		return c.client.Post("/redeem", body, &result)
	})
	if err != nil {
		return nil, err
	}

	return &result, nil
}
