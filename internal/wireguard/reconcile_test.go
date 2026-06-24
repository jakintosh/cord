package wireguard

import (
	"net"
	"sort"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func mustGenerateKey(t *testing.T) wgtypes.Key {
	t.Helper()
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

func mustParseCIDR(t *testing.T, s string) net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatalf("parse CIDR %q: %v", s, err)
	}
	return *n
}

func mustResolveUDP(t *testing.T, s string) *net.UDPAddr {
	t.Helper()
	addr, err := net.ResolveUDPAddr("udp", s)
	if err != nil {
		t.Fatalf("resolve UDP %q: %v", s, err)
	}
	return addr
}

func mkDesired(key wgtypes.Key, allowedIPs []string, endpoint string, policy EndpointPolicy, keepalive time.Duration) desiredPeer {
	var ep *net.UDPAddr
	if endpoint != "" {
		addr, err := net.ResolveUDPAddr("udp", endpoint)
		if err != nil {
			panic(err)
		}
		ep = addr
	}
	var ips []net.IPNet
	for _, cidr := range allowedIPs {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(err)
		}
		ips = append(ips, *n)
	}
	return desiredPeer{
		PublicKey:           key,
		AllowedIPs:          ips,
		Endpoint:            ep,
		EndpointPolicy:      policy,
		PersistentKeepalive: keepalive,
	}
}

func mkObserved(key wgtypes.Key, allowedIPs []string, endpoint string, keepalive time.Duration) ObservedPeer {
	var ep *net.UDPAddr
	if endpoint != "" {
		addr, err := net.ResolveUDPAddr("udp", endpoint)
		if err != nil {
			panic(err)
		}
		ep = addr
	}
	var ips []net.IPNet
	for _, cidr := range allowedIPs {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(err)
		}
		ips = append(ips, *n)
	}
	return ObservedPeer{
		PublicKey:           key,
		AllowedIPs:          ips,
		Endpoint:            ep,
		PersistentKeepalive: keepalive,
	}
}

func TestPlanPeerReconciliation_EmptyBoth(t *testing.T) {
	plan := PlanPeerReconciliation(nil, nil)
	if len(plan.Operations) != 0 {
		t.Errorf("expected no operations, got %d", len(plan.Operations))
	}
}

func TestPlanPeerReconciliation_AllNew(t *testing.T) {
	k1, k2, k3 := mustGenerateKey(t), mustGenerateKey(t), mustGenerateKey(t)
	desired := []desiredPeer{
		mkDesired(k1, []string{"10.0.0.1/32"}, "", EndpointDynamic, 0),
		mkDesired(k2, []string{"10.0.0.2/32"}, "", EndpointDynamic, 0),
		mkDesired(k3, []string{"10.0.0.3/32"}, "", EndpointDynamic, 0),
	}

	plan := PlanPeerReconciliation(desired, nil)
	adds, updates, removes := plan.OperationCounts()
	if adds != 3 {
		t.Errorf("adds = %d, want 3", adds)
	}
	if updates != 0 {
		t.Errorf("updates = %d, want 0", updates)
	}
	if removes != 0 {
		t.Errorf("removes = %d, want 0", removes)
	}
}

func TestPlanPeerReconciliation_AllRemoved(t *testing.T) {
	k1, k2, k3 := mustGenerateKey(t), mustGenerateKey(t), mustGenerateKey(t)
	observed := []ObservedPeer{
		mkObserved(k1, []string{"10.0.0.1/32"}, "", 0),
		mkObserved(k2, []string{"10.0.0.2/32"}, "", 0),
		mkObserved(k3, []string{"10.0.0.3/32"}, "", 0),
	}

	plan := PlanPeerReconciliation(nil, observed)
	adds, _, removes := plan.OperationCounts()
	if removes != 3 {
		t.Errorf("removes = %d, want 3", removes)
	}
	if adds != 0 {
		t.Errorf("adds = %d, want 0", adds)
	}
}

func TestPlanPeerReconciliation_ExactMatch(t *testing.T) {
	k1, k2 := mustGenerateKey(t), mustGenerateKey(t)
	desired := []desiredPeer{
		mkDesired(k1, []string{"10.0.0.1/32"}, "", EndpointDynamic, 0),
		mkDesired(k2, []string{"10.0.0.2/32"}, "", EndpointDynamic, 0),
	}
	observed := []ObservedPeer{
		mkObserved(k1, []string{"10.0.0.1/32"}, "", 0),
		mkObserved(k2, []string{"10.0.0.2/32"}, "", 0),
	}

	plan := PlanPeerReconciliation(desired, observed)
	if len(plan.Operations) != 0 {
		t.Errorf("expected 0 operations, got %d", len(plan.Operations))
	}
}

func TestPlanPeerReconciliation_Mixed(t *testing.T) {
	kA, kB, kC, kD := mustGenerateKey(t), mustGenerateKey(t), mustGenerateKey(t), mustGenerateKey(t)
	desired := []desiredPeer{
		mkDesired(kA, []string{"10.0.0.1/32"}, "", EndpointDynamic, 0),
		mkDesired(kB, []string{"10.0.0.2/32"}, "", EndpointDynamic, 0),
		mkDesired(kC, []string{"10.0.0.3/32"}, "", EndpointDynamic, 0),
	}
	observed := []ObservedPeer{
		mkObserved(kB, []string{"10.0.0.2/32"}, "", 0),
		mkObserved(kD, []string{"10.0.0.4/32"}, "", 0),
	}

	plan := PlanPeerReconciliation(desired, observed)
	adds, updates, removes := plan.OperationCounts()
	if adds != 2 {
		t.Errorf("adds = %d, want 2 (A and C)", adds)
	}
	if removes != 1 {
		t.Errorf("removes = %d, want 1 (D)", removes)
	}
	if updates != 0 {
		t.Errorf("updates = %d, want 0", updates)
	}
}

func TestPlanPeerReconciliation_AllowedIPsChanged(t *testing.T) {
	k := mustGenerateKey(t)
	desired := []desiredPeer{
		mkDesired(k, []string{"10.0.1.0/24"}, "", EndpointDynamic, 0),
	}
	observed := []ObservedPeer{
		mkObserved(k, []string{"10.0.0.0/24"}, "", 0),
	}

	plan := PlanPeerReconciliation(desired, observed)
	adds, updates, _ := plan.OperationCounts()
	if updates != 1 || adds != 0 {
		t.Errorf("expected 1 update, got add=%d update=%d", adds, updates)
	}
	if !plan.Operations[0].UpdateAllowedIPs {
		t.Error("expected UpdateAllowedIPs=true")
	}
}

func TestPlanPeerReconciliation_KeepaliveChanged(t *testing.T) {
	k := mustGenerateKey(t)
	desired := []desiredPeer{
		mkDesired(k, []string{"10.0.0.1/32"}, "", EndpointDynamic, 25*time.Second),
	}
	observed := []ObservedPeer{
		mkObserved(k, []string{"10.0.0.1/32"}, "", 0),
	}

	plan := PlanPeerReconciliation(desired, observed)
	_, updates, _ := plan.OperationCounts()
	if updates != 1 {
		t.Errorf("updates = %d, want 1", updates)
	}
	if !plan.Operations[0].UpdateKeepalive {
		t.Error("expected UpdateKeepalive=true")
	}
}

func TestPlanPeerReconciliation_EndpointFixedChanged(t *testing.T) {
	k := mustGenerateKey(t)
	desired := []desiredPeer{
		mkDesired(k, []string{"10.0.0.1/32"}, "1.2.3.4:51820", EndpointFixed, 0),
	}
	observed := []ObservedPeer{
		mkObserved(k, []string{"10.0.0.1/32"}, "5.6.7.8:51821", 0),
	}

	plan := PlanPeerReconciliation(desired, observed)
	_, updates, _ := plan.OperationCounts()
	if updates != 1 {
		t.Errorf("updates = %d, want 1", updates)
	}
	if !plan.Operations[0].UpdateEndpoint {
		t.Error("expected UpdateEndpoint=true")
	}
}

func TestPlanPeerReconciliation_EndpointDynamicIgnoresChange(t *testing.T) {
	k := mustGenerateKey(t)
	desired := []desiredPeer{
		mkDesired(k, []string{"10.0.0.1/32"}, "1.2.3.4:51820", EndpointDynamic, 0),
	}
	observed := []ObservedPeer{
		mkObserved(k, []string{"10.0.0.1/32"}, "5.6.7.8:51821", 0),
	}

	plan := PlanPeerReconciliation(desired, observed)
	if len(plan.Operations) != 0 {
		t.Errorf("expected no operations for EndpointDynamic, got %d", len(plan.Operations))
	}
}

func TestPlanPeerReconciliation_EndpointBootstrapIgnoresChange(t *testing.T) {
	k := mustGenerateKey(t)
	desired := []desiredPeer{
		mkDesired(k, []string{"10.0.0.1/32"}, "1.2.3.4:51820", EndpointBootstrap, 0),
	}
	observed := []ObservedPeer{
		mkObserved(k, []string{"10.0.0.1/32"}, "5.6.7.8:51821", 0),
	}

	plan := PlanPeerReconciliation(desired, observed)
	if len(plan.Operations) != 0 {
		t.Errorf("expected no operations for EndpointBootstrap, got %d", len(plan.Operations))
	}
}

func TestPlanPeerReconciliation_NoOp(t *testing.T) {
	k := mustGenerateKey(t)
	desired := []desiredPeer{
		mkDesired(k, []string{"10.0.0.1/32"}, "1.2.3.4:51820", EndpointFixed, 10*time.Second),
	}
	observed := []ObservedPeer{
		mkObserved(k, []string{"10.0.0.1/32"}, "1.2.3.4:51820", 10*time.Second),
	}

	plan := PlanPeerReconciliation(desired, observed)
	if len(plan.Operations) != 0 {
		t.Errorf("expected no operations for exact match, got %d", len(plan.Operations))
	}
}

func TestPlanPeerReconciliation_MultipleChanges(t *testing.T) {
	k := mustGenerateKey(t)
	desired := []desiredPeer{
		mkDesired(k, []string{"10.0.1.0/24"}, "1.2.3.4:51820", EndpointFixed, 25*time.Second),
	}
	observed := []ObservedPeer{
		mkObserved(k, []string{"10.0.0.0/24"}, "5.6.7.8:51821", 0),
	}

	plan := PlanPeerReconciliation(desired, observed)
	_, updates, _ := plan.OperationCounts()
	if updates != 1 {
		t.Fatalf("updates = %d, want 1", updates)
	}
	op := plan.Operations[0]
	if !op.UpdateAllowedIPs {
		t.Error("expected UpdateAllowedIPs=true")
	}
	if !op.UpdateEndpoint {
		t.Error("expected UpdateEndpoint=true")
	}
	if !op.UpdateKeepalive {
		t.Error("expected UpdateKeepalive=true")
	}
}

func TestPlanPeerReconciliation_OperationOrdering(t *testing.T) {
	kA, kB, kC := mustGenerateKey(t), mustGenerateKey(t), mustGenerateKey(t)
	// kA: remove (in desired but not observed, wait no — set it up as
	// kA: observed only → remove
	// kB: desired only → add
	// kC: both but changed → update
	desired := []desiredPeer{
		mkDesired(kB, []string{"10.0.0.2/32"}, "", EndpointDynamic, 0),
		mkDesired(kC, []string{"10.0.1.0/24"}, "", EndpointDynamic, 0),
	}
	observed := []ObservedPeer{
		mkObserved(kA, []string{"10.0.0.1/32"}, "", 0),
		mkObserved(kC, []string{"10.0.0.0/24"}, "", 0),
	}

	plan := PlanPeerReconciliation(desired, observed)
	if len(plan.Operations) != 3 {
		t.Fatalf("expected 3 operations, got %d", len(plan.Operations))
	}
	// Order: removes, adds, updates
	if plan.Operations[0].Type != PeerRemove {
		t.Errorf("first op type = %s, want remove", plan.Operations[0].Type)
	}
	if plan.Operations[1].Type != PeerAdd {
		t.Errorf("second op type = %s, want add", plan.Operations[1].Type)
	}
	if plan.Operations[2].Type != PeerUpdate {
		t.Errorf("third op type = %s, want update", plan.Operations[2].Type)
	}
}

func TestReconcilePlan_OperationCounts(t *testing.T) {
	plan := ReconcilePlan{
		Operations: []PeerOperation{
			{Type: PeerRemove},
			{Type: PeerRemove},
			{Type: PeerAdd},
			{Type: PeerUpdate},
			{Type: PeerAdd},
		},
	}
	adds, updates, removes := plan.OperationCounts()
	if adds != 2 {
		t.Errorf("adds = %d, want 2", adds)
	}
	if updates != 1 {
		t.Errorf("updates = %d, want 1", updates)
	}
	if removes != 2 {
		t.Errorf("removes = %d, want 2", removes)
	}
}

func TestPeerOperation_Fields(t *testing.T) {
	key := mustGenerateKey(t)

	addOp := PeerOperation{
		Type: PeerAdd,
		Peer: desiredPeer{EndpointPolicy: EndpointDynamic},
	}
	s := addOp.Fields()
	if !contains(s, "allowed-ips") || !contains(s, "keepalive") {
		t.Errorf("add fields = %q, want allowed-ips,keepalive", s)
	}

	addWithEP := PeerOperation{
		Type: PeerAdd,
		Peer: desiredPeer{
			EndpointPolicy: EndpointFixed,
			Endpoint:       mustResolveUDP(t, "1.2.3.4:51820"),
		},
	}
	s = addWithEP.Fields()
	if !contains(s, "endpoint") {
		t.Errorf("add+ep fields = %q, want endpoint included", s)
	}

	removeOp := PeerOperation{Type: PeerRemove}
	if removeOp.Fields() != "peer" {
		t.Errorf("remove fields = %q, want peer", removeOp.Fields())
	}

	updateOp := PeerOperation{
		Type:             PeerUpdate,
		UpdateAllowedIPs: true,
		UpdateEndpoint:   true,
	}
	s = updateOp.Fields()
	if !contains(s, "allowed-ips") || !contains(s, "endpoint") {
		t.Errorf("update fields = %q, want allowed-ips,endpoint", s)
	}

	_ = key
}

func TestReconcileStatus_Degraded(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		lastAttempt time.Time
		lastSuccess time.Time
		want        bool
	}{
		{"never attempted", time.Time{}, time.Time{}, true},
		{"succeeded after attempt", now.Add(-time.Minute), now.Add(-time.Minute), false},
		{"failed (attempt after success)", now, now.Add(-time.Minute), true},
		{"first attempt failed", now, time.Time{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := ReconcileStatus{
				LastAttempt: tt.lastAttempt,
				LastSuccess: tt.lastSuccess,
			}
			if s.Degraded() != tt.want {
				t.Errorf("Degraded() = %v, want %v", s.Degraded(), tt.want)
			}
		})
	}
}

func TestWgPeerConfig_Add(t *testing.T) {
	k := mustGenerateKey(t)
	keepalive := 30 * time.Second

	peer := desiredPeer{
		PublicKey:           k,
		AllowedIPs:          []net.IPNet{mustParseCIDR(t, "10.0.0.1/32")},
		EndpointPolicy:      EndpointFixed,
		Endpoint:            mustResolveUDP(t, "1.2.3.4:51820"),
		PersistentKeepalive: keepalive,
	}
	op := PeerOperation{Type: PeerAdd, Peer: peer}
	cfg := wgPeerConfig(op)

	if cfg.Remove {
		t.Error("add: Remove should be false")
	}
	if !cfg.ReplaceAllowedIPs {
		t.Error("add: ReplaceAllowedIPs should be true")
	}
	if cfg.Endpoint == nil || cfg.Endpoint.String() != "1.2.3.4:51820" {
		t.Errorf("add: endpoint = %v, want 1.2.3.4:51820", cfg.Endpoint)
	}
	if cfg.PersistentKeepaliveInterval == nil || *cfg.PersistentKeepaliveInterval != keepalive {
		t.Errorf("add: keepalive = %v, want %v", cfg.PersistentKeepaliveInterval, keepalive)
	}
}

func TestWgPeerConfig_AddDynamicEndpoint(t *testing.T) {
	k := mustGenerateKey(t)
	peer := desiredPeer{
		PublicKey:           k,
		AllowedIPs:          []net.IPNet{mustParseCIDR(t, "10.0.0.1/32")},
		EndpointPolicy:      EndpointDynamic,
		Endpoint:            mustResolveUDP(t, "1.2.3.4:51820"),
		PersistentKeepalive: 0,
	}
	op := PeerOperation{Type: PeerAdd, Peer: peer}
	cfg := wgPeerConfig(op)

	if cfg.Endpoint != nil {
		t.Error("add+dynamic: Endpoint should be nil")
	}
}

func TestWgPeerConfig_Remove(t *testing.T) {
	k := mustGenerateKey(t)
	op := PeerOperation{Type: PeerRemove, Peer: desiredPeer{PublicKey: k}}
	cfg := wgPeerConfig(op)

	if !cfg.Remove {
		t.Error("remove: Remove should be true")
	}
}

func TestWgPeerConfig_Update(t *testing.T) {
	k := mustGenerateKey(t)
	keepalive := 25 * time.Second

	peer := desiredPeer{
		PublicKey:           k,
		AllowedIPs:          []net.IPNet{mustParseCIDR(t, "10.0.0.0/24")},
		EndpointPolicy:      EndpointFixed,
		Endpoint:            mustResolveUDP(t, "1.2.3.4:51820"),
		PersistentKeepalive: keepalive,
	}
	op := PeerOperation{
		Type:             PeerUpdate,
		Peer:             peer,
		UpdateAllowedIPs: true,
		UpdateEndpoint:   true,
		UpdateKeepalive:  true,
	}
	cfg := wgPeerConfig(op)

	if !cfg.UpdateOnly {
		t.Error("update: UpdateOnly should be true")
	}
	if !cfg.ReplaceAllowedIPs {
		t.Error("update: ReplaceAllowedIPs should be true")
	}
	if cfg.Endpoint == nil || cfg.Endpoint.String() != "1.2.3.4:51820" {
		t.Errorf("update: endpoint = %v, want 1.2.3.4:51820", cfg.Endpoint)
	}
	if cfg.PersistentKeepaliveInterval == nil || *cfg.PersistentKeepaliveInterval != keepalive {
		t.Errorf("update: keepalive = %v, want %v", cfg.PersistentKeepaliveInterval, keepalive)
	}
}

func TestNormalizeAllowedIPs(t *testing.T) {
	ips := []net.IPNet{
		mustParseCIDR(t, "10.0.2.0/24"),
		mustParseCIDR(t, "10.0.1.0/24"),
		mustParseCIDR(t, "10.0.3.0/24"),
	}
	normalized := normalizeAllowedIPs(ips)
	if len(normalized) != 3 {
		t.Fatalf("length = %d, want 3", len(normalized))
	}
	if normalized[0].String() != "10.0.1.0/24" {
		t.Errorf("first = %s, want 10.0.1.0/24", normalized[0].String())
	}
	if normalized[1].String() != "10.0.2.0/24" {
		t.Errorf("second = %s, want 10.0.2.0/24", normalized[1].String())
	}
}

func TestNormalizeAllowedIPs_SortsInPlaceAndReturnsNewSlice(t *testing.T) {
	original := []net.IPNet{
		mustParseCIDR(t, "10.0.2.0/24"),
		mustParseCIDR(t, "10.0.1.0/24"),
	}
	normalized := normalizeAllowedIPs(original)
	if normalized[0].String() != "10.0.1.0/24" {
		t.Error("sort failed")
	}
	// original should be unchanged (normalize makes a copy)
	if original[0].String() != "10.0.2.0/24" {
		t.Error("original slice mutated")
	}
}

func TestAllowedIPsEqual(t *testing.T) {
	a := []net.IPNet{mustParseCIDR(t, "10.0.0.0/24")}
	b := []net.IPNet{mustParseCIDR(t, "10.0.0.0/24")}
	c := []net.IPNet{mustParseCIDR(t, "10.0.1.0/24")}
	d := []net.IPNet{mustParseCIDR(t, "10.0.0.0/24"), mustParseCIDR(t, "10.0.1.0/24")}

	if !allowedIPsEqual(a, b) {
		t.Error("same CIDRs should be equal")
	}
	if allowedIPsEqual(a, c) {
		t.Error("different CIDRs should not be equal")
	}
	if allowedIPsEqual(a, d) {
		t.Error("different lengths should not be equal")
	}
}

func TestEndpointsEqual(t *testing.T) {
	ep1 := mustResolveUDP(t, "1.2.3.4:51820")
	ep2 := mustResolveUDP(t, "5.6.7.8:51821")
	ep1Copy := mustResolveUDP(t, "1.2.3.4:51820")

	if !endpointsEqual(ep1, ep1Copy) {
		t.Error("same endpoint should be equal")
	}
	if endpointsEqual(ep1, ep2) {
		t.Error("different endpoints should not be equal")
	}
	if endpointsEqual(nil, nil) != true {
		t.Error("both nil should be equal")
	}
	if endpointsEqual(ep1, nil) {
		t.Error("one nil should not be equal")
	}
	if endpointsEqual(nil, ep1) {
		t.Error("one nil should not be equal")
	}
}

func TestShortKey(t *testing.T) {
	k := mustGenerateKey(t)
	s := k.String()

	short := shortKey(k)
	if len(short) != 11 {
		t.Errorf("short key length = %d, want 11 (8 + 3 dots)", len(short))
	}
	if short != s[:8]+"..." {
		t.Errorf("short key = %q, want %q", short, s[:8]+"...")
	}

	// short key is already defined on the static wgtypes.Key zero value
	var zero wgtypes.Key
	sz := shortKey(zero)
	expected := "AAAAAAAA"
	if sz != expected+"..." {
		t.Errorf("short zero key = %q, want %q", sz, expected+"...")
	}
}

func TestEndpointString(t *testing.T) {
	if endpointString(nil) != "" {
		t.Error("nil endpoint should return empty string")
	}
	ep := mustResolveUDP(t, "1.2.3.4:51820")
	if endpointString(ep) != "1.2.3.4:51820" {
		t.Errorf("endpoint string = %q, want 1.2.3.4:51820", endpointString(ep))
	}
}

func TestPeerOperationType_String(t *testing.T) {
	tests := []struct {
		op   PeerOperationType
		want string
	}{
		{PeerAdd, "add"},
		{PeerUpdate, "update"},
		{PeerRemove, "remove"},
		{PeerOperationType(99), "unknown"},
	}
	for _, tt := range tests {
		if tt.op.String() != tt.want {
			t.Errorf("%d.String() = %q, want %q", tt.op, tt.op.String(), tt.want)
		}
	}
}

func TestSortSliceOnKeys(t *testing.T) {
	k1, k2 := mustGenerateKey(t), mustGenerateKey(t)
	keys := []wgtypes.Key{k2, k1}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].String() < keys[j].String()
	})
	if keys[0] != k1 && keys[1] != k2 {
		// the exact ordering depends on key values, but they must be sorted
		if keys[0].String() > keys[1].String() {
			t.Error("keys not sorted")
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
