package invite_test

import (
	"bytes"
	"errors"
	"net"
	"net/http"
	"reflect"
	"testing"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/invite"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
	invitation "git.studiopollinator.com/pollinator/cord/pkg/invite"
)

func TestAPIRedeemInvite_ReturnsPermanentIdentityAndRetries(
	t *testing.T,
) {
	// Setup a real registration with an assigned permanent route.
	env := testutil.SetupRuntime(t)
	network := testutil.SeedNetwork(t, env.Service)
	initial, err := env.Service.CreateRegistration("testnet", "invitee", service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.50")})
	if err != nil {
		t.Fatal(err)
	}
	router := inviteSource(t, invite.New(env.Runtime, nil).Router("testnet"), initial.Peer.Route)

	// Request redemption using the temporary identity's source address.
	url := "/redeem"
	body := `{
		"perm_pubkey": "new-perm-key"
	}`
	headers := []wire.TestHeader{{Key: "Content-Type", Value: "application/json"}}
	result := wire.TestPost[invitation.Invitation](router, url, body, headers...)

	// Verify the complete main-plane invitation, excluding secret material.
	got := result.ExpectStatusOK(t, http.StatusOK)
	want := invitation.Invitation{
		Network: invitation.NetworkInfo{
			Name:        "testnet",
			PublicKey:   network.PublicKey,
			Endpoint:    "192.168.1.1:51820",
			ServerRoute: "10.0.0.1/32",
			NetworkCidr: "10.0.0.0/16",
			APIPort:     8080,
		},
		Peer: invitation.PeerIdentity{Route: "10.0.0.50/32"},
	}
	if got != want || bytes.Contains(result.Raw, []byte(`"private_key"`)) {
		t.Fatalf("invitation = %+v, want %+v; raw: %s", got, want, result.Raw)
	}

	// Verify durable redemption and its provisional peer.
	reg, err := env.Database.GetRegistration("testnet", "invitee")
	if err != nil {
		t.Fatal(err)
	}
	if !reg.Redeemed || reg.RedeemedKey != "new-perm-key" || reg.Confirmed || reg.InviteRoute != initial.Peer.Route || reg.MainRoute != want.Peer.Route {
		t.Fatalf("registration = %+v", reg)
	}
	peer, err := env.Service.GetPeer("testnet", "invitee")
	if err != nil {
		t.Fatal(err)
	}
	if peer.Name != "invitee" || peer.PublicKey != "new-perm-key" || peer.Route != want.Peer.Route || peer.Confirmed || !peer.Enabled {
		t.Fatalf("peer = %+v", peer)
	}

	// Retry the same request and verify the same response and durable state.
	retry := wire.TestPost[invitation.Invitation](router, url, body, headers...)
	if retried := retry.ExpectStatusOK(t, http.StatusOK); retried != want {
		t.Fatalf("retry = %+v, want %+v", retried, want)
	}
	afterReg, err := env.Database.GetRegistration("testnet", "invitee")
	if err != nil {
		t.Fatal(err)
	}
	afterPeer, err := env.Service.GetPeer("testnet", "invitee")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reg, afterReg) || !reflect.DeepEqual(peer, afterPeer) {
		t.Fatalf("retry changed registration or peer: %+v, %+v", afterReg, afterPeer)
	}
}

func TestAPIRedeemInvite_RejectedRequestsPreserveRegistration(
	t *testing.T,
) {
	for _, tc := range []struct {
		name    string
		body    string
		unknown bool
		expiry  time.Duration
		status  int
		message string
	}{
		{name: "invalid JSON", body: "{", status: http.StatusBadRequest, message: "invalid JSON"},
		{name: "unknown identity", body: `{"perm_pubkey": "new-perm-key"}`, unknown: true, status: http.StatusForbidden, message: "identity unknown"},
		{name: "expired identity", body: `{"perm_pubkey": "new-perm-key"}`, expiry: -time.Hour, status: http.StatusForbidden, message: "identity unknown"},
		{name: "expired identity with malformed JSON", body: "{", expiry: -time.Hour, status: http.StatusForbidden, message: "identity unknown"},
	} {
		t.Run(tc.name, func(
			t *testing.T,
		) {
			// Setup an isolated registration; negative expiry makes it already expired.
			env := testutil.SetupRuntime(t)
			testutil.SeedNetwork(t, env.Service)
			initial, err := env.Service.CreateRegistration("testnet", "invitee", service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.50"), ExpiresIn: &tc.expiry})
			if err != nil {
				t.Fatal(err)
			}
			before, err := env.Database.GetRegistration("testnet", "invitee")
			if err != nil {
				t.Fatal(err)
			}
			source := initial.Peer.Route
			if tc.unknown {
				source = "10.1.0.99/32"
			}
			router := inviteSource(t, invite.New(env.Runtime, nil).Router("testnet"), source)

			// Request with explicit JSON headers and the scenario's body.
			url := "/redeem"
			body := tc.body
			headers := []wire.TestHeader{{Key: "Content-Type", Value: "application/json"}}
			result := wire.TestPost[invitation.Invitation](router, url, body, headers...)

			// Verify status before the error envelope.
			apiErr := result.ExpectStatusError(t, tc.status)
			if apiErr.Message != tc.message {
				t.Fatalf("error = %q, want %q", apiErr.Message, tc.message)
			}

			// Verify rejection leaves the registration intact and creates no peer.
			after, err := env.Database.GetRegistration("testnet", "invitee")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("registration changed: before %+v, after %+v", before, after)
			}
			if _, err := env.Service.GetPeer("testnet", "invitee"); !errors.Is(err, service.ErrNotFound) {
				t.Fatalf("peer after rejection: %v", err)
			}
		})
	}
}

func inviteSource(
	t *testing.T,
	next http.Handler,
	route string,
) http.Handler {
	t.Helper()
	ip, _, err := net.ParseCIDR(route)
	if err != nil {
		t.Fatal(err)
	}
	return http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		r.RemoteAddr = net.JoinHostPort(ip.String(), "12345")
		next.ServeHTTP(w, r)
	})
}
