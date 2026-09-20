package invite_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/invite"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
	invitation "git.studiopollinator.com/pollinator/cord/pkg/invite"
)

func TestAPIRedeemInvite_AuthenticatedFailures(
	t *testing.T,
) {
	for _, tc := range []struct {
		name     string
		redeemed bool
		key      string
		action   string
		status   int
		message  string
	}{
		{"different key", true, "different-key", "", http.StatusConflict, "registration is no longer redeemable"},
		{"peer key conflict", false, "taken-key", "", http.StatusConflict, "resource conflict"},
		{"first redemption expires", false, "perm-key", "expire", http.StatusGone, "registration expired"},
		{"retry expires", true, "perm-key", "expire", http.StatusGone, "registration expired"},
		{"registration revoked", false, "perm-key", "revoke", http.StatusNotFound, "resource not found"},
		{"network removed", false, "perm-key", "delete network", http.StatusNotFound, "resource not found"},
		{"confirmed concurrently", true, "perm-key", "confirm", http.StatusConflict, "registration is no longer redeemable"},
		{"runtime stopped", false, "perm-key", "stop", http.StatusServiceUnavailable, "service unavailable"},
		{"database failure", false, "perm-key", "close database", http.StatusInternalServerError, "internal server error"},
		{"request canceled", false, "perm-key", "cancel", 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Setup a real invitation and a clock independent of wall time.
			var seconds atomic.Int64
			seconds.Store(testutil.FixedTime.Unix())
			env := testutil.SetupRuntimeWithClock(t, func() time.Time { return time.Unix(seconds.Load(), 0) })
			testutil.SeedNetwork(t, env.Service)
			testutil.SeedPeerDB(t, env.Database, "testnet", "existing", "10.0.0.60/32", "taken-key", false, true, true)
			expiry := time.Hour
			initial, err := env.Service.CreateRegistration("testnet", "invitee", service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.50"), ExpiresIn: &expiry})
			if err != nil {
				t.Fatal(err)
			}
			reg, err := env.Database.GetRegistration("testnet", "invitee")
			if err != nil {
				t.Fatal(err)
			}
			if tc.redeemed {
				if _, err := env.Service.RedeemRegistration("testnet", reg.InvitePublicKey, "perm-key"); err != nil {
					t.Fatal(err)
				}
			}
			before, err := env.Database.GetRegistration("testnet", "invitee")
			if err != nil {
				t.Fatal(err)
			}
			var logs bytes.Buffer
			apiRouter := invite.New(env.Runtime, slog.New(slog.NewTextHandler(&logs, nil))).Router("testnet")
			router := inviteSource(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx, cancel := context.WithCancel(r.Context())
				defer cancel()
				r = r.WithContext(ctx)
				// The handler reads the body only after real identity authentication.
				r.Body = &beforeInviteBody{ReadCloser: r.Body, before: func() {
					var err error
					switch tc.action {
					case "expire":
						seconds.Store(reg.ExpiresAt.Unix())
					case "revoke":
						err = env.Service.RevokeRegistration("testnet", "invitee")
					case "delete network":
						err = env.Service.DeleteNetwork("testnet")
					case "confirm":
						err = env.Service.ConfirmPeer("testnet", "invitee")
					case "stop":
						err = env.Stop()
					case "close database":
						err = env.Database.Close()
					case "cancel":
						cancel()
					}
					if err != nil {
						t.Fatal(err)
					}
				}}
				apiRouter.ServeHTTP(w, r)
			}), initial.Peer.Route)

			// Request redemption; the chosen transition happens between lookup and operation.
			url := "/redeem"
			body := `{"perm_pubkey": "` + tc.key + `"}`
			result := wire.TestPost[invitation.Invitation](router, url, body)

			// Verify stable public messages and diagnostic disclosure policy.
			if tc.action == "cancel" {
				if len(result.Raw) != 0 || strings.Contains(logs.String(), "level=ERROR") {
					t.Fatalf("canceled request wrote response or error log: %s, %s", result.Raw, logs.String())
				}
			} else {
				apiErr := result.ExpectStatusError(t, tc.status)
				if apiErr.Message != tc.message {
					t.Fatalf("error = %q, want %q", apiErr.Message, tc.message)
				}
			}
			if tc.action == "close database" {
				for _, want := range []string{"database is closed", "operation=\"redeem invite\"", "network=testnet"} {
					if !strings.Contains(logs.String(), want) {
						t.Fatalf("log missing %q: %s", want, logs.String())
					}
				}
				return
			}
			if strings.Contains(logs.String(), "level=ERROR") {
				t.Fatalf("expected rejection logged as server error: %s", logs.String())
			}

			// Rejected redemption must not add or replace provisional state.
			if tc.action == "revoke" || tc.action == "delete network" {
				if _, err := env.Database.GetRegistration("testnet", "invitee"); !errors.Is(err, service.ErrNotFound) {
					t.Fatalf("registration recreated: %v", err)
				}
			} else {
				after, err := env.Database.GetRegistration("testnet", "invitee")
				if err != nil {
					t.Fatal(err)
				}
				if tc.action == "confirm" {
					before.Confirmed = true
				}
				if !reflect.DeepEqual(before, after) {
					t.Fatalf("registration changed: before %+v, after %+v", before, after)
				}
			}
			p, err := env.Database.GetPeer("testnet", "invitee")
			if tc.redeemed {
				if err != nil || p.PublicKey != "perm-key" || p.Confirmed != (tc.action == "confirm") {
					t.Fatalf("provisional state = %+v, %v", p, err)
				}
			} else if !errors.Is(err, service.ErrNotFound) {
				t.Fatalf("rejection created peer: %+v, %v", p, err)
			}
		})
	}
}

// beforeInviteBody orders a real state transition after authentication without
// adding application fault hooks or depending on goroutine timing.
type beforeInviteBody struct {
	io.ReadCloser
	before func()
}

func (b *beforeInviteBody) Read(p []byte) (int, error) {
	if b.before != nil {
		before := b.before
		b.before = nil
		before()
	}
	return b.ReadCloser.Read(p)
}
