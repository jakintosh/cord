package database_test

import (
	"errors"
	"reflect"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestListGroups_SortedAndNetworkScoped(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	seedDatabaseNetwork(t, db, "groups")
	seedDatabaseNetwork(t, db, "other")
	var want []*service.Group
	for _, name := range []string{"zebra", "alpha"} {
		group, err := db.InsertGroup("groups", name)
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, group)
	}
	want[0], want[1] = want[1], want[0]
	if _, err := db.InsertGroup("other", "alpha"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.InsertGroup("groups", "alpha"); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("duplicate = %v, want ErrConflict", err)
	}
	if err := db.DeleteGroup("groups", "missing"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("delete missing = %v, want ErrNotFound", err)
	}
	got, err := db.ListGroups("groups")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("groups = %+v, want %+v", got, want)
	}
}

func TestInsertGroup_MissingNetworkLeavesNoGroup(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	if _, err := db.InsertGroup("missing", "staff"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("insert = %v, want ErrNotFound", err)
	}
	groups, err := db.ListGroups("missing")
	if err != nil || len(groups) != 0 {
		t.Fatalf("groups after rejection = %+v, %v", groups, err)
	}
	seedDatabaseNetwork(t, db, "missing")
	if _, err := db.InsertGroup("missing", "staff"); err != nil {
		t.Fatalf("insert after creating parent: %v", err)
	}
}

func TestDeleteGroup_CascadesAssignmentsAndAssociations(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	seedDatabaseNetwork(t, db, "groups")
	removed, err := db.InsertGroup("groups", "removed")
	if err != nil {
		t.Fatal(err)
	}
	kept, err := db.InsertGroup("groups", "kept")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AssignCidrGroup("groups", "groups", "removed"); err != nil {
		t.Fatal(err)
	}
	if err := db.AssignCidrGroup("groups", "groups", "kept"); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertAssociation("groups", &service.Association{Group1: "removed", Group2: "kept"}); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteGroup("groups", "removed"); err != nil {
		t.Fatal(err)
	}
	got, err := db.ListCidrGroups("groups", "groups")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []*service.Group{kept}) {
		t.Fatalf("remaining assignments = %+v", got)
	}
	// Joined adapter reads hide orphan rows, so inspect the deleted foreign key directly.
	for _, query := range []string{
		`SELECT count(*) FROM cidr_assignment WHERE group_id = ?1`,
		`SELECT count(*) FROM association WHERE group1_id = ?1 OR group2_id = ?1`,
	} {
		var count int
		if err := db.Conn.QueryRow(query, removed.ID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("orphan rows = %d for %s", count, query)
		}
	}
}

func TestDeleteGroup_RemovesOnlyRelatedAssociations(
	t *testing.T,
) {
	db := testutil.SetupDB(t)
	seedNetworkForAssoc(t, db)

	if err := db.InsertAssociation("assocnet", &service.Association{
		Group1: "group-a",
		Group2: "group-b",
	}); err != nil {
		t.Fatalf("insert association: %v", err)
	}
	if err := db.InsertAssociation("assocnet", &service.Association{
		Group1: "group-b",
		Group2: "group-c",
	}); err != nil {
		t.Fatalf("insert unrelated association: %v", err)
	}

	if err := db.DeleteGroup("assocnet", "group-a"); err != nil {
		t.Fatalf("delete group: %v", err)
	}

	assocs, err := db.ListAssociations("assocnet")
	if err != nil {
		t.Fatalf("list after cascade: %v", err)
	}
	if len(assocs) != 1 || assocs[0].Group1 != "group-b" || assocs[0].Group2 != "group-c" {
		t.Fatalf("associations after cascade = %+v, want group-b/group-c", assocs)
	}
}
