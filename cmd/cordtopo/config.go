package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

type Config struct {
	Network      string              `json:"network"`
	Cidrs        []Cidr              `json:"cidrs"`
	Peers        []Peer              `json:"peers"`
	Associations []Association       `json:"associations"`
	Assignments  []Assignment        `json:"assignments"`
	Expected     map[string][]string `json:"expected"`
}

type Cidr struct {
	Name     string   `json:"name"`
	Cidr     string   `json:"cidr"`
	Groups   []string `json:"groups"`
	Terminal bool     `json:"terminal"`
}

type Peer struct {
	Name      string `json:"name"`
	Cidr      string `json:"cidr"`
	PublicKey string `json:"public_key"`
}

type Association struct {
	Group1 string `json:"group1"`
	Group2 string `json:"group2"`
}

type Assignment struct {
	Cidr  string `json:"cidr"`
	Group string `json:"group"`
}

func loadConfig(
	path string,
) (
	*Config,
	error,
) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return &c, nil
}

func setup(
	configPath string,
) (
	*service.TopologyState,
	*Config,
	*database.DB,
	error,
) {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return nil, nil, nil, err
	}

	db, err := seedDB(cfg)
	if err != nil {
		return nil, nil, nil, err
	}

	state, err := db.LoadTopologyState(cfg.Network)
	if err != nil {
		db.Close()
		return nil, nil, nil, fmt.Errorf("load topology: %w", err)
	}

	return state, cfg, db, nil
}

func seedDB(
	cfg *Config,
) (
	db *database.DB,
	err error,
) {
	if len(cfg.Cidrs) == 0 {
		return nil, fmt.Errorf("config has no CIDRs")
	}

	dbOpts := database.Options{
		Path: ":memory:",
		WAL:  false,
	}
	db, err = database.Open(dbOpts)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	defer func() {
		if err != nil {
			db.Close()
		}
	}()

	key, err := wireguard.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	pubKey, err := wireguard.PublicKey(key)
	if err != nil {
		return nil, fmt.Errorf("derive pub key: %w", err)
	}

	rootCidrStr := cfg.Cidrs[0].Cidr
	_, rootNet, _ := net.ParseCIDR(rootCidrStr)
	if rootNet == nil {
		return nil, fmt.Errorf("root CIDR %q is not a valid network", rootCidrStr)
	}

	prefix, bits := rootNet.Mask.Size()
	rootCidr := &service.Cidr{
		Name:   cfg.Cidrs[0].Name,
		Cidr:   cfg.Cidrs[0].Cidr,
		Prefix: prefix,
		Bits:   bits,
	}

	serverIP := netaddr.FirstAssignable(rootNet)
	serverRoute := netaddr.HostRoute(serverIP)
	serverCidr := &service.Cidr{
		Name:     "cord-server",
		Cidr:     serverRoute.String(),
		Prefix:   netaddr.TerminalPrefix(serverIP),
		Bits:     rootCidr.Bits,
		Terminal: true,
	}

	serverPeer := &service.Peer{
		Name:      "cord-server",
		CidrName:  "cord-server",
		Route:     serverRoute.String(),
		PublicKey: pubKey,
		Admin:     true,
		Enabled:   true,
		Confirmed: true,
	}

	netCfg := &service.Network{
		Name:       cfg.Network,
		PrivateKey: key,
		PublicKey:  pubKey,
		ExternalIP: "127.0.0.1",
		Main: service.Plane{
			Name:          cfg.Network,
			Cidr:          cfg.Cidrs[0].Cidr,
			WireguardPort: 51820,
			ApiPort:       8080,
		},
		Invite: service.Plane{
			Name:          cfg.Network + "-i",
			Cidr:          "172.16.10.0/24",
			WireguardPort: 51821,
			ApiPort:       8080,
		},
	}

	if err := db.BootstrapNetwork(netCfg, rootCidr, serverCidr, serverPeer); err != nil {
		return nil, fmt.Errorf("bootstrap network: %w", err)
	}

	seenGroups := make(map[string]bool)
	seenAssignments := make(map[string]bool)
	for i, cidr := range cfg.Cidrs {
		if i != 0 {
			_, n, err := net.ParseCIDR(cidr.Cidr)
			if err != nil {
				return nil, fmt.Errorf("parse cidr %q: %w", cidr.Name, err)
			}

			prefix, bits := n.Mask.Size()
			if err := db.CreateCidr(
				cfg.Network,
				&service.Cidr{
					Name:     cidr.Name,
					Cidr:     cidr.Cidr,
					Terminal: cidr.Terminal,
					Prefix:   prefix,
					Bits:     bits,
				},
			); err != nil {
				return nil, fmt.Errorf("insert cidr %q: %w", cidr.Name, err)
			}
		}

		for _, group := range cidr.Groups {
			if !seenGroups[group] {
				seenGroups[group] = true
				if _, err := db.InsertGroup(
					cfg.Network,
					group,
				); err != nil {
					return nil, fmt.Errorf("insert group %q: %w", group, err)
				}
			}
			key := cidr.Name + "/" + group
			if !seenAssignments[key] {
				seenAssignments[key] = true
				if err := db.AssignCidrGroup(
					cfg.Network,
					cidr.Name,
					group,
				); err != nil {
					return nil, fmt.Errorf("assign group %q to cidr %q: %w", group, cidr.Name, err)
				}
			}
		}
	}

	for _, assignment := range cfg.Assignments {
		if err := db.AssignCidrGroup(
			cfg.Network,
			assignment.Cidr,
			assignment.Group,
		); err != nil {
			return nil, fmt.Errorf("assign group %q to cidr %q: %w", assignment.Group, assignment.Cidr, err)
		}
	}

	for _, peer := range cfg.Peers {
		if err := db.InsertPeer(
			cfg.Network,
			&service.Peer{
				Name:      peer.Name,
				CidrName:  peer.Cidr,
				PublicKey: peer.PublicKey,
				Enabled:   true,
				Confirmed: true,
			},
		); err != nil {
			return nil, fmt.Errorf("insert peer %q: %w", peer.Name, err)
		}
	}

	for _, association := range cfg.Associations {
		if err := db.InsertAssociation(
			cfg.Network,
			&service.Association{
				Group1: association.Group1,
				Group2: association.Group2,
			},
		); err != nil {
			return nil, fmt.Errorf("insert association %q<->%q: %w", association.Group1, association.Group2, err)
		}
	}

	return db, nil
}
