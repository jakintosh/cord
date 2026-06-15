package wireguard

import (
	"net"
	"strings"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func TestPlanPeerReconciliation_NoOpPreservesLearnedEndpoint(t *testing.T) {
	key := wgtypes.Key{1}
	learned := udpAddr(t, "198.51.100.10:51820")
	desired := []Peer{{
		PublicKey:      key,
		AllowedIPs:     allowedIPs(t, "10.0.0.2/32"),
		EndpointPolicy: EndpointDynamic,
	}}
	observed := []PeerStatus{{
		PublicKey:  key,
		AllowedIPs: allowedIPs(t, "10.0.0.2/32"),
		Endpoint:   learned,
	}}

	plan := PlanPeerReconciliation(desired, observed)

	if len(plan.Operations) != 0 {
		t.Fatalf("operations = %v, want none", plan.Operations)
	}
}

func TestPlanPeerReconciliation_AllowedIPOrderIsNotDrift(t *testing.T) {
	key := wgtypes.Key{1}
	desired := []Peer{{
		PublicKey:  key,
		AllowedIPs: allowedIPs(t, "10.0.0.2/32", "192.0.2.0/24"),
	}}
	observed := []PeerStatus{{
		PublicKey:  key,
		AllowedIPs: allowedIPs(t, "192.0.2.0/24", "10.0.0.2/32"),
	}}

	plan := PlanPeerReconciliation(desired, observed)

	if len(plan.Operations) != 0 {
		t.Fatalf("operations = %v, want none", plan.Operations)
	}
}

func TestPlanPeerReconciliation_AddRemoveAndUpdate(t *testing.T) {
	removeKey := wgtypes.Key{1}
	addKey := wgtypes.Key{2}
	updateKey := wgtypes.Key{3}
	desired := []Peer{
		{
			PublicKey:           addKey,
			AllowedIPs:          allowedIPs(t, "10.0.0.2/32"),
			Endpoint:            udpAddr(t, "203.0.113.10:51820"),
			EndpointPolicy:      EndpointBootstrap,
			PersistentKeepalive: 25 * time.Second,
		},
		{
			PublicKey:           updateKey,
			AllowedIPs:          allowedIPs(t, "10.0.0.3/32"),
			Endpoint:            udpAddr(t, "203.0.113.20:51820"),
			EndpointPolicy:      EndpointFixed,
			PersistentKeepalive: 25 * time.Second,
		},
	}
	observed := []PeerStatus{
		{PublicKey: removeKey, AllowedIPs: allowedIPs(t, "10.0.0.9/32")},
		{
			PublicKey:           updateKey,
			AllowedIPs:          allowedIPs(t, "10.0.0.30/32"),
			Endpoint:            udpAddr(t, "203.0.113.30:51820"),
			PersistentKeepalive: 0,
		},
	}

	plan := PlanPeerReconciliation(desired, observed)

	if len(plan.Operations) != 3 {
		t.Fatalf("operations = %d, want 3: %v", len(plan.Operations), plan.Operations)
	}
	if plan.Operations[0].Type != PeerRemove || plan.Operations[0].Peer.PublicKey != removeKey {
		t.Fatalf("first operation = %v, want remove", plan.Operations[0])
	}
	if plan.Operations[1].Type != PeerAdd || plan.Operations[1].Peer.PublicKey != addKey {
		t.Fatalf("second operation = %v, want add", plan.Operations[1])
	}
	update := plan.Operations[2]
	if update.Type != PeerUpdate || update.Peer.PublicKey != updateKey {
		t.Fatalf("third operation = %v, want update", update)
	}
	if !update.UpdateAllowedIPs || !update.UpdateEndpoint || !update.UpdateKeepalive {
		t.Fatalf("update flags = %+v, want all fields", update)
	}
}

func TestPlanPeerReconciliation_BootstrapEndpointDoesNotOverrideRoaming(t *testing.T) {
	key := wgtypes.Key{1}
	desired := []Peer{{
		PublicKey:      key,
		AllowedIPs:     allowedIPs(t, "10.0.0.2/32"),
		Endpoint:       udpAddr(t, "203.0.113.10:51820"),
		EndpointPolicy: EndpointBootstrap,
	}}
	observed := []PeerStatus{{
		PublicKey:  key,
		AllowedIPs: allowedIPs(t, "10.0.0.2/32"),
		Endpoint:   udpAddr(t, "198.51.100.20:51820"),
	}}

	plan := PlanPeerReconciliation(desired, observed)

	if len(plan.Operations) != 0 {
		t.Fatalf("operations = %v, want none", plan.Operations)
	}
}

func TestWgPeerConfig_TargetedOperations(t *testing.T) {
	keepalive := 25 * time.Second
	peer := Peer{
		PublicKey:           wgtypes.Key{1},
		AllowedIPs:          allowedIPs(t, "10.0.0.2/32"),
		Endpoint:            udpAddr(t, "203.0.113.10:51820"),
		EndpointPolicy:      EndpointBootstrap,
		PersistentKeepalive: keepalive,
	}

	add := wgPeerConfig(PeerOperation{Type: PeerAdd, Peer: peer})
	if add.UpdateOnly || add.Remove || !add.ReplaceAllowedIPs || add.Endpoint == nil {
		t.Fatalf("unexpected add config: %+v", add)
	}

	update := wgPeerConfig(PeerOperation{
		Type:             PeerUpdate,
		Peer:             peer,
		UpdateAllowedIPs: true,
	})
	if !update.UpdateOnly || !update.ReplaceAllowedIPs || update.Endpoint != nil ||
		update.PersistentKeepaliveInterval != nil {
		t.Fatalf("unexpected update config: %+v", update)
	}

	remove := wgPeerConfig(PeerOperation{Type: PeerRemove, Peer: peer})
	if !remove.Remove {
		t.Fatalf("unexpected remove config: %+v", remove)
	}
}

func TestPeerOperationsUAPI_DoesNotReplacePeers(t *testing.T) {
	peer := Peer{
		PublicKey:      wgtypes.Key{1},
		AllowedIPs:     allowedIPs(t, "10.0.0.2/32"),
		EndpointPolicy: EndpointDynamic,
	}

	raw := peerOperationsUAPI([]PeerOperation{{Type: PeerAdd, Peer: peer}})

	if strings.Contains(raw, "replace_peers=true") {
		t.Fatalf("targeted operations contain full replacement: %s", raw)
	}
	if !strings.Contains(raw, "replace_allowed_ips=true") {
		t.Fatalf("add operation missing allowed IP replacement: %s", raw)
	}
}

func allowedIPs(t *testing.T, values ...string) []net.IPNet {
	t.Helper()
	ips := make([]net.IPNet, 0, len(values))
	for _, value := range values {
		_, cidr, err := net.ParseCIDR(value)
		if err != nil {
			t.Fatalf("parse CIDR %q: %v", value, err)
		}
		ips = append(ips, *cidr)
	}
	return ips
}

func udpAddr(t *testing.T, value string) *net.UDPAddr {
	t.Helper()
	addr, err := net.ResolveUDPAddr("udp", value)
	if err != nil {
		t.Fatalf("resolve endpoint %q: %v", value, err)
	}
	return addr
}
