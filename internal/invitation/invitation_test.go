package invitation_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/invitation"
)

func TestParse_Success(t *testing.T) {
	input := `{
		"network": {
			"name": "testnet",
			"public_key": "server-key",
			"endpoint": "1.2.3.4:51820",
			"server_route": "10.0.0.1/32",
			"api_port": 8080
		},
		"peer": {
			"route": "10.1.0.2/32",
			"private_key": "temp-key"
		}
	}`

	inv, err := invitation.Parse(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if inv.Network.Name != "testnet" {
		t.Errorf("network_name = %q, want testnet", inv.Network.Name)
	}
	if inv.Peer.PrivateKey != "temp-key" {
		t.Errorf("private_key = %q, want temp-key", inv.Peer.PrivateKey)
	}
	if inv.Peer.Route != "10.1.0.2/32" {
		t.Errorf("route = %q, want 10.1.0.2/32", inv.Peer.Route)
	}
	if inv.Network.PublicKey != "server-key" {
		t.Errorf("server public_key = %q, want server-key", inv.Network.PublicKey)
	}
	if inv.Network.Endpoint != "1.2.3.4:51820" {
		t.Errorf("endpoint = %q, want 1.2.3.4:51820", inv.Network.Endpoint)
	}
	if inv.Network.ServerRoute != "10.0.0.1/32" {
		t.Errorf("server_route = %q, want 10.0.0.1/32", inv.Network.ServerRoute)
	}
	if inv.Network.APIPort != 8080 {
		t.Errorf("api_port = %d, want 8080", inv.Network.APIPort)
	}
}

func TestParse_InvalidJSON(t *testing.T) {
	input := `{not valid json`

	_, err := invitation.Parse(bytes.NewReader([]byte(input)))
	if !errors.Is(err, invitation.ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
}

func TestInvitation_Write(t *testing.T) {
	inv := &invitation.Invitation{
		Network: invitation.NetworkInfo{
			Name:        "mynet",
			PublicKey:   "pub",
			Endpoint:    "1.2.3.4:51820",
			ServerRoute: "10.0.0.1/32",
			APIPort:     8080,
		},
		Peer: invitation.PeerIdentity{
			Route:      "10.0.0.5/32",
			PrivateKey: "priv",
		},
	}

	var buf bytes.Buffer
	if err := inv.Write(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}

	var parsed invitation.Invitation
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal written: %v", err)
	}

	if parsed.Network.Name != "mynet" {
		t.Errorf("network_name = %q, want mynet", parsed.Network.Name)
	}
	if parsed.Network.PublicKey != "pub" {
		t.Errorf("public_key = %q, want pub", parsed.Network.PublicKey)
	}
}
