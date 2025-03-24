package app

import (
	"database/sql"
	"fmt"
	"net"

	_ "modernc.org/sqlite"
)

type BackendType int

const (
	UndefinedBackend BackendType = iota
	KernelBackend
	UserspaceBackend
)

func Init(configPath string) {

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
	fmt.Printf("configDir: %s\n", configDir)
	fmt.Printf("dataDir: %s\n", dataDir)
	fmt.Printf("name: %s\n", name)
	fmt.Printf("cidr: %v\n", cidr)
	fmt.Printf("ip: %v\n", address)
	fmt.Printf("port: %d\n", port)

	// create a database
	db, err := initDatabase(name, dataDir)
	if err != nil {
		return fmt.Errorf("failed to init database: %w", err)
	}

	// root cidr
	addCidr(db, name, cidr, true)

	// server cidr
	ip := firstAssignableIp(cidr)
	l := len(ip) * 8
	serverCidr := &net.IPNet{
		IP:   ip,
		Mask: net.CIDRMask(l, l),
	}
	addCidr(db, "innernet-server", serverCidr, false)

	// create a server peer
	fmt.Printf("creating peer 'innernet-server'\n")

	return nil
}

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

func DeleteNetwork(
	configDir string,
	dataDir string,
	network string,
) error {

	fmt.Println("Delete Network")
	fmt.Printf("configDir: %s\n", configDir)
	fmt.Printf("dataDir: %s\n", dataDir)
	fmt.Printf("network: %s\n", network)

	return nil
}

func addCidr(
	db *sql.DB,
	name string,
	cidr *net.IPNet,
	root bool,
) error {

	prefix, length := cidr.Mask.Size()
	base, last := rangeFromCidr(cidr)

	var err error
	if root {
		_, err = db.Exec(`
			INSERT INTO cidr (id, name, cidr, length, prefix, base, last)
			VALUES (1, ?, ?, ?, ?, ?, ?);`,
			name, cidr.String(), length, prefix, base, last,
		)
	} else {
		_, err = db.Exec(`
			INSERT INTO cidr (name, cidr, length, prefix, base, last)
			SELECT ?1, ?2, ?3, ?4, ?5, ?6
			FROM cidr c
			WHERE c.id = 1
			AND c.base <= ?5
			AND ?5 <= c.last;`,
			name, cidr.String(), length, prefix, base, last,
		)
	}
	if err != nil {
		return fmt.Errorf("failed to insert into cidr: %w", err)
	}

	return nil
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
	fmt.Printf("configDir: %s\n", configDir)
	fmt.Printf("dataDir: %s\n", dataDir)
	fmt.Printf("network: %s\n", network)
	fmt.Printf("name: %s\n", name)
	fmt.Printf("cidr: %s\n", cidr)
	fmt.Printf("parent: %s\n", parent)

	db, err := openDatabase(network, dataDir)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	return addCidr(db, name, cidr, false)
}

func RenameCidr(
	configDir string,
	dataDir string,
	network string,
	cidr string,
	newName string,
) error {

	fmt.Println("Rename Cidr")
	fmt.Printf("configDir: %s\n", configDir)
	fmt.Printf("dataDir: %s\n", dataDir)
	fmt.Printf("network: %s\n", network)
	fmt.Printf("cidr: %s\n", cidr)
	fmt.Printf("new-name: %s\n", newName)

	return nil
}

func DeleteCidr(
	configDir string,
	dataDir string,
	network string,
	cidr string,
) error {

	fmt.Println("Delete Cidr")
	fmt.Printf("configDir: %s\n", configDir)
	fmt.Printf("dataDir: %s\n", dataDir)
	fmt.Printf("network: %s\n", network)
	fmt.Printf("cidr: %s\n", cidr)

	return nil
}

func AddPeer(
	configDir string,
	dataDir string,
	network string,
	name string,
	cidr string,
	ip net.IP,
	admin bool,
	savePath string,
	inviteExpires int64,
) error {

	fmt.Println("Add Peer")
	fmt.Printf("configDir: %s\n", configDir)
	fmt.Printf("dataDir: %s\n", dataDir)
	fmt.Printf("network: %s\n", network)
	fmt.Printf("name: %s\n", name)
	fmt.Printf("cidr: %s\n", cidr)
	fmt.Printf("ip: %v\n", ip)
	fmt.Printf("admin: %t\n", admin)
	fmt.Printf("save-path: %s\n", savePath)
	fmt.Printf("invite-expires: %d\n", inviteExpires)

	return nil
}

func RenamePeer(
	configDir string,
	dataDir string,
	network string,
	peer string,
	newName string,
) error {

	fmt.Println("Rename Peer")
	fmt.Printf("configDir: %s\n", configDir)
	fmt.Printf("dataDir: %s\n", dataDir)
	fmt.Printf("network: %s\n", network)
	fmt.Printf("peer: %s\n", peer)
	fmt.Printf("new-name: %s\n", newName)

	return nil
}

func EnablePeer(
	configDir string,
	dataDir string,
	network string,
	peer string,
) error {

	fmt.Println("Enable Peer")
	fmt.Printf("configDir: %s\n", configDir)
	fmt.Printf("dataDir: %s\n", dataDir)
	fmt.Printf("network: %s\n", network)
	fmt.Printf("peer: %s\n", peer)

	return nil
}

func DisablePeer(
	configDir string,
	dataDir string,
	network string,
	peer string,
) error {

	fmt.Println("Disable Peer")
	fmt.Printf("configDir: %s\n", configDir)
	fmt.Printf("dataDir: %s\n", dataDir)
	fmt.Printf("network: %s\n", network)
	fmt.Printf("peer: %s\n", peer)

	return nil
}

func AddAssociation(
	configDir string,
	dataDir string,
	network string,
	cidr1 string,
	cidr2 string,
) error {

	fmt.Println("Add Association")
	fmt.Printf("configDir: %s\n", configDir)
	fmt.Printf("dataDir: %s\n", dataDir)
	fmt.Printf("network: %s\n", network)
	fmt.Printf("cidr1: %s\n", cidr1)
	fmt.Printf("cidr2: %s\n", cidr2)

	return nil
}

func DeleteAssociation(
	configDir string,
	dataDir string,
	network string,
	cidr1 string,
	cidr2 string,
) error {

	fmt.Println("Delete Association")
	fmt.Printf("configDir: %s\n", configDir)
	fmt.Printf("dataDir: %s\n", dataDir)
	fmt.Printf("network: %s\n", network)
	fmt.Printf("cidr1: %s\n", cidr1)
	fmt.Printf("cidr2: %s\n", cidr2)

	return nil
}
