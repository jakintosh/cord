package app

import (
	"fmt"
	"net"
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

	fmt.Printf("configDir: %s\n", configDir)
	fmt.Printf("dataDir: %s\n", dataDir)
	fmt.Printf("name: %s\n", name)
	fmt.Printf("cidr: %v\n", cidr)
	fmt.Printf("ip: %v\n", address)
	fmt.Printf("port: %d\n", port)

	// create a database
	initDatabase(name, dataDir)

	// create a root cidr
	// create a server cidr
	// create a server peer

	return nil
}
