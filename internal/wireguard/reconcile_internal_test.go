package wireguard

import (
	"net"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func TestPlanPeerReconciliation_EmptyBoth(t *testing.T) {
	ops := planPeerReconciliation(nil, nil)
	if len(ops) != 0 {
		t.Errorf("expected no operations, got %d", len(ops))
	}
}

func TestPlanPeerReconciliation_AllNew(t *testing.T) {
	k1, k2, k3 := mustGenKey(t), mustGenKey(t), mustGenKey(t)
	desired := []PeerConfig{
		pcfg(k1, []string{"10.0.0.1/32"}, "", EndpointDynamic, 0),
		pcfg(k2, []string{"10.0.0.2/32"}, "", EndpointDynamic, 0),
		pcfg(k3, []string{"10.0.0.3/32"}, "", EndpointDynamic, 0),
	}

	ops := planPeerReconciliation(desired, nil)
	if len(ops) != 3 {
		t.Errorf("expected 3 adds, got %d ops", len(ops))
	}
	for _, op := range ops {
		if op.Remove {
			t.Error("expected all non-remove ops, got remove")
		}
	}
}

func TestPlanPeerReconciliation_AllRemoved(t *testing.T) {
	k1, k2, k3 := mustGenKey(t), mustGenKey(t), mustGenKey(t)
	observed := []PeerStatus{
		pstat(k1, []string{"10.0.0.1/32"}, "", 0),
		pstat(k2, []string{"10.0.0.2/32"}, "", 0),
		pstat(k3, []string{"10.0.0.3/32"}, "", 0),
	}

	ops := planPeerReconciliation(nil, observed)
	if len(ops) != 3 {
		t.Errorf("expected 3 removes, got %d", len(ops))
	}
	for _, op := range ops {
		if !op.Remove {
			t.Error("expected all remove ops")
		}
	}
}

func TestPlanPeerReconciliation_ExactMatch(t *testing.T) {
	k1, k2 := mustGenKey(t), mustGenKey(t)
	desired := []PeerConfig{
		pcfg(k1, []string{"10.0.0.1/32"}, "", EndpointDynamic, 0),
		pcfg(k2, []string{"10.0.0.2/32"}, "", EndpointDynamic, 0),
	}
	observed := []PeerStatus{
		pstat(k1, []string{"10.0.0.1/32"}, "", 0),
		pstat(k2, []string{"10.0.0.2/32"}, "", 0),
	}

	ops := planPeerReconciliation(desired, observed)
	if len(ops) != 0 {
		t.Errorf("expected 0 operations, got %d", len(ops))
	}
}

func TestPlanPeerReconciliation_Mixed(t *testing.T) {
	kA, kB, kC, kD := mustGenKey(t), mustGenKey(t), mustGenKey(t), mustGenKey(t)
	desired := []PeerConfig{
		pcfg(kA, []string{"10.0.0.1/32"}, "", EndpointDynamic, 0),
		pcfg(kB, []string{"10.0.0.2/32"}, "", EndpointDynamic, 0),
		pcfg(kC, []string{"10.0.0.3/32"}, "", EndpointDynamic, 0),
	}
	observed := []PeerStatus{
		pstat(kB, []string{"10.0.0.2/32"}, "", 0),
		pstat(kD, []string{"10.0.0.4/32"}, "", 0),
	}

	ops := planPeerReconciliation(desired, observed)
	var adds, removes int
	for _, op := range ops {
		if op.Remove {
			removes++
		} else if op.Config.PublicKey == kA || op.Config.PublicKey == kC {
			adds++
		}
	}
	if adds != 2 {
		t.Errorf("adds = %d, want 2", adds)
	}
	if removes != 1 {
		t.Errorf("removes = %d, want 1", removes)
	}
}

func TestPlanPeerReconciliation_AllowedIPsChanged(t *testing.T) {
	k := mustGenKey(t)
	desired := []PeerConfig{
		pcfg(k, []string{"10.0.1.0/24"}, "", EndpointDynamic, 0),
	}
	observed := []PeerStatus{
		pstat(k, []string{"10.0.0.0/24"}, "", 0),
	}

	ops := planPeerReconciliation(desired, observed)
	if len(ops) != 1 {
		t.Fatalf("expected 1 update op, got %d", len(ops))
	}
	if ops[0].Remove {
		t.Error("expected non-remove op")
	}
}

func TestPlanPeerReconciliation_AllowedIPOrderIgnored(t *testing.T) {
	k := mustGenKey(t)
	desired := []PeerConfig{
		pcfg(k, []string{"10.0.1.0/24", "10.0.0.0/24"}, "", EndpointDynamic, 0),
	}
	observed := []PeerStatus{
		pstat(k, []string{"10.0.0.0/24", "10.0.1.0/24"}, "", 0),
	}

	ops := planPeerReconciliation(desired, observed)
	if len(ops) != 0 {
		t.Errorf("expected no operations for same CIDRs in different order, got %d", len(ops))
	}
}

func TestPlanPeerReconciliation_KeepaliveChanged(t *testing.T) {
	k := mustGenKey(t)
	desired := []PeerConfig{
		pcfg(k, []string{"10.0.0.1/32"}, "", EndpointDynamic, 25*time.Second),
	}
	observed := []PeerStatus{
		pstat(k, []string{"10.0.0.1/32"}, "", 0),
	}

	ops := planPeerReconciliation(desired, observed)
	if len(ops) != 1 {
		t.Errorf("expected 1 update op, got %d", len(ops))
	}
}

func TestPlanPeerReconciliation_EndpointFixedChanged(t *testing.T) {
	k := mustGenKey(t)
	desired := []PeerConfig{
		pcfg(k, []string{"10.0.0.1/32"}, "1.2.3.4:51820", EndpointFixed, 0),
	}
	observed := []PeerStatus{
		pstat(k, []string{"10.0.0.1/32"}, "5.6.7.8:51821", 0),
	}

	ops := planPeerReconciliation(desired, observed)
	if len(ops) != 1 {
		t.Fatalf("expected 1 update op, got %d", len(ops))
	}
	if ops[0].Config.Endpoint == nil {
		t.Error("expected endpoint to be set for Fixed peer with drifted endpoint")
	}
}

func TestPlanPeerReconciliation_EndpointDynamicIgnoresChange(t *testing.T) {
	k := mustGenKey(t)
	desired := []PeerConfig{
		pcfg(k, []string{"10.0.0.1/32"}, "1.2.3.4:51820", EndpointDynamic, 0),
	}
	observed := []PeerStatus{
		pstat(k, []string{"10.0.0.1/32"}, "5.6.7.8:51821", 0),
	}

	ops := planPeerReconciliation(desired, observed)
	if len(ops) != 0 {
		t.Errorf("expected no operations for EndpointDynamic, got %d", len(ops))
	}
}

func TestPlanPeerReconciliation_EndpointBootstrapIgnoresChange(t *testing.T) {
	k := mustGenKey(t)
	desired := []PeerConfig{
		pcfg(k, []string{"10.0.0.1/32"}, "1.2.3.4:51820", EndpointBootstrap, 0),
	}
	observed := []PeerStatus{
		pstat(k, []string{"10.0.0.1/32"}, "5.6.7.8:51821", 0),
	}

	ops := planPeerReconciliation(desired, observed)
	if len(ops) != 0 {
		t.Errorf("expected no operations for EndpointBootstrap, got %d", len(ops))
	}
}

func TestPlanPeerReconciliation_MultipleChanges(t *testing.T) {
	k := mustGenKey(t)
	desired := []PeerConfig{
		pcfg(k, []string{"10.0.1.0/24"}, "1.2.3.4:51820", EndpointFixed, 25*time.Second),
	}
	observed := []PeerStatus{
		pstat(k, []string{"10.0.0.0/24"}, "5.6.7.8:51821", 0),
	}

	ops := planPeerReconciliation(desired, observed)
	if len(ops) != 1 {
		t.Fatalf("expected 1 update op, got %d", len(ops))
	}
	op := ops[0]
	if op.Remove {
		t.Fatal("expected non-remove op")
	}
	if op.Config.Endpoint == nil {
		t.Error("expected endpoint to be set for Fixed peer with drifted endpoint")
	}
}

func TestPlanPeerReconciliation_OperationOrdering(t *testing.T) {
	kA, kB, kC := mustGenKey(t), mustGenKey(t), mustGenKey(t)
	desired := []PeerConfig{
		pcfg(kB, []string{"10.0.0.2/32"}, "", EndpointDynamic, 0),
		pcfg(kC, []string{"10.0.1.0/24"}, "", EndpointDynamic, 0),
	}
	observed := []PeerStatus{
		pstat(kA, []string{"10.0.0.1/32"}, "", 0),
		pstat(kC, []string{"10.0.0.0/24"}, "", 0),
	}

	ops := planPeerReconciliation(desired, observed)
	if len(ops) != 3 {
		t.Fatalf("expected 3 operations, got %d", len(ops))
	}
	if !ops[0].Remove {
		t.Errorf("first op type = %v, want remove", ops[0])
	}
	if ops[1].Remove {
		t.Errorf("second op type = %v, want add", ops[1])
	}
	if ops[2].Remove {
		t.Errorf("third op type = %v, want update", ops[2])
	}
}

func TestPlanPeerReconciliation_NoEndpointOnAddForDynamic(t *testing.T) {
	k := mustGenKey(t)
	desired := []PeerConfig{
		pcfg(k, []string{"10.0.0.1/32"}, "1.2.3.4:51820", EndpointDynamic, 0),
	}

	ops := planPeerReconciliation(desired, nil)
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	if ops[0].Config.Endpoint != nil {
		t.Error("expected nil endpoint for Dynamic peer on add")
	}
}

func TestPlanPeerReconciliation_EndpointOnAddForFixed(t *testing.T) {
	k := mustGenKey(t)
	desired := []PeerConfig{
		pcfg(k, []string{"10.0.0.1/32"}, "1.2.3.4:51820", EndpointFixed, 0),
	}

	ops := planPeerReconciliation(desired, nil)
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	if ops[0].Config.Endpoint == nil {
		t.Error("expected endpoint for Fixed peer on add")
	}
}

func TestPlanPeerReconciliation_EndpointOnAddForBootstrap(t *testing.T) {
	k := mustGenKey(t)
	desired := []PeerConfig{
		pcfg(k, []string{"10.0.0.1/32"}, "1.2.3.4:51820", EndpointBootstrap, 0),
	}

	ops := planPeerReconciliation(desired, nil)
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	if ops[0].Config.Endpoint == nil {
		t.Error("expected endpoint for Bootstrap peer on add")
	}
}

// --- helpers ---

func mustGenKey(t *testing.T) wgtypes.Key {
	t.Helper()
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

func pcfg(key wgtypes.Key, allowedIPs []string, endpoint string, policy EndpointPolicy, keepalive time.Duration) PeerConfig {
	var ep *net.UDPAddr
	if endpoint != "" {
		addr, err := net.ResolveUDPAddr("udp", endpoint)
		if err != nil {
			panic(err)
		}
		ep = addr
	}
	ips := make([]net.IPNet, 0, len(allowedIPs))
	for _, cidr := range allowedIPs {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(err)
		}
		ips = append(ips, *n)
	}
	return PeerConfig{
		PublicKey:           key,
		AllowedIPs:          ips,
		Endpoint:            ep,
		EndpointPolicy:      policy,
		PersistentKeepalive: keepalive,
	}
}

func pstat(key wgtypes.Key, allowedIPs []string, endpoint string, keepalive time.Duration) PeerStatus {
	cfg := pcfg(key, allowedIPs, endpoint, EndpointDynamic, keepalive)
	return PeerStatus{
		PublicKey:           cfg.PublicKey,
		AllowedIPs:          cfg.AllowedIPs,
		Endpoint:            cfg.Endpoint,
		PersistentKeepalive: cfg.PersistentKeepalive,
	}
}
