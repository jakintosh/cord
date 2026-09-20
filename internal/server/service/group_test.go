package service_test

import (
	"errors"
	"reflect"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestCreateGroup_ListsSortedGroupsAndRejectsConflicts(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	for _, name := range []string{"operations", "engineering"} {
		group, err := env.Service.CreateGroup("testnet", name)
		if err != nil {
			t.Fatal(err)
		}
		if group.ID == 0 || group.Name != name {
			t.Fatalf("created group = %+v", group)
		}
	}
	before, err := env.Service.ListGroups("testnet")
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 2 || before[0].Name != "engineering" || before[1].Name != "operations" {
		t.Fatalf("groups = %+v", before)
	}
	if _, err := env.Service.CreateGroup("testnet", "engineering"); !errors.Is(err, service.ErrConflict) {
		t.Fatalf("duplicate: %v", err)
	}
	after, err := env.Service.ListGroups("testnet")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("duplicate changed groups: %+v", after)
	}
	if err := env.Service.DeleteGroup("testnet", "engineering"); err != nil {
		t.Fatal(err)
	}
	after, err = env.Service.ListGroups("testnet")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || *after[0] != *before[1] {
		t.Fatalf("groups after delete = %+v", after)
	}
}

func TestCreateGroup_RejectsInvalidInputWithoutCreatingGroups(
	t *testing.T,
) {
	for _, tc := range []struct {
		name, network, group string
		want                 error
	}{
		{"empty network", "", "staff", service.ErrInvalidInput},
		{"empty group", "testnet", "", service.ErrInvalidInput},
		{"missing network", "missing", "staff", service.ErrNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := testutil.SetupService(t)
			testutil.SeedNetwork(t, env.Service)
			if _, err := env.Service.CreateGroup(tc.network, tc.group); !errors.Is(err, tc.want) {
				t.Fatalf("create: %v, want %v", err, tc.want)
			}
			groups, err := env.Service.ListGroups("testnet")
			if err != nil || len(groups) != 0 {
				t.Fatalf("groups after rejection = %+v, %v", groups, err)
			}
		})
	}
}

func TestDeleteGroup_RemovesAssignmentsAndAssociations(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	seedTwoGroups(t, env.Service)
	if err := env.Service.AssignCidrGroup("testnet", "testnet", "group-a"); err != nil {
		t.Fatal(err)
	}
	if err := env.Service.CreateAssociation("testnet", "group-a", "group-b"); err != nil {
		t.Fatal(err)
	}
	if err := env.Service.DeleteGroup("testnet", "group-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Service.CreateGroup("testnet", "group-a"); err != nil {
		t.Fatal(err)
	}
	groups, err := env.Service.ListCidrGroups("testnet", "testnet")
	if err != nil || len(groups) != 0 {
		t.Fatalf("assignments after recreation = %+v, %v", groups, err)
	}
	associations, err := env.Service.ListAssociations("testnet")
	if err != nil || len(associations) != 0 {
		t.Fatalf("associations after recreation = %+v, %v", associations, err)
	}
	if err := env.Service.DeleteGroup("testnet", "missing"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("delete missing group: %v", err)
	}
}

func TestDeleteGroup_InvalidatesCompiledModel(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	if err := env.Service.CreateCidr("testnet", "east", "10.0.1.0/24"); err != nil {
		t.Fatalf("create CIDR: %v", err)
	}
	if _, err := env.Service.CreateGroup("testnet", "riders"); err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := env.Service.AssignCidrGroup("testnet", "east", "riders"); err != nil {
		t.Fatalf("assign group: %v", err)
	}

	if _, err := env.Service.GetNetworkTopology("testnet"); err != nil {
		t.Fatalf("get topology: %v", err)
	}
	if err := env.Service.DeleteGroup("testnet", "riders"); err != nil {
		t.Fatalf("delete group: %v", err)
	}

	view, err := env.Service.GetNetworkTopology("testnet")
	if err != nil {
		t.Fatalf("get topology after delete: %v", err)
	}
	for _, node := range view.Nodes {
		if node.Cidr.Name != "east" {
			continue
		}
		for _, group := range node.Groups {
			if group == "riders" {
				t.Fatal("deleted group still present in compiled topology")
			}
		}
		return
	}
	t.Fatal("east CIDR missing after deleting its group")
}
