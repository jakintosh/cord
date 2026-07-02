package wireguard_test

import (
	"strings"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

func TestPlanPeerReconciliation_EmptyBoth(t *testing.T) {
	plan := wireguard.PlanPeerReconciliation(nil, nil)
	if len(plan.Operations) != 0 {
		t.Errorf("expected no operations, got %d", len(plan.Operations))
	}
}

func TestPlanPeerReconciliation_AllNew(t *testing.T) {
	k1, k2, k3 := mustGenerateKey(t), mustGenerateKey(t), mustGenerateKey(t)
	desired := []wireguard.Peer{
		peer(k1, []string{"10.0.0.1/32"}, "", wireguard.EndpointDynamic, 0),
		peer(k2, []string{"10.0.0.2/32"}, "", wireguard.EndpointDynamic, 0),
		peer(k3, []string{"10.0.0.3/32"}, "", wireguard.EndpointDynamic, 0),
	}

	plan := wireguard.PlanPeerReconciliation(desired, nil)
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
	observed := []wireguard.Peer{
		peer(k1, []string{"10.0.0.1/32"}, "", wireguard.EndpointDynamic, 0),
		peer(k2, []string{"10.0.0.2/32"}, "", wireguard.EndpointDynamic, 0),
		peer(k3, []string{"10.0.0.3/32"}, "", wireguard.EndpointDynamic, 0),
	}

	plan := wireguard.PlanPeerReconciliation(nil, observed)
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
	desired := []wireguard.Peer{
		peer(k1, []string{"10.0.0.1/32"}, "", wireguard.EndpointDynamic, 0),
		peer(k2, []string{"10.0.0.2/32"}, "", wireguard.EndpointDynamic, 0),
	}
	observed := []wireguard.Peer{
		peer(k1, []string{"10.0.0.1/32"}, "", wireguard.EndpointDynamic, 0),
		peer(k2, []string{"10.0.0.2/32"}, "", wireguard.EndpointDynamic, 0),
	}

	plan := wireguard.PlanPeerReconciliation(desired, observed)
	if len(plan.Operations) != 0 {
		t.Errorf("expected 0 operations, got %d", len(plan.Operations))
	}
}

func TestPlanPeerReconciliation_Mixed(t *testing.T) {
	kA, kB, kC, kD := mustGenerateKey(t), mustGenerateKey(t), mustGenerateKey(t), mustGenerateKey(t)
	desired := []wireguard.Peer{
		peer(kA, []string{"10.0.0.1/32"}, "", wireguard.EndpointDynamic, 0),
		peer(kB, []string{"10.0.0.2/32"}, "", wireguard.EndpointDynamic, 0),
		peer(kC, []string{"10.0.0.3/32"}, "", wireguard.EndpointDynamic, 0),
	}
	observed := []wireguard.Peer{
		peer(kB, []string{"10.0.0.2/32"}, "", wireguard.EndpointDynamic, 0),
		peer(kD, []string{"10.0.0.4/32"}, "", wireguard.EndpointDynamic, 0),
	}

	plan := wireguard.PlanPeerReconciliation(desired, observed)
	adds, updates, removes := plan.OperationCounts()
	if adds != 2 {
		t.Errorf("adds = %d, want 2", adds)
	}
	if removes != 1 {
		t.Errorf("removes = %d, want 1", removes)
	}
	if updates != 0 {
		t.Errorf("updates = %d, want 0", updates)
	}
}

func TestPlanPeerReconciliation_AllowedIPsChanged(t *testing.T) {
	k := mustGenerateKey(t)
	desired := []wireguard.Peer{
		peer(k, []string{"10.0.1.0/24"}, "", wireguard.EndpointDynamic, 0),
	}
	observed := []wireguard.Peer{
		peer(k, []string{"10.0.0.0/24"}, "", wireguard.EndpointDynamic, 0),
	}

	plan := wireguard.PlanPeerReconciliation(desired, observed)
	adds, updates, _ := plan.OperationCounts()
	if updates != 1 || adds != 0 {
		t.Errorf("expected 1 update, got add=%d update=%d", adds, updates)
	}
	if !plan.Operations[0].UpdateAllowedIPs {
		t.Error("expected UpdateAllowedIPs=true")
	}
}

func TestPlanPeerReconciliation_AllowedIPOrderIgnored(t *testing.T) {
	k := mustGenerateKey(t)
	desired := []wireguard.Peer{
		peer(k, []string{"10.0.1.0/24", "10.0.0.0/24"}, "", wireguard.EndpointDynamic, 0),
	}
	observed := []wireguard.Peer{
		peer(k, []string{"10.0.0.0/24", "10.0.1.0/24"}, "", wireguard.EndpointDynamic, 0),
	}

	plan := wireguard.PlanPeerReconciliation(desired, observed)
	if len(plan.Operations) != 0 {
		t.Errorf("expected no operations for same CIDRs in different order, got %d", len(plan.Operations))
	}
}

func TestPlanPeerReconciliation_KeepaliveChanged(t *testing.T) {
	k := mustGenerateKey(t)
	desired := []wireguard.Peer{
		peer(k, []string{"10.0.0.1/32"}, "", wireguard.EndpointDynamic, 25*time.Second),
	}
	observed := []wireguard.Peer{
		peer(k, []string{"10.0.0.1/32"}, "", wireguard.EndpointDynamic, 0),
	}

	plan := wireguard.PlanPeerReconciliation(desired, observed)
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
	desired := []wireguard.Peer{
		peer(k, []string{"10.0.0.1/32"}, "1.2.3.4:51820", wireguard.EndpointFixed, 0),
	}
	observed := []wireguard.Peer{
		peer(k, []string{"10.0.0.1/32"}, "5.6.7.8:51821", wireguard.EndpointDynamic, 0),
	}

	plan := wireguard.PlanPeerReconciliation(desired, observed)
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
	desired := []wireguard.Peer{
		peer(k, []string{"10.0.0.1/32"}, "1.2.3.4:51820", wireguard.EndpointDynamic, 0),
	}
	observed := []wireguard.Peer{
		peer(k, []string{"10.0.0.1/32"}, "5.6.7.8:51821", wireguard.EndpointDynamic, 0),
	}

	plan := wireguard.PlanPeerReconciliation(desired, observed)
	if len(plan.Operations) != 0 {
		t.Errorf("expected no operations for EndpointDynamic, got %d", len(plan.Operations))
	}
}

func TestPlanPeerReconciliation_EndpointBootstrapIgnoresChange(t *testing.T) {
	k := mustGenerateKey(t)
	desired := []wireguard.Peer{
		peer(k, []string{"10.0.0.1/32"}, "1.2.3.4:51820", wireguard.EndpointBootstrap, 0),
	}
	observed := []wireguard.Peer{
		peer(k, []string{"10.0.0.1/32"}, "5.6.7.8:51821", wireguard.EndpointDynamic, 0),
	}

	plan := wireguard.PlanPeerReconciliation(desired, observed)
	if len(plan.Operations) != 0 {
		t.Errorf("expected no operations for EndpointBootstrap, got %d", len(plan.Operations))
	}
}

func TestPlanPeerReconciliation_MultipleChanges(t *testing.T) {
	k := mustGenerateKey(t)
	desired := []wireguard.Peer{
		peer(k, []string{"10.0.1.0/24"}, "1.2.3.4:51820", wireguard.EndpointFixed, 25*time.Second),
	}
	observed := []wireguard.Peer{
		peer(k, []string{"10.0.0.0/24"}, "5.6.7.8:51821", wireguard.EndpointDynamic, 0),
	}

	plan := wireguard.PlanPeerReconciliation(desired, observed)
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
	desired := []wireguard.Peer{
		peer(kB, []string{"10.0.0.2/32"}, "", wireguard.EndpointDynamic, 0),
		peer(kC, []string{"10.0.1.0/24"}, "", wireguard.EndpointDynamic, 0),
	}
	observed := []wireguard.Peer{
		peer(kA, []string{"10.0.0.1/32"}, "", wireguard.EndpointDynamic, 0),
		peer(kC, []string{"10.0.0.0/24"}, "", wireguard.EndpointDynamic, 0),
	}

	plan := wireguard.PlanPeerReconciliation(desired, observed)
	if len(plan.Operations) != 3 {
		t.Fatalf("expected 3 operations, got %d", len(plan.Operations))
	}
	if plan.Operations[0].Type != wireguard.PeerRemove {
		t.Errorf("first op type = %s, want remove", plan.Operations[0].Type)
	}
	if plan.Operations[1].Type != wireguard.PeerAdd {
		t.Errorf("second op type = %s, want add", plan.Operations[1].Type)
	}
	if plan.Operations[2].Type != wireguard.PeerUpdate {
		t.Errorf("third op type = %s, want update", plan.Operations[2].Type)
	}
}

func TestReconcilePlan_OperationCounts(t *testing.T) {
	plan := wireguard.ReconcilePlan{
		Operations: []wireguard.PeerOperation{
			{Type: wireguard.PeerRemove},
			{Type: wireguard.PeerRemove},
			{Type: wireguard.PeerAdd},
			{Type: wireguard.PeerUpdate},
			{Type: wireguard.PeerAdd},
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
	addOp := wireguard.PeerOperation{
		Type: wireguard.PeerAdd,
		Peer: wireguard.Peer{EndpointPolicy: wireguard.EndpointDynamic},
	}
	s := addOp.Fields()
	if !strings.Contains(s, "allowed-ips") || !strings.Contains(s, "keepalive") {
		t.Errorf("add fields = %q, want allowed-ips,keepalive", s)
	}

	addWithEndpoint := wireguard.PeerOperation{
		Type: wireguard.PeerAdd,
		Peer: wireguard.Peer{
			EndpointPolicy: wireguard.EndpointFixed,
			Endpoint:       mustResolveUDP(t, "1.2.3.4:51820"),
		},
	}
	s = addWithEndpoint.Fields()
	if !strings.Contains(s, "endpoint") {
		t.Errorf("add+endpoint fields = %q, want endpoint included", s)
	}

	removeOp := wireguard.PeerOperation{Type: wireguard.PeerRemove}
	if removeOp.Fields() != "peer" {
		t.Errorf("remove fields = %q, want peer", removeOp.Fields())
	}

	updateOp := wireguard.PeerOperation{
		Type:             wireguard.PeerUpdate,
		UpdateAllowedIPs: true,
		UpdateEndpoint:   true,
	}
	s = updateOp.Fields()
	if !strings.Contains(s, "allowed-ips") || !strings.Contains(s, "endpoint") {
		t.Errorf("update fields = %q, want allowed-ips,endpoint", s)
	}
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
		{"failed", now, now.Add(-time.Minute), true},
		{"first attempt failed", now, time.Time{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := wireguard.ReconcileStatus{
				LastAttempt: tt.lastAttempt,
				LastSuccess: tt.lastSuccess,
			}
			if s.Degraded() != tt.want {
				t.Errorf("Degraded() = %v, want %v", s.Degraded(), tt.want)
			}
		})
	}
}

func TestPeerOperationType_String(t *testing.T) {
	tests := []struct {
		op   wireguard.PeerOperationType
		want string
	}{
		{wireguard.PeerAdd, "add"},
		{wireguard.PeerUpdate, "update"},
		{wireguard.PeerRemove, "remove"},
		{wireguard.PeerOperationType(99), "unknown"},
	}
	for _, tt := range tests {
		if tt.op.String() != tt.want {
			t.Errorf("%d.String() = %q, want %q", tt.op, tt.op.String(), tt.want)
		}
	}
}
