package testutil

import (
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func SetupDB(t *testing.T) *database.DB {
	t.Helper()

	opts := database.Options{
		Path: ":memory:",
	}
	db, err := database.Open(opts)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	return db
}

func SeedPeerDB(
	t *testing.T,
	db *database.DB,
	network string,
	name string,
	cidr string,
	publicKey string,
	admin bool,
	enabled bool,
	confirmed bool,
) {
	t.Helper()

	if err := db.InsertCidr(network, &service.Cidr{
		Name: name,
		Cidr: cidr,
	}); err != nil {
		t.Fatalf("seed peer cidr %s: %v", name, err)
	}

	if err := db.InsertPeer(network, &service.Peer{
		Name:      name,
		CidrName:  name,
		PublicKey: publicKey,
		Admin:     admin,
		Enabled:   enabled,
		Confirmed: confirmed,
	}); err != nil {
		t.Fatalf("seed peer %s: %v", name, err)
	}
}
