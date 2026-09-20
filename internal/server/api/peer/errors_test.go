package peer_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/peer"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestAPIReportEndpoints_AuthenticatedFailures(
	t *testing.T,
) {
	for _, tc := range []struct {
		name    string
		status  int
		message string
	}{
		{"remove witness", http.StatusNotFound, "resource not found"},
		{"remove network", http.StatusNotFound, "resource not found"},
		{"stop", http.StatusServiceUnavailable, "service unavailable"},
		{"close database", http.StatusInternalServerError, "internal server error"},
		{"cancel", 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Setup authenticated and observed peers with real storage.
			env := testutil.SetupRuntime(t)
			testutil.SeedNetwork(t, env.Service)
			testutil.SeedPeerDB(t, env.Database, "testnet", "alice", "10.0.0.5/32", "alice-key", false, true, true)
			testutil.SeedPeerDB(t, env.Database, "testnet", "bob", "10.0.0.6/32", "bob-key", false, true, true)
			var logs bytes.Buffer
			apiRouter := peer.New(env.Runtime, slog.New(slog.NewTextHandler(&logs, nil))).Router("testnet")
			router := fromPeer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx, cancel := context.WithCancel(r.Context())
				defer cancel()
				r = r.WithContext(ctx)
				r.Body = &beforeEndpointBody{ReadCloser: r.Body, before: func() {
					var err error
					switch tc.name {
					case "remove witness":
						err = env.Service.RemovePeer("testnet", "alice")
					case "remove network":
						err = env.Service.DeleteNetwork("testnet")
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
			}), "10.0.0.5:12345")

			// Reading the report orders the transition after successful authentication.
			url := "/endpoints"
			body := `[{"peer_key":"bob-key", "endpoint":"1.2.3.4:51820"}]`
			result := wire.TestPost[protocol.StatusResponse](router, url, body)

			// Verify disclosure and cancellation behavior.
			if tc.name == "cancel" {
				if len(result.Raw) != 0 || strings.Contains(logs.String(), "level=ERROR") {
					t.Fatalf("canceled request wrote response or error log: %s, %s", result.Raw, logs.String())
				}
			} else if apiErr := result.ExpectStatusError(t, tc.status); apiErr.Message != tc.message {
				t.Fatalf("error = %q", apiErr.Message)
			}
			if tc.name == "close database" {
				for _, want := range []string{"database is closed", "operation=\"report endpoints\"", "network=testnet"} {
					if !strings.Contains(logs.String(), want) {
						t.Fatalf("log missing %q: %s", want, logs.String())
					}
				}
				return
			}
			if strings.Contains(logs.String(), "level=ERROR") {
				t.Fatalf("expected rejection logged as server error: %s", logs.String())
			}
			endpoints, err := env.Database.GetRecentEndpoints("testnet", time.Time{})
			if err != nil || len(endpoints) != 0 {
				t.Fatalf("rejection persisted observations: %+v, %v", endpoints, err)
			}
		})
	}
}

// The body is read only after identity lookup, giving a deterministic ordering
// for a real state transition between authentication and the operation.
type beforeEndpointBody struct {
	io.ReadCloser
	before func()
}

func (b *beforeEndpointBody) Read(p []byte) (int, error) {
	if b.before != nil {
		before := b.before
		b.before = nil
		before()
	}
	return b.ReadCloser.Read(p)
}
