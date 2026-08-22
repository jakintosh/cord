package protocol_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/protocol"
)

func validInvitation() *protocol.Invitation {
	return &protocol.Invitation{
		Network: protocol.NetworkInfo{
			Name:        "testnet",
			PublicKey:   "server-key",
			Endpoint:    "1.2.3.4:51820",
			ServerRoute: "10.0.0.1/32",
			NetworkCidr: "10.0.0.0/16",
			APIPort:     8080,
		},
		Peer: protocol.PeerIdentity{
			Route:      "10.1.0.2/32",
			PrivateKey: "temp-key",
		},
	}
}

func TestParse_Success(t *testing.T) {
	input := `{
		"network": {
			"name": "testnet",
			"public_key": "server-key",
			"endpoint": "1.2.3.4:51820",
			"server_route": "10.0.0.1/32",
			"network_cidr": "10.0.0.0/16",
			"api_port": 8080
		},
		"peer": {
			"route": "10.1.0.2/32",
			"private_key": "temp-key"
		}
	}`

	inv, err := protocol.ParseInvitation(bytes.NewReader([]byte(input)))
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
	if inv.Network.NetworkCidr != "10.0.0.0/16" {
		t.Errorf("network_cidr = %q, want 10.0.0.0/16", inv.Network.NetworkCidr)
	}
	if inv.Network.APIPort != 8080 {
		t.Errorf("api_port = %d, want 8080", inv.Network.APIPort)
	}
}

func TestParse_InvalidJSON(t *testing.T) {
	input := `{not valid json`

	_, err := protocol.ParseInvitation(bytes.NewReader([]byte(input)))
	if !errors.Is(err, protocol.ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
}

func TestInvitation_Write(t *testing.T) {
	inv := validInvitation()

	var buf bytes.Buffer
	if err := inv.Write(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}

	var parsed protocol.Invitation
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal written: %v", err)
	}

	if parsed.Network.Name != "testnet" {
		t.Errorf("network_name = %q, want testnet", parsed.Network.Name)
	}
	if parsed.Network.PublicKey != "server-key" {
		t.Errorf("public_key = %q, want server-key", parsed.Network.PublicKey)
	}
}

func TestInvitation_Validate(t *testing.T) {
	if err := validInvitation().Validate(true); err != nil {
		t.Fatalf("valid invitation rejected: %v", err)
	}

	cases := map[string]func(*protocol.Invitation){
		"missing network name":     func(i *protocol.Invitation) { i.Network.Name = "" },
		"missing peer private key": func(i *protocol.Invitation) { i.Peer.PrivateKey = "" },
		"missing peer route":       func(i *protocol.Invitation) { i.Peer.Route = "" },
		"missing server pubkey":    func(i *protocol.Invitation) { i.Network.PublicKey = "" },
		"missing server endpoint":  func(i *protocol.Invitation) { i.Network.Endpoint = "" },
		"missing server route":     func(i *protocol.Invitation) { i.Network.ServerRoute = "" },
		"missing network cidr":     func(i *protocol.Invitation) { i.Network.NetworkCidr = "" },
		"missing api port":         func(i *protocol.Invitation) { i.Network.APIPort = 0 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			inv := validInvitation()
			mutate(inv)
			if err := inv.Validate(true); !errors.Is(err, protocol.ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestInvitation_Validate_Redeemed(t *testing.T) {
	inv := validInvitation()
	inv.Peer.PrivateKey = ""
	if err := inv.Validate(false); err != nil {
		t.Fatalf("redeemed invitation without private key rejected: %v", err)
	}

	cases := map[string]func(*protocol.Invitation){
		"missing network name":    func(i *protocol.Invitation) { i.Network.Name = "" },
		"missing peer route":      func(i *protocol.Invitation) { i.Peer.Route = "" },
		"missing server pubkey":   func(i *protocol.Invitation) { i.Network.PublicKey = "" },
		"missing server endpoint": func(i *protocol.Invitation) { i.Network.Endpoint = "" },
		"missing server route":    func(i *protocol.Invitation) { i.Network.ServerRoute = "" },
		"missing network cidr":    func(i *protocol.Invitation) { i.Network.NetworkCidr = "" },
		"missing api port":        func(i *protocol.Invitation) { i.Network.APIPort = 0 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			inv := validInvitation()
			inv.Peer.PrivateKey = ""
			mutate(inv)
			if err := inv.Validate(false); !errors.Is(err, protocol.ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
		})
	}
}
