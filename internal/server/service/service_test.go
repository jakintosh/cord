package service_test

import (
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func lastTempKey(
	t *testing.T,
	svc *service.Service,
	network string,
) string {
	t.Helper()
	regs, err := svc.ListRegistrations(network)
	if err != nil {
		t.Fatalf("list registrations: %v", err)
	}
	if len(regs) == 0 {
		t.Fatal("no registrations found")
	}
	return regs[len(regs)-1].InvitePublicKey
}
