package peer_test

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/protocol/client"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/peer"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestAPIReportEndpoints_AttributesCallerAndReceiptTime(
	t *testing.T,
) {
	// Setup all peers before the runtime loads its topology model.
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	testutil.SeedPeerDB(t, env.Database, "testnet", "alice", "10.0.0.5/32", "alice-key", false, true, true)
	testutil.SeedPeerDB(t, env.Database, "testnet", "bob", "10.0.0.6/32", "bob-key", false, true, true)
	router := fromPeer(peer.New(env.Runtime, nil).Router("testnet"), "10.0.0.5:12345")

	// Report an endpoint with forged attribution fields.
	url := "/endpoints"
	body := `[
		{
			"peer_key": "bob-key",
			"endpoint": "1.2.3.4:51820",
			"witness_key": "bob-key",
			"timestamp": "2000-01-01T00:00:00Z"
		}
	]`
	start := time.Now().Truncate(time.Second)
	result := wire.TestPost[protocol.StatusResponse](router, url, body)
	end := time.Now()

	// Verify the response.
	data := result.ExpectOK(t)
	if data.Status != "ok" {
		t.Fatalf("status = %q", data.Status)
	}

	// Verify durable attribution and receipt time; SQLite stores whole seconds.
	endpoints, err := env.Database.GetRecentEndpoints("testnet", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(endpoints) != 1 || len(endpoints["bob-key"]) != 1 {
		t.Fatalf("endpoints = %+v", endpoints)
	}
	sighting := endpoints["bob-key"][0]
	if sighting.Witness != "alice-key" || sighting.Endpoint != "1.2.3.4:51820" || sighting.Timestamp.Before(start) || sighting.Timestamp.After(end) {
		t.Fatalf("sighting = %+v, receipt window %v to %v", sighting, start, end)
	}
}

func TestAPIReportEndpoints_InvalidJSONLeavesStateUnchanged(
	t *testing.T,
) {
	// Setup an authenticated caller.
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	testutil.SeedPeerDB(t, env.Database, "testnet", "alice", "10.0.0.5/32", "alice-key", false, true, true)
	router := fromPeer(peer.New(env.Runtime, nil).Router("testnet"), "10.0.0.5:12345")

	// Send malformed JSON.
	url := "/endpoints"
	body := `{`
	result := wire.TestPost[protocol.StatusResponse](router, url, body)

	// Verify the error envelope.
	apiErr := result.ExpectStatusError(t, http.StatusBadRequest)
	if apiErr.Message != "invalid JSON" {
		t.Fatalf("error = %+v", apiErr)
	}

	// Verify no observations were written.
	endpoints, err := env.Database.GetRecentEndpoints("testnet", time.Time{})
	if err != nil || len(endpoints) != 0 {
		t.Fatalf("endpoints = %+v, err = %v", endpoints, err)
	}
}

func TestAPIConfirmPeer_EnablesAuthenticatedOperations(
	t *testing.T,
) {
	// Setup a real redeemed registration.
	var seconds atomic.Int64
	seconds.Store(testutil.FixedTime.Unix())
	env := testutil.SetupRuntimeWithClock(t, func() time.Time { return time.Unix(seconds.Load(), 0) })
	testutil.SeedNetwork(t, env.Service)
	if _, err := env.Service.CreateRegistration("testnet", "alice", service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.5")}); err != nil {
		t.Fatal(err)
	}
	regs, err := env.Service.ListRegistrations("testnet")
	if err != nil || len(regs) != 1 {
		t.Fatalf("registrations = %+v, err = %v", regs, err)
	}
	if _, err := env.Service.RedeemRegistration("testnet", regs[0].InvitePublicKey, "alice-key"); err != nil {
		t.Fatal(err)
	}
	router := fromPeer(peer.New(env.Runtime, nil).Router("testnet"), "10.0.0.5:12345")

	// A provisional source cannot read snapshots or report observations.
	snapshotURL := "/snapshot"
	endpointsURL := "/endpoints"
	body := "[]"
	wire.TestGet[protocol.VisibleNetworkSnapshot](router, snapshotURL).ExpectStatusError(t, http.StatusForbidden)
	wire.TestPost[protocol.StatusResponse](router, endpointsURL, body).ExpectStatusError(t, http.StatusForbidden)

	// Rejected operations leave the peer provisional and create no observations.
	provisional, err := env.Database.GetPeer("testnet", "alice")
	if err != nil || provisional.Confirmed {
		t.Fatalf("provisional = %+v, err = %v", provisional, err)
	}
	endpoints, err := env.Database.GetRecentEndpoints("testnet", time.Time{})
	if err != nil || len(endpoints) != 0 {
		t.Fatalf("endpoints = %+v, err = %v", endpoints, err)
	}

	// Confirm through the route that also supports confirmed retries.
	url := "/confirm"
	result := wire.TestPost[protocol.StatusResponse](router, url, "")

	// Verify the response.
	data := result.ExpectOK(t)
	if data.Status != "confirmed" {
		t.Fatalf("status = %q", data.Status)
	}
	// Once confirmed, invitation expiry does not invalidate confirmation retries.
	seconds.Store(regs[0].ExpiresAt.Add(time.Hour).Unix())
	if retry := wire.TestPost[protocol.StatusResponse](router, url, "").ExpectOK(t); retry != data {
		t.Fatalf("confirmation retry = %+v, want %+v", retry, data)
	}

	// Verify durable confirmation and subsequent authentication.
	p, err := env.Database.GetPeer("testnet", "alice")
	if err != nil || !p.Confirmed || !p.Enabled || p.PublicKey != "alice-key" || p.Route != "10.0.0.5/32" {
		t.Fatalf("peer = %+v, err = %v", p, err)
	}
	regs, err = env.Service.ListRegistrations("testnet")
	if err != nil || len(regs) != 1 || !regs[0].Confirmed || !regs[0].Redeemed || regs[0].RedeemedKey != "alice-key" {
		t.Fatalf("registrations = %+v, err = %v", regs, err)
	}
	snapshot := wire.TestGet[protocol.VisibleNetworkSnapshot](router, snapshotURL).ExpectOK(t)
	if snapshot.Topology.SubjectPeer != "alice" {
		t.Fatalf("subject = %q", snapshot.Topology.SubjectPeer)
	}
	wire.TestPost[protocol.StatusResponse](router, endpointsURL, body).ExpectOK(t)
}

func TestAPIConfirmPeer_ExpiredBeforePruning(
	t *testing.T,
) {
	// Setup a redeemed peer and advance to its registration deadline.
	var seconds atomic.Int64
	seconds.Store(testutil.FixedTime.Unix())
	env := testutil.SetupRuntimeWithClock(t, func() time.Time { return time.Unix(seconds.Load(), 0) })
	testutil.SeedNetwork(t, env.Service)
	if _, err := env.Service.CreateRegistration("testnet", "alice", service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.5")}); err != nil {
		t.Fatal(err)
	}
	reg, err := env.Database.GetRegistration("testnet", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Service.RedeemRegistration("testnet", reg.InvitePublicKey, "alice-key"); err != nil {
		t.Fatal(err)
	}
	seconds.Store(reg.ExpiresAt.Unix())
	router := fromPeer(peer.New(env.Runtime, nil).Router("testnet"), "10.0.0.5:12345")

	// Expired but unpruned peers authenticate; initial confirmation must fail.
	url := "/confirm"
	result := wire.TestPost[protocol.StatusResponse](router, url, "")
	if apiErr := result.ExpectStatusError(t, http.StatusGone); apiErr.Message != "registration expired" {
		t.Fatalf("error = %q", apiErr.Message)
	}
	p, err := env.Database.GetPeer("testnet", "alice")
	if err != nil || p.Confirmed {
		t.Fatalf("peer after rejection = %+v, %v", p, err)
	}

	// Once pruned, the same identity is rejected opaquely before the handler.
	if err := env.Service.PruneNetwork("testnet"); err != nil {
		t.Fatal(err)
	}
	pruned := wire.TestPost[protocol.StatusResponse](router, url, "")
	if apiErr := pruned.ExpectStatusError(t, http.StatusForbidden); apiErr.Message != "identity unknown" {
		t.Fatalf("error = %q", apiErr.Message)
	}
}

func TestAPIConfirmPeer_RetriesLostResponse(
	t *testing.T,
) {
	// Setup a real provisional identity and the protocol client's HTTP transport.
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	if _, err := env.Service.CreateRegistration("testnet", "alice", service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.5")}); err != nil {
		t.Fatal(err)
	}
	reg, err := env.Database.GetRegistration("testnet", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Service.RedeemRegistration("testnet", reg.InvitePublicKey, "alice-key"); err != nil {
		t.Fatal(err)
	}
	router := fromPeer(peer.New(env.Runtime, nil).Router("testnet"), "10.0.0.5:12345")
	attempts := 0
	c, err := client.NewPeerClient("cord.test", &http.Client{Transport: confirmationTransport(func(r *http.Request) (*http.Response, error) {
		attempts++
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("attempt %d: %d %s", attempts, w.Code, w.Body.String())
		}
		if attempts == 1 {
			return nil, errors.New("confirmation response lost")
		}
		return w.Result(), nil
	})})
	if err != nil {
		t.Fatal(err)
	}

	// The first operation commits but its response is lost; the client retries.
	if err := c.ConfirmPeer(t.Context()); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	confirmed, err := env.Database.GetRegistration("testnet", "alice")
	if err != nil || !confirmed.Confirmed || confirmed.RedeemedKey != "alice-key" {
		t.Fatalf("registration = %+v, %v", confirmed, err)
	}
}

type confirmationTransport func(*http.Request) (*http.Response, error)

func (f confirmationTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAPIReportEndpoints_UnknownPeerRejectsWholeBatch(
	t *testing.T,
) {
	// Setup a caller and a valid observed peer.
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	testutil.SeedPeerDB(t, env.Database, "testnet", "alice", "10.0.0.5/32", "alice-key", false, true, true)
	testutil.SeedPeerDB(t, env.Database, "testnet", "bob", "10.0.0.6/32", "bob-key", false, true, true)
	router := fromPeer(peer.New(env.Runtime, nil).Router("testnet"), "10.0.0.5:12345")

	// A valid observation followed by an unknown peer fails as one batch.
	url := "/endpoints"
	body := `[
		{"peer_key":"bob-key", "endpoint":"1.2.3.4:51820"},
		{"peer_key":"unknown-key", "endpoint":"2.3.4.5:51820"}
	]`
	result := wire.TestPost[protocol.StatusResponse](router, url, body)

	// A missing referenced peer has a stable public error.
	apiErr := result.ExpectStatusError(t, http.StatusNotFound)
	if apiErr.Message != "resource not found" {
		t.Fatalf("error = %q", apiErr.Message)
	}

	// The failed batch does not retain the valid first observation.
	endpoints, err := env.Database.GetRecentEndpoints("testnet", time.Time{})
	if err != nil || len(endpoints) != 0 {
		t.Fatalf("endpoints = %+v, err = %v", endpoints, err)
	}
}
