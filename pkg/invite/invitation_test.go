package invite_test

import (
	"bytes"
	"errors"
	"testing"

	"git.studiopollinator.com/pollinator/cord/pkg/invite"
)

func TestInvitationRoundTrip(t *testing.T) {
	original := &invite.Invitation{
		Network: invite.NetworkInfo{
			Name:        "example",
			PublicKey:   "server-key",
			Endpoint:    "example.test:51820",
			ServerRoute: "10.42.0.1/32",
			NetworkCidr: "10.42.0.0/16",
			APIPort:     8080,
		},
		Peer: invite.PeerIdentity{
			Route:      "10.42.0.2/32",
			PrivateKey: "peer-key",
		},
	}
	var encoded bytes.Buffer
	if err := original.Write(&encoded); err != nil {
		t.Fatalf("write invitation: %v", err)
	}

	parsed, err := invite.Parse(&encoded)
	if err != nil {
		t.Fatalf("parse invitation: %v", err)
	}
	if err := parsed.Validate(true); err != nil {
		t.Fatalf("validate invitation: %v", err)
	}
	if parsed.Network.Name != original.Network.Name {
		t.Fatalf("network name = %q, want %q", parsed.Network.Name, original.Network.Name)
	}
}

func TestParseInvalidInvitation(t *testing.T) {
	_, err := invite.Parse(bytes.NewBufferString("{"))
	if !errors.Is(err, invite.ErrInvalid) {
		t.Fatalf("error = %v, want ErrInvalid", err)
	}
}
