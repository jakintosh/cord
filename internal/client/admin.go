package client

import (
	"git.sr.ht/~jakintosh/cord/internal/api"
	"git.sr.ht/~jakintosh/cord/internal/server"
)

// Admin performs remote administration of a cord server over its HTTP
// API. It requires an installed network and an admin peer identity;
// the server enforces the latter by source IP.
type Admin struct {
	api *apiClient
}

// Admin returns a remote administration handle for this network.
func (c *Client) Admin() (*Admin, error) {
	cfg, err := c.LoadConfig()
	if err != nil {
		return nil, err
	}
	return &Admin{api: newApiClient(cfg.Server.InternalEndpoint)}, nil
}

// AddPeer creates a peer invite. ExpiresIn is in seconds; zero uses
// the server default.
func (a *Admin) AddPeer(
	name string,
	ip string,
	admin bool,
	expiresIn int64,
) (
	*server.PeerInvite,
	error,
) {
	return a.api.adminCreatePeer(api.CreatePeerRequest{
		Name:      name,
		IP:        ip,
		Admin:     admin,
		ExpiresIn: expiresIn,
	})
}

func (a *Admin) RenamePeer(name string, newName string) (*server.Peer, error) {
	return a.api.adminUpdatePeer(name, api.UpdatePeerRequest{Name: &newName})
}

func (a *Admin) EnablePeer(name string) (*server.Peer, error) {
	enabled := true
	return a.api.adminUpdatePeer(name, api.UpdatePeerRequest{Enabled: &enabled})
}

func (a *Admin) DisablePeer(name string) (*server.Peer, error) {
	enabled := false
	return a.api.adminUpdatePeer(name, api.UpdatePeerRequest{Enabled: &enabled})
}

func (a *Admin) DeletePeer(name string) error {
	return a.api.adminDeletePeer(name)
}

func (a *Admin) AddCidr(name string, cidr string) (*server.Cidr, error) {
	return a.api.adminCreateCidr(api.CreateCidrRequest{Name: name, Cidr: cidr})
}

func (a *Admin) RenameCidr(name string, newName string) (*server.Cidr, error) {
	return a.api.adminRenameCidr(name, newName)
}

func (a *Admin) DeleteCidr(name string) error {
	return a.api.adminDeleteCidr(name)
}

func (a *Admin) AddAssociation(cidr1 string, cidr2 string) error {
	return a.api.adminCreateAssociation(cidr1, cidr2)
}

func (a *Admin) DeleteAssociation(cidr1 string, cidr2 string) error {
	return a.api.adminDeleteAssociation(cidr1, cidr2)
}
