package database_test

import (
	"testing"
)

// TestCidrCreateRoot tests creating a root CIDR
func TestCidrCreateRoot(t *testing.T) {
	store := setupTestDB(t)

	// create root CIDR
	err := createRootCidr(t, store, TestCidrRoot)
	expectNoError(t, err, "creating root CIDR")
	assertCidrExists(t, store, TestCidrRoot)
}

// TestCidrCreateValidSubnet tests creating a valid subnet within root
func TestCidrCreateValidSubnet(t *testing.T) {
	store := setupTestDB(t)

	// create root CIDR first
	err := createRootCidr(t, store, TestCidrRoot)
	expectNoError(t, err, "creating root CIDR")

	// create valid subnet
	err = createCidr(t, store, TestCidr1)
	expectNoError(t, err, "creating valid subnet")
	assertCidrExists(t, store, TestCidr1)
}

// TestCidrCreateInvalidSubnet tests creating a subnet outside root CIDR
func TestCidrCreateInvalidSubnet(t *testing.T) {
	store := setupTestDB(t)

	// create root CIDR first
	err := createRootCidr(t, store, TestCidrRoot)
	expectNoError(t, err, "creating root CIDR")

	// attempt to create subnet outside root CIDR range
	invalidCidr := TestCidrDesc{
		Name:   "invalid-subnet",
		Cidr:   "192.168.1.0/24",
		Length: 32,
		Prefix: 24,
	}
	err = createCidr(t, store, invalidCidr)
	expectError(t, err, "creating invalid subnet outside root CIDR")
}

// TestCidrCreateDuplicateName tests that duplicate names are rejected
func TestCidrCreateDuplicateName(t *testing.T) {
	store := setupTestDB(t)

	// create root CIDR first
	err := createRootCidr(t, store, TestCidrRoot)
	expectNoError(t, err, "creating root CIDR")

	// create valid CIDR
	err = createCidr(t, store, TestCidr1)
	expectNoError(t, err, "creating valid CIDR")

	// attempt to create CIDR with duplicate name but different IP range
	duplicateCidr := TestCidr1
	duplicateCidr.Cidr = "10.0.67.0/24"
	err = createCidr(t, store, duplicateCidr)
	expectError(t, err, "creating CIDR with duplicate name")
}

// TestCidrCreateDuplicateCidr tests that duplicate CIDR ranges are rejected
func TestCidrCreateDuplicateCidr(t *testing.T) {
	store := setupTestDB(t)

	// create root CIDR first
	err := createRootCidr(t, store, TestCidrRoot)
	expectNoError(t, err, "creating root CIDR")

	// create valid CIDR
	err = createCidr(t, store, TestCidr1)
	expectNoError(t, err, "creating valid CIDR")

	// attempt to create CIDR with same range but different name
	duplicateCidr := TestCidr1
	duplicateCidr.Name = "duplicate-subnet"
	err = createCidr(t, store, duplicateCidr)
	expectError(t, err, "creating CIDR with duplicate range")
}

// TestCidrGetExisting tests retrieving an existing CIDR
func TestCidrGetExisting(t *testing.T) {
	store := setupTestDB(t)

	// create root and subnet CIDRs
	err := createRootCidr(t, store, TestCidrRoot)
	expectNoError(t, err, "creating root CIDR")

	err = createCidr(t, store, TestCidr1)
	expectNoError(t, err, "creating subnet CIDR")

	// retrieve the CIDR
	cidr, err := store.CidrGet(TestCidr1.Name)
	expectNoError(t, err, "getting existing CIDR")

	// verify all CIDR fields match expected values
	if cidr.Name != TestCidr1.Name {
		t.Errorf("CIDR name = %v, want %v", cidr.Name, TestCidr1.Name)
	}
	if cidr.Cidr != TestCidr1.Cidr {
		t.Errorf("CIDR cidr = %v, want %v", cidr.Cidr, TestCidr1.Cidr)
	}
	if cidr.Length != TestCidr1.Length {
		t.Errorf("CIDR length = %v, want %v", cidr.Length, TestCidr1.Length)
	}
	if cidr.Prefix != TestCidr1.Prefix {
		t.Errorf("CIDR prefix = %v, want %v", cidr.Prefix, TestCidr1.Prefix)
	}
}

// TestCidrGetNonExistent tests retrieving a non-existent CIDR
func TestCidrGetNonExistent(t *testing.T) {
	store := setupTestDB(t)

	// attempt to get non-existent CIDR
	_, err := store.CidrGet("non-existent")
	expectError(t, err, "getting non-existent CIDR")
}

// TestCidrListMultiple tests listing multiple CIDRs
func TestCidrListMultiple(t *testing.T) {
	store := setupTestDB(t)

	// create root CIDR first
	err := createRootCidr(t, store, TestCidrRoot)
	expectNoError(t, err, "creating root CIDR")

	// create multiple subnet CIDRs
	cidrs := []TestCidrDesc{TestCidr1, TestCidr2, TestCidrSmall}
	err = createCidrs(t, store, cidrs)
	expectNoError(t, err, "creating multiple CIDRs")

	// list all CIDRs
	result, err := store.CidrList()
	expectNoError(t, err, "listing CIDRs")

	// verify correct number of CIDRs returned (including root)
	expectedCount := len(cidrs) + 1 // +1 for root CIDR
	if len(result) != expectedCount {
		t.Errorf("returned %d CIDRs, want %d", len(result), expectedCount)
	}

	// verify results are ordered by name ASC (as specified in SQL)
	for i := 1; i < len(result); i++ {
		if result[i-1].Name > result[i].Name {
			t.Errorf("results not ordered by name ASC: %v > %v",
				result[i-1].Name, result[i].Name)
		}
	}

	// verify each CIDR has required fields populated
	for _, cidr := range result {
		if cidr.Name == "" {
			t.Error("CIDR has empty name")
		}
		if cidr.Cidr == "" {
			t.Error("CIDR has empty cidr")
		}
		if cidr.Length <= 0 {
			t.Error("CIDR has invalid length")
		}
		if cidr.Prefix <= 0 {
			t.Error("CIDR has invalid prefix")
		}
	}
}

// TestCidrListEmpty tests listing when no CIDRs exist
func TestCidrListEmpty(t *testing.T) {
	store := setupTestDB(t)

	// list CIDRs from empty database
	result, err := store.CidrList()
	expectNoError(t, err, "listing empty CIDRs")

	// verify no CIDRs returned
	if len(result) != 0 {
		t.Errorf("empty db returned %d CIDRs, want 0", len(result))
	}
	assertCidrCount(t, store, 0)
}

// TestCidrRename tests renaming an existing CIDR
func TestCidrRename(t *testing.T) {
	store := setupTestDB(t)

	// create root and subnet CIDRs
	err := createRootCidr(t, store, TestCidrRoot)
	expectNoError(t, err, "creating root CIDR")

	err = createCidr(t, store, TestCidr1)
	expectNoError(t, err, "creating subnet CIDR")

	// rename the CIDR
	newName := "renamed-subnet"
	err = store.CidrRename(TestCidr1.Name, newName)
	expectNoError(t, err, "renaming CIDR")

	// verify old name no longer exists
	assertCidrNotExists(t, store, TestCidr1.Name)

	// verify new name exists and has correct properties
	renamedCidr, err := store.CidrGet(newName)
	expectNoError(t, err, "getting renamed CIDR")

	if renamedCidr.Name != newName {
		t.Errorf("renamed CIDR name = %v, want %v", renamedCidr.Name, newName)
	}
	if renamedCidr.Cidr != TestCidr1.Cidr {
		t.Errorf("renamed CIDR cidr changed unexpectedly: %v", renamedCidr.Cidr)
	}
	if renamedCidr.Length != TestCidr1.Length {
		t.Errorf("renamed CIDR length changed unexpectedly: %v", renamedCidr.Length)
	}
	if renamedCidr.Prefix != TestCidr1.Prefix {
		t.Errorf("renamed CIDR prefix changed unexpectedly: %v", renamedCidr.Prefix)
	}
}

// TestCidrRenameNonExistent tests renaming a non-existent CIDR
func TestCidrRenameNonExistent(t *testing.T) {
	store := setupTestDB(t)

	// attempt to rename non-existent CIDR
	err := store.CidrRename("non-existent", "new-name")
	expectNoError(t, err, "renaming non-existent CIDR should succeed silently")
}

// TestCidrRenameToExistingName tests renaming to a name that already exists
func TestCidrRenameToExistingName(t *testing.T) {
	store := setupTestDB(t)

	// create root CIDR first
	err := createRootCidr(t, store, TestCidrRoot)
	expectNoError(t, err, "creating root CIDR")

	// create two subnet CIDRs
	err = createCidr(t, store, TestCidr1)
	expectNoError(t, err, "creating first CIDR")

	err = createCidr(t, store, TestCidr2)
	expectNoError(t, err, "creating second CIDR")

	// attempt to rename first CIDR to second CIDR's name
	err = store.CidrRename(TestCidr1.Name, TestCidr2.Name)
	expectError(t, err, "renaming to existing name")
}

// TestCidrDelete tests deleting an existing CIDR
func TestCidrDelete(t *testing.T) {
	store := setupTestDB(t)

	// create root and subnet CIDRs
	err := createRootCidr(t, store, TestCidrRoot)
	expectNoError(t, err, "creating root CIDR")

	err = createCidr(t, store, TestCidr1)
	expectNoError(t, err, "creating subnet CIDR")

	// verify CIDR exists before deletion
	assertCidrExists(t, store, TestCidr1)

	// delete the CIDR
	err = store.CidrDelete(TestCidr1.Name)
	expectNoError(t, err, "deleting CIDR")

	// verify CIDR no longer exists
	assertCidrNotExists(t, store, TestCidr1.Name)

	// verify CIDR count decreased
	assertCidrCount(t, store, 1) // only root should remain
}

// TestCidrDeleteNonExistent tests deleting a non-existent CIDR
func TestCidrDeleteNonExistent(t *testing.T) {
	store := setupTestDB(t)

	// attempt to delete non-existent CIDR
	err := store.CidrDelete("non-existent")
	expectNoError(t, err, "deleting non-existent CIDR should succeed silently")
}

// TestCidrDeleteRoot tests deleting the root CIDR
func TestCidrDeleteRoot(t *testing.T) {
	store := setupTestDB(t)

	// create root CIDR
	err := createRootCidr(t, store, TestCidrRoot)
	expectNoError(t, err, "creating root CIDR")

	// delete root CIDR
	err = store.CidrDelete(TestCidrRoot.Name)
	expectNoError(t, err, "deleting root CIDR")

	// verify root CIDR no longer exists
	assertCidrNotExists(t, store, TestCidrRoot.Name)
	assertCidrCount(t, store, 0)
}
