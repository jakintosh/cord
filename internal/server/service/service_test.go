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
	invites, err := svc.ListInvites(network)
	if err != nil {
		t.Fatalf("list invites: %v", err)
	}
	if len(invites) == 0 {
		t.Fatal("no invites found")
	}
	return invites[len(invites)-1].InvitePubKey
}
