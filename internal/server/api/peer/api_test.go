package peer_test

import (
	"net"
	"net/http"
	"testing"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/peer"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestAPIRouter_RejectsUnknownAndDisabledSources(
	t *testing.T,
) {
	for _, source := range []string{"10.0.0.99:12345", "10.0.0.5:12345"} {
		t.Run(source, func(
			t *testing.T,
		) {
			// Setup a disabled source and an enabled identity to spoof.
			env := testutil.SetupRuntime(t)
			testutil.SeedNetwork(t, env.Service)
			testutil.SeedPeerDB(t, env.Database, "testnet", "disabled", "10.0.0.5/32", "disabled-key", false, false, true)
			testutil.SeedPeerDB(t, env.Database, "testnet", "alice", "10.0.0.6/32", "alice-key", false, true, true)
			router := fromPeer(peer.New(env.Runtime, nil).Router("testnet"), source)
			headers := []wire.TestHeader{{Key: "X-Forwarded-For", Value: "10.0.0.6"}}

			// Every route authenticates the source despite a spoofed header.
			results := []wire.TestResult[any]{
				wire.TestGet[any](router, "/snapshot", headers...),
				wire.TestPost[any](router, "/endpoints", `[]`, headers...),
				wire.TestPost[any](router, "/confirm", "", headers...),
			}
			// Verify opaque identity errors.
			for _, result := range results {
				apiErr := result.ExpectStatusError(t, http.StatusForbidden)
				if apiErr.Message != "identity unknown" {
					t.Fatalf("error = %+v", apiErr)
				}
			}
			// Rejected writes cannot create observations.
			endpoints, err := env.Database.GetRecentEndpoints("testnet", time.Time{})
			if err != nil || len(endpoints) != 0 {
				t.Fatalf("endpoints = %+v, err = %v", endpoints, err)
			}
		})
	}
}

func fromPeer(
	router http.Handler,
	address string,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.RemoteAddr = address
		router.ServeHTTP(w, r)
	})
}

func TestAPIRouter_ConfirmationDoesNotMaskLookupFailure(
	t *testing.T,
) {
	// Cache a confirmed identity that would succeed if used as a fallback.
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	testutil.SeedPeerDB(t, env.Database, "testnet", "alice", "10.0.0.5/32", "alice-key", false, true, true)
	if _, err := env.Runtime.ResolvePeerIdentity(t.Context(), "testnet", net.ParseIP("10.0.0.5")); err != nil {
		t.Fatal(err)
	}
	if err := env.Database.Close(); err != nil {
		t.Fatal(err)
	}
	router := fromPeer(peer.New(env.Runtime, nil).Router("testnet"), "10.0.0.5:12345")

	// A storage failure in provisional lookup must not fall back to that cache.
	result := wire.TestPost[any](router, "/confirm", "")
	apiErr := result.ExpectStatusError(t, http.StatusForbidden)
	if apiErr.Message != "identity unknown" {
		t.Fatalf("error = %q", apiErr.Message)
	}
}
