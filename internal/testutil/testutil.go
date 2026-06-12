// Package testutil is the test composition root: it owns deterministic
// wiring of the store, service, and API routers for in-process tests.
package testutil

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"

	"git.sr.ht/~jakintosh/cord/internal/api"
	"git.sr.ht/~jakintosh/cord/internal/database"
	"git.sr.ht/~jakintosh/cord/internal/server"
	wg "git.sr.ht/~jakintosh/cord/internal/wireguard"
)

// Deterministic test network shape shared across API tests.
const (
	NetworkName = "test-network"
	RootCidr    = "10.0.0.0/16"
	InviteCidr  = "172.16.10.0/24"

	// AdminAddr is the cord-server peer's address: the network creator
	// is always a confirmed admin at the root's first assignable IP.
	AdminAddr = "10.0.0.1:40000"
)

// remoteAddrHeader carries a forged source address for tests. The API
// authenticates by connection source IP, which wire's in-process test
// helpers cannot set directly; a test-only wrapper applies it instead.
const remoteAddrHeader = "X-Test-Remote-Addr"

// FromAddr builds the test header that forges the request source address.
func FromAddr(addr string) wire.TestHeader {
	return wire.TestHeader{Key: remoteAddrHeader, Value: addr}
}

// withTestRemoteAddr rewrites RemoteAddr from the test header before
// the real routers see the request. Test wiring only.
func withTestRemoteAddr(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if addr := r.Header.Get(remoteAddrHeader); addr != "" {
			r.RemoteAddr = addr
		}
		next.ServeHTTP(w, r)
	})
}

// TestEnv is one fully wired in-memory cord network.
type TestEnv struct {
	Service      *server.Context
	Router       http.Handler
	InviteRouter http.Handler
	Mutations    int
}

// SetupTestEnv builds an in-memory network with the deterministic test
// shape and returns both API routers ready for in-process requests.
func SetupTestEnv(
	t *testing.T,
) *TestEnv {
	t.Helper()

	store, err := database.Init(NetworkName, ":memory:", false)
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}

	service, err := server.NewContext(NetworkName, server.NewMemConfig(), store)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}

	_, rootNet, _ := net.ParseCIDR(RootCidr)
	_, inviteNet, _ := net.ParseCIDR(InviteCidr)
	err = service.CreateNetwork(server.CreateNetworkRequest{
		RootCidr:   rootNet,
		InviteCidr: inviteNet,
		ExternalIP: net.IPv4(203, 0, 113, 1),
		ListenPort: 51820,
		InvitePort: 51821,
		ApiPort:    51820,
	})
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	env := &TestEnv{Service: service}

	apiOpts := api.Options{
		Service:    service,
		OnMutation: func() { env.Mutations++ },
	}
	apiServer, err := api.New(apiOpts)
	if err != nil {
		t.Fatalf("create api: %v", err)
	}

	env.Router = withTestRemoteAddr(apiServer.Router())
	env.InviteRouter = withTestRemoteAddr(apiServer.InviteRouter())
	return env
}

// NewPeerKey generates a fresh WireGuard public key for a test peer.
func NewPeerKey(
	t *testing.T,
) string {
	t.Helper()

	key, err := wg.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("generate peer key: %v", err)
	}
	return key.PublicKey().String()
}

// CreateInvite mints an invite through the admin API and returns the
// invite payload.
func CreateInvite(
	t *testing.T,
	env *TestEnv,
	name string,
	ip string,
	admin bool,
) api.PeerInviteDTO {
	t.Helper()

	body, err := json.Marshal(api.CreatePeerRequest{
		Name:  name,
		IP:    ip,
		Admin: admin,
	})
	if err != nil {
		t.Fatalf("encode invite request: %v", err)
	}

	result := wire.TestPost[api.PeerInviteDTO](
		env.Router, "/api/v1/admin/peer", string(body), FromAddr(AdminAddr),
	)
	return result.ExpectStatusOK(t, http.StatusCreated)
}

// InviteAddr returns the source address an invitee calls from on the
// invite network.
func InviteAddr(
	t *testing.T,
	invite api.PeerInviteDTO,
) string {
	t.Helper()

	ip, _, err := net.ParseCIDR(invite.Interface.AssignedCidr)
	if err != nil {
		t.Fatalf("parse invite cidr: %v", err)
	}
	return ip.String() + ":40000"
}

// RedeemInvite redeems an invite with a fresh permanent key, returning
// the key and the main-network assignment.
func RedeemInvite(
	t *testing.T,
	env *TestEnv,
	invite api.PeerInviteDTO,
) (
	string,
	api.RedeemResultDTO,
) {
	t.Helper()

	permKey := NewPeerKey(t)
	body := fmt.Sprintf(`{"publicKey": %q}`, permKey)
	result := wire.TestPost[api.RedeemResultDTO](
		env.InviteRouter, "/api/v1/invite/redeem", body, FromAddr(InviteAddr(t, invite)),
	)
	return permKey, result.ExpectStatusOK(t, http.StatusOK)
}

// JoinPeer walks a peer through the full join flow: invite, redeem
// from the invite network, confirm from the main network. Returns the
// peer's permanent public key.
func JoinPeer(
	t *testing.T,
	env *TestEnv,
	name string,
	ip string,
	admin bool,
) string {
	t.Helper()

	invite := CreateInvite(t, env, name, ip, admin)
	permKey, redeemed := RedeemInvite(t, env, invite)

	finalIP, _, err := net.ParseCIDR(redeemed.AssignedCidr)
	if err != nil {
		t.Fatalf("parse assigned cidr: %v", err)
	}

	body := fmt.Sprintf(`{"publicKey": %q}`, permKey)
	result := wire.TestPost[any](
		env.Router, "/api/v1/invite/confirm", body, FromAddr(finalIP.String()+":40000"),
	)
	result.ExpectStatus(t, http.StatusOK)

	return permKey
}
