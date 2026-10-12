package database_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
	sqlite "modernc.org/sqlite"
)

func TestCheckSqliteErr_ClassifiesWrappedConstraints(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	seedDatabaseNetwork(t, db, "constraints")
	group, err := db.InsertGroup("constraints", "staff")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, query string
		args        []any
		conflict    bool
	}{
		{"primary key", `INSERT INTO "group" (id, network_name, name) VALUES (?1, ?2, ?3)`, []any{group.ID, "constraints", "new"}, true},
		{"unique", `INSERT INTO "group" (network_name, name) VALUES (?1, ?2)`, []any{"constraints", "staff"}, true},
		{"foreign key", `INSERT INTO "group" (network_name, name) VALUES (?1, ?2)`, []any{"missing", "staff"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Real SQL failures prove driver classification, including wrapped errors.
			_, original := db.Conn.Exec(tc.query, tc.args...)
			if original == nil {
				t.Fatal("expected constraint failure")
			}
			for _, input := range []error{original, fmt.Errorf("outer: %w", original)} {
				got := database.CheckSqliteErr("test constraint", input)
				if errors.Is(got, service.ErrConflict) != tc.conflict || errors.Is(got, service.ErrNotFound) {
					t.Fatalf("classification = %v, want conflict %v", got, tc.conflict)
				}
				var driverErr *sqlite.Error
				if !errors.As(got, &driverErr) {
					t.Fatalf("lost driver diagnostic: %v", got)
				}
			}
			groups, err := db.ListGroups("constraints")
			if err != nil || !reflect.DeepEqual(groups, []*service.Group{group}) {
				t.Fatalf("rejected insert changed groups: %+v, %v", groups, err)
			}
		})
	}
}

func TestOpen_ReopensDurableNetwork(
	t *testing.T,
) {
	root := "../../../.opencode-tmp"
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(root, "database-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	opts := database.Options{Path: filepath.Join(dir, "cord.db"), WAL: true}
	db, err := database.Open(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	seedDatabaseNetwork(t, db, "durable")
	if err := db.SetNetworkEnabled("durable", true); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := database.Open(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	got, err := reopened.GetNetwork("durable")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "durable" || !got.Enabled || got.PublicKey != "pub-durable" {
		t.Fatalf("reopened network = %+v", got)
	}
	if _, err := reopened.InsertGroup("durable", "ready"); err != nil {
		t.Fatalf("reopened adapter not ready: %v", err)
	}
}

func TestOpen_ClosesCleanly(
	t *testing.T,
) {
	opts := database.Options{
		Path: ":memory:",
	}
	db, err := database.Open(opts)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}
