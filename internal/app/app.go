package app

import (
	"database/sql"
	"fmt"
	"net"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	_ "modernc.org/sqlite"
)

type BackendType int

const (
	UndefinedBackend BackendType = iota
	KernelBackend
	UserspaceBackend
)

func Init(configPath string) {}

func ServeNetwork(
	configDir string,
	dataDir string,
	network string,
	noRouting bool,
	mtu int,
	backend BackendType,
) error {

	fmt.Println("Serve Network")
	fmt.Printf("network: %s\n", network)
	fmt.Printf("configDir: %s\n", configDir)
	fmt.Printf("dataDir: %s\n", dataDir)
	fmt.Printf("noRouting: %t\n", noRouting)
	fmt.Printf("mtu: %d\n", mtu)
	fmt.Printf("backend: %v\n", backend)

	return nil
}

func CreateNetwork(
	configDir string,
	dataDir string,
	name string,
	cidr *net.IPNet,
	address net.IP,
	port uint16,
) error {

	fmt.Println("Create Network")

	// create a database
	db, err := initDatabase(name, dataDir)
	if err != nil {
		return fmt.Errorf("failed to init database: %w", err)
	}

	// create root cidr
	err = addRootCidr(db, name, cidr)
	if err != nil {
		return err
	}

	// add server cidr peer
	serverIp := firstAssignableIp(cidr)
	serverCidr := peerCidr(serverIp)
	err = addCidr(db, "innernet-server", serverCidr)
	if err != nil {
		return err
	}

	return nil
}

func DeleteNetwork(
	configDir string,
	dataDir string,
	network string,
) error {

	fmt.Println("Delete Network")

	return deleteDatabase(network, dataDir)
}

func AddCidr(
	configDir string,
	dataDir string,
	network string,
	name string,
	cidr *net.IPNet,
	parent string,
) error {

	fmt.Println("Add Cidr")

	db, err := openDatabase(network, dataDir)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	return addCidr(db, name, cidr)
}

func RenameCidr(
	configDir string,
	dataDir string,
	network string,
	cidr string,
	newName string,
) error {

	fmt.Println("Rename Cidr")

	db, err := openDatabase(network, dataDir)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	return renameCidr(db, cidr, newName)
}

func DeleteCidr(
	configDir string,
	dataDir string,
	network string,
	cidr string,
) error {

	fmt.Println("Delete Cidr")

	db, err := openDatabase(network, dataDir)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	return deleteCidr(db, cidr)
}

func AddPeer(
	configDir string,
	dataDir string,
	network string,
	name string,
	ip net.IP,
	admin bool,
	savePath string,
	inviteExpires int64,
) error {

	fmt.Println("Add Peer")

	db, err := openDatabase(network, dataDir)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	pub, priv, err := generateKeypair()
	if err != nil {
		return err
	}

	cidr := peerCidr(ip)
	err = addCidr(db, name, cidr)
	if err != nil {
		return err
	}

	err = addPeer(db, name, pub, admin, inviteExpires)
	if err != nil {
		return err
	}

	return writeInvite(savePath, name, ip, priv)
}

func RenamePeer(
	configDir string,
	dataDir string,
	network string,
	peer string,
	newName string,
) error {

	fmt.Println("Rename Peer")

	db, err := openDatabase(network, dataDir)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	return renameCidr(db, peer, newName)
}

func EnablePeer(
	configDir string,
	dataDir string,
	network string,
	peer string,
) error {

	fmt.Println("Enable Peer")

	db, err := openDatabase(network, dataDir)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	return setPeerEnabled(db, peer, true)
}

func DisablePeer(
	configDir string,
	dataDir string,
	network string,
	peer string,
) error {

	fmt.Println("Disable Peer")

	db, err := openDatabase(network, dataDir)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	return setPeerEnabled(db, peer, false)
}

func AddAssociation(
	configDir string,
	dataDir string,
	network string,
	cidr1 string,
	cidr2 string,
) error {

	fmt.Println("Add Association")

	db, err := openDatabase(network, dataDir)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	return addAssociation(db, cidr1, cidr2)
}

func DeleteAssociation(
	configDir string,
	dataDir string,
	network string,
	cidr1 string,
	cidr2 string,
) error {

	fmt.Println("Delete Association")

	db, err := openDatabase(network, dataDir)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	return deleteAssociation(db, cidr1, cidr2)
}

func addRootCidr(
	db *sql.DB,
	name string,
	cidr *net.IPNet,
) error {
	prefix, length := cidr.Mask.Size()
	base, last := rangeFromCidr(cidr)

	_, err := db.Exec(`
		INSERT INTO cidr (id, name, cidr, length, prefix, base, last)
		VALUES (1, ?, ?, ?, ?, ?, ?);
		`,
		name, cidr.String(), length, prefix, base, last,
	)

	return checkSqliteErr(err)
}

func addCidr(
	db *sql.DB,
	name string,
	cidr *net.IPNet,
) error {

	prefix, length := cidr.Mask.Size()
	base, last := rangeFromCidr(cidr)

	result, err := db.Exec(`
		INSERT INTO cidr (name, cidr, length, prefix, base, last)
		SELECT ?1, ?2, ?3, ?4, ?5, ?6
		FROM cidr c
		WHERE c.id = 1
		AND c.base <= ?5
		AND ?5 <= c.last;
		`,
		name, cidr.String(), length, prefix, base, last,
	)
	if err := checkSqliteErr(err); err != nil {
		return err
	}

	if resultsEmpty(result) {
		return fmt.Errorf("Invalid CIDR")
	}

	return nil
}

func renameCidr(
	db *sql.DB,
	name string,
	newName string,
) error {

	_, err := db.Exec(`
		UPDATE cidr
		SET name=?2
		WHERE name=?1;
		`,
		name,
		newName)
	return checkSqliteErr(err)
}

func deleteCidr(
	db *sql.DB,
	name string,
) error {

	_, err := db.Exec(`
		DELETE FROM cidr
		WHERE name = ?;
		`,
		name,
	)
	return checkSqliteErr(err)
}

func addPeer(
	db *sql.DB,
	name string,
	pubkey wgtypes.Key,
	admin bool,
	inviteExpires int64,
) error {

	_, err := db.Exec(`
		INSERT INTO peer (cidr, public_key, admin, invite_expires)
		SELECT c.id, ?2, ?3, ?4
		FROM cidr c
		WHERE c.name=?1;
		`,
		name, pubkey.String(), admin, inviteExpires,
	)
	return checkSqliteErr(err)
}

func setPeerEnabled(
	db *sql.DB,
	peer string,
	enabled bool,
) error {
	_, err := db.Exec(`
		UPDATE peer
		SET disabled=?2
		WHERE name=?1;
		`,
		peer,
		!enabled,
	)
	return checkSqliteErr(err)
}

func addAssociation(
	db *sql.DB,
	a string,
	b string,
) error {
	_, err := db.Exec(`
		INSERT INTO association (cidr1, cidr2)
		SELECT c1.id, c2.id
		FROM cidr c1, cidr c2
		WHERE c1.name=? AND c2.name=?;
		`,
		a,
		b,
	)
	return checkSqliteErr(err)
}

func deleteAssociation(
	db *sql.DB,
	a string,
	b string,
) error {
	_, err := db.Exec(`
		DELETE FROM association
		WHERE id in (
			SELECT a.id
			FROM association a
			JOIN cidr c1 ON c1.id=a.cidr1
			JOIN cidr c2 ON c2.id=a.cidr2
			WHERE (c1.name=?1 AND c2.name=?2)
			OR (c1.name=?2 AND c2.name=?1)
		);
		`,
		a,
		b,
	)
	return checkSqliteErr(err)
}

func writeInvite(
	path string,
	name string,
	ip net.IP,
	privKey wgtypes.Key,
) error {
	fmt.Printf(
		"Writing to: %s\n\n[Peer]\nname=%s\nip=%s\nprivateKey=%s",
		path, name, ip.String(), privKey.String(),
	)
	return nil
}
