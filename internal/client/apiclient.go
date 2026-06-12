package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"

	"git.sr.ht/~jakintosh/cord/internal/api"
	"git.sr.ht/~jakintosh/cord/internal/server"
)

// apiClient talks to a cord server's HTTP API over a WireGuard network.
// It speaks the API's DTO contract and converts to server types at the
// boundary so the rest of the client never handles wire shapes.
type apiClient struct {
	wire wire.Client
}

// newApiClient targets an internal "host:port" endpoint.
func newApiClient(internalEndpoint string) *apiClient {
	return &apiClient{
		wire: wire.Client{
			BaseURL:    "http://" + internalEndpoint + "/api/v1",
			HTTPClient: &http.Client{Timeout: 10 * time.Second},
		},
	}
}

// postJSON encodes body and issues a POST.
func (c *apiClient) postJSON(path string, body any, response any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to encode request: %w", err)
	}
	return c.wire.Post(path, payload, response)
}

// postJSONWithRetry retries a POST with linear backoff; redemption and
// confirmation ride on a WireGuard tunnel that may still be handshaking.
func (c *apiClient) postJSONWithRetry(
	attempts int,
	delay time.Duration,
	path string,
	body any,
	response any,
) error {
	var err error
	for i := range attempts {
		if i > 0 {
			time.Sleep(delay * time.Duration(i))
		}
		if err = c.postJSON(path, body, response); err == nil {
			return nil
		}
	}
	return fmt.Errorf("gave up after %d attempts: %w", attempts, err)
}

func (c *apiClient) redeem(pubKey string) (*server.RedeemResult, error) {
	var dto api.RedeemResultDTO
	err := c.postJSONWithRetry(
		5, 2*time.Second,
		"/invite/redeem", api.KeyRequest{PublicKey: pubKey},
		&dto,
	)
	if err != nil {
		return nil, err
	}

	result := dto.ToServer()
	return &result, nil
}

func (c *apiClient) confirm(pubKey string) error {
	return c.postJSONWithRetry(
		5, 2*time.Second,
		"/invite/confirm", api.KeyRequest{PublicKey: pubKey},
		nil,
	)
}

func (c *apiClient) peers() ([]server.PublicPeer, error) {
	var dtos []api.PublicPeerDTO
	if err := c.wire.Get("/peers", &dtos); err != nil {
		return nil, err
	}

	peers := make([]server.PublicPeer, 0, len(dtos))
	for _, dto := range dtos {
		peers = append(peers, dto.ToServer())
	}
	return peers, nil
}

func (c *apiClient) reportEndpoints(sightings []server.EndpointSighting) error {
	dtos := make([]api.EndpointSightingDTO, 0, len(sightings))
	for _, sighting := range sightings {
		dtos = append(dtos, api.EndpointSightingDTO{
			PeerKey:   sighting.PeerKey,
			Endpoint:  sighting.Endpoint,
			Timestamp: sighting.Timestamp,
		})
	}
	return c.postJSON("/endpoint", dtos, nil)
}

// admin operations

func (c *apiClient) adminCreatePeer(req api.CreatePeerRequest) (*server.PeerInvite, error) {
	var dto api.PeerInviteDTO
	if err := c.postJSON("/admin/peer", req, &dto); err != nil {
		return nil, err
	}

	invite := dto.ToServer()
	return &invite, nil
}

func (c *apiClient) adminUpdatePeer(name string, req api.UpdatePeerRequest) (*server.Peer, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
	}

	var dto api.PeerDTO
	if err := c.wire.Patch("/admin/peer/"+url.PathEscape(name), payload, &dto); err != nil {
		return nil, err
	}

	peer := dto.ToServer()
	return &peer, nil
}

func (c *apiClient) adminDeletePeer(name string) error {
	return c.wire.Delete("/admin/peer/"+url.PathEscape(name), nil)
}

func (c *apiClient) adminCreateCidr(req api.CreateCidrRequest) (*server.Cidr, error) {
	var dto api.CidrDTO
	if err := c.postJSON("/admin/cidr", req, &dto); err != nil {
		return nil, err
	}

	cidr := dto.ToServer()
	return &cidr, nil
}

func (c *apiClient) adminRenameCidr(name string, newName string) (*server.Cidr, error) {
	payload, err := json.Marshal(api.RenameCidrRequest{Name: newName})
	if err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
	}

	var dto api.CidrDTO
	if err := c.wire.Patch("/admin/cidr/"+url.PathEscape(name), payload, &dto); err != nil {
		return nil, err
	}

	cidr := dto.ToServer()
	return &cidr, nil
}

func (c *apiClient) adminDeleteCidr(name string) error {
	return c.wire.Delete("/admin/cidr/"+url.PathEscape(name), nil)
}

func (c *apiClient) adminCreateAssociation(cidr1, cidr2 string) error {
	return c.postJSON("/admin/association", api.AssociationDTO{Cidr1: cidr1, Cidr2: cidr2}, nil)
}

func (c *apiClient) adminDeleteAssociation(cidr1, cidr2 string) error {
	path := "/admin/association/" + url.PathEscape(cidr1) + "/" + url.PathEscape(cidr2)
	return c.wire.Delete(path, nil)
}
