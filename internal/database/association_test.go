package database_test

import "testing"

// TestAssociationCreateValid tests creating a valid association
func TestAssociationCreateValid(t *testing.T) {
	store := setupTestDB(t)

	// create root CIDR first
	err := createRootCidr(t, store, TestCidrRoot)
	expectNoError(t, err, "creating root CIDR")

	// create test CIDRs as subnets
	err = createCidrs(t, store, []TestCidrDesc{TestCidr1, TestCidr2})
	expectNoError(t, err, "creating test CIDRs")

	// create association between the CIDRs
	err = createAssociation(t, store, TestCidr1.Name, TestCidr2.Name)
	expectNoError(t, err, "creating valid association")
	assertAssociationExists(t, store, TestCidr1.Name, TestCidr2.Name)
}

// TestAssociationCreateNonExistentCidr tests creating association with non-existent CIDR
func TestAssociationCreateNonExistentCidr(t *testing.T) {
	store := setupTestDB(t)

	// create root CIDR first
	err := createRootCidr(t, store, TestCidrRoot)
	expectNoError(t, err, "creating root CIDR")

	// create only one test CIDR
	err = createCidr(t, store, TestCidr1)
	expectNoError(t, err, "creating test CIDR")

	// attempt to create association with non-existent CIDR
	err = createAssociation(t, store, TestCidr1.Name, "non-existent-cidr")
	expectError(t, err, "creating association with non-existent CIDR")
}

// TestAssociationCreateDuplicate tests creating duplicate associations
func TestAssociationCreateDuplicate(t *testing.T) {
	store := setupTestDB(t)

	// create root CIDR first
	err := createRootCidr(t, store, TestCidrRoot)
	expectNoError(t, err, "creating root CIDR")

	// create test CIDRs first
	err = createCidrs(t, store, []TestCidrDesc{TestCidr1, TestCidr2})
	expectNoError(t, err, "creating test CIDRs")

	// create association first time
	err = createAssociation(t, store, TestCidr1.Name, TestCidr2.Name)
	expectNoError(t, err, "creating initial association")

	// attempt to create duplicate association
	err = createAssociation(t, store, TestCidr1.Name, TestCidr2.Name)
	expectError(t, err, "creating duplicate association")

	// verify only one association exists
	assertAssociationCount(t, store, 1)
}

// TestAssociationCreateSymmetric tests that associations are symmetric
func TestAssociationCreateSymmetric(t *testing.T) {
	store := setupTestDB(t)

	// create root CIDR first
	err := createRootCidr(t, store, TestCidrRoot)
	expectNoError(t, err, "creating root CIDR")

	// create test CIDRs first
	err = createCidrs(t, store, []TestCidrDesc{TestCidr1, TestCidr2})
	expectNoError(t, err, "creating test CIDRs")

	// create association in one direction
	err = createAssociation(t, store, TestCidr1.Name, TestCidr2.Name)
	expectNoError(t, err, "creating association")

	// attempt to create reverse association should fail (already exists)
	err = createAssociation(t, store, TestCidr2.Name, TestCidr1.Name)
	expectError(t, err, "creating reverse association")

	// verify only one association exists
	assertAssociationCount(t, store, 1)
}

// TestAssociationListEmpty tests listing when no associations exist
func TestAssociationListEmpty(t *testing.T) {
	store := setupTestDB(t)

	// list associations from empty database
	result, err := store.AssociationList()
	expectNoError(t, err, "listing empty associations")

	// verify no associations returned
	if len(result) != 0 {
		t.Errorf("empty db returned %d associations, want 0", len(result))
	}
	assertAssociationCount(t, store, 0)
}

// TestAssociationListMultiple tests listing multiple associations
func TestAssociationListMultiple(t *testing.T) {
	store := setupTestDB(t)

	// create root CIDR first
	err := createRootCidr(t, store, TestCidrRoot)
	expectNoError(t, err, "creating root CIDR")

	// create test CIDRs first
	cidrs := []TestCidrDesc{TestCidr1, TestCidr2, TestCidrSmall}
	err = createCidrs(t, store, cidrs)
	expectNoError(t, err, "creating test CIDRs")

	// create multiple associations
	associations := [][2]string{
		{TestCidr1.Name, TestCidr2.Name},
		{TestCidr1.Name, TestCidrSmall.Name},
	}
	err = createAssociations(t, store, associations)
	expectNoError(t, err, "creating multiple associations")

	// list all associations
	result, err := store.AssociationList()
	expectNoError(t, err, "listing associations")

	// verify correct number of associations returned
	if len(result) != len(associations) {
		t.Errorf("returned %d associations, want %d", len(result), len(associations))
	}

	// verify results are ordered by CIDR names (as specified in SQL)
	for i := 1; i < len(result); i++ {
		if result[i-1].Cidr1 > result[i].Cidr1 ||
			(result[i-1].Cidr1 == result[i].Cidr1 && result[i-1].Cidr2 > result[i].Cidr2) {
			t.Errorf("results not ordered correctly: %s,%s should come before %s,%s",
				result[i].Cidr1, result[i].Cidr2, result[i-1].Cidr1, result[i-1].Cidr2)
		}
	}

	// verify each association has both CIDR names populated
	for _, assoc := range result {
		if assoc.Cidr1 == "" {
			t.Error("association has empty Cidr1")
		}
		if assoc.Cidr2 == "" {
			t.Error("association has empty Cidr2")
		}
	}
}

// TestAssociationDeleteValid tests deleting a valid association
func TestAssociationDeleteValid(t *testing.T) {
	store := setupTestDB(t)

	// create root CIDR first
	err := createRootCidr(t, store, TestCidrRoot)
	expectNoError(t, err, "creating root CIDR")

	// create test CIDRs first
	err = createCidrs(t, store, []TestCidrDesc{TestCidr1, TestCidr2})
	expectNoError(t, err, "creating test CIDRs")

	// create association
	err = createAssociation(t, store, TestCidr1.Name, TestCidr2.Name)
	expectNoError(t, err, "creating association")
	assertAssociationExists(t, store, TestCidr1.Name, TestCidr2.Name)

	// delete the association
	err = store.AssociationDelete(TestCidr1.Name, TestCidr2.Name)
	expectNoError(t, err, "deleting association")
	assertAssociationNotExists(t, store, TestCidr1.Name, TestCidr2.Name)
}

// TestAssociationDeleteSymmetric tests that deletion works in both directions
func TestAssociationDeleteSymmetric(t *testing.T) {
	store := setupTestDB(t)

	// create root CIDR first
	err := createRootCidr(t, store, TestCidrRoot)
	expectNoError(t, err, "creating root CIDR")

	// create test CIDRs first
	err = createCidrs(t, store, []TestCidrDesc{TestCidr1, TestCidr2})
	expectNoError(t, err, "creating test CIDRs")

	// create association
	err = createAssociation(t, store, TestCidr1.Name, TestCidr2.Name)
	expectNoError(t, err, "creating association")

	// delete association in reverse order
	err = store.AssociationDelete(TestCidr2.Name, TestCidr1.Name)
	expectNoError(t, err, "deleting association in reverse")
	assertAssociationNotExists(t, store, TestCidr1.Name, TestCidr2.Name)
}

// TestAssociationDeleteNonExistent tests deleting non-existent association
func TestAssociationDeleteNonExistent(t *testing.T) {
	store := setupTestDB(t)

	// create root CIDR first
	err := createRootCidr(t, store, TestCidrRoot)
	expectNoError(t, err, "creating root CIDR")

	// create test CIDRs first
	err = createCidrs(t, store, []TestCidrDesc{TestCidr1, TestCidr2})
	expectNoError(t, err, "creating test CIDRs")

	// attempt to delete non-existent association (should not error)
	err = store.AssociationDelete(TestCidr1.Name, TestCidr2.Name)
	expectNoError(t, err, "deleting non-existent association")
	assertAssociationCount(t, store, 0)
}

// TestAssociationDeleteWithNonExistentCidr tests deleting with non-existent CIDR names
func TestAssociationDeleteWithNonExistentCidr(t *testing.T) {
	store := setupTestDB(t)

	// create root CIDR first
	err := createRootCidr(t, store, TestCidrRoot)
	expectNoError(t, err, "creating root CIDR")

	// create one test CIDR
	err = createCidr(t, store, TestCidr1)
	expectNoError(t, err, "creating test CIDR")

	// attempt to delete association with non-existent CIDR (should not error)
	err = store.AssociationDelete(TestCidr1.Name, "non-existent-cidr")
	expectNoError(t, err, "deleting association with non-existent CIDR")
	assertAssociationCount(t, store, 0)
}
