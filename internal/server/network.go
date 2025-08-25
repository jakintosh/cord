package server

import (
	"database/sql"
	"fmt"
	"net"

	db "git.sr.ht/~jakintosh/cord/internal/database"
	"git.sr.ht/~jakintosh/cord/internal/utils"
)

type NetworkDesc struct {
	Name string
	Cidr string
	Ip   net.IP
	Port uint16
}

func (ctx *Context) CreateNetwork(
	cidr *net.IPNet,
	address net.IP,
	port uint16,
) error {

	if err := utils.ValidateHostName(ctx.Name); err != nil {
		return fmt.Errorf("failed to validate network name: %w", err)
	}

	// make sure we get file handle before we do all the db work
	fileName := ctx.Name + ".toml"
	cfgFile, err := ctx.Config.GetConfigWriter(fileName)
	if err != nil {
		return fmt.Errorf("failed to create config writer: %w", err)
	}

	if err := initNetworkDb(ctx.Db); err != nil {
		return fmt.Errorf("failed to init database: %w", err)
	}

	if err := ctx.CreateRootCidr(cidr); err != nil {
		return fmt.Errorf("failed to add root cidr: %w", err)
	}

	serverIp := utils.GetFirstAssignableIpFromCidr(cidr)
	deviceCfg, peerCfg, err := ctx.CreatePeer("cord-server", serverIp, true, 0)
	if err != nil {
		return fmt.Errorf("failed to add server peer: %w", err)
	}

	pubKey := peerCfg.PublicKey.String()
	if err := ctx.RedeemPeer(pubKey, pubKey); err != nil {
		return fmt.Errorf("failed to redeem server peer: %w", err)
	}

	// TODO: also write out the server config file here

	// TODO: what is this supposed to be doing here?
	err = peerCfg.WriteInvite(cfgFile, deviceCfg)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func (ctx *Context) DeleteNetwork() error {

	// a network is really just a sqlite database file
	if err := ctx.Data.DeleteDatabase(ctx.Name); err != nil {
		return fmt.Errorf("failed to delete network: %w", err)
	}
	return nil
}

func initNetworkDb(d *sql.DB) error {

	if err := db.EnableForeignKeys(d); err != nil {
		return err
	}

	if err := db.InitTable(d, "cidr", `
		CREATE TABLE IF NOT EXISTS cidr (
			id					INTEGER PRIMARY KEY,
			name				TEXT NOT NULL UNIQUE,
			cidr				TEXT NOT NULL UNIQUE,
			length				INTEGER NOT NULL,
			prefix				INTEGER NOT NULL,
			base				BLOB NOT NULL,
			last				BLOB NOT NULL,
			UNIQUE (base, prefix)
		);
	`); err != nil {
		return err
	}

	if err := db.InitTable(d, "association", `
		CREATE TABLE IF NOT EXISTS association (
			id					INTEGER PRIMARY KEY,
			cidr1				INTEGER NOT NULL,
			cidr2				INTEGER NOT NULL,
			FOREIGN KEY (cidr1)
				REFERENCES cidr (id)
					ON UPDATE RESTRICT,
			FOREIGN KEY (cidr2)
				REFERENCES cidr (id)
					ON UPDATE RESTRICT
		);
	`); err != nil {
		return err
	}

	// Invites used during peer redemption
	if err := db.InitTable(d, "invite", `
		CREATE TABLE IF NOT EXISTS invite (
			id					INTEGER PRIMARY KEY,
			public_key			TEXT NOT NULL UNIQUE,
			temp_cidr			TEXT NOT NULL UNIQUE,
			final_cidr			INTEGER NOT NULL UNIQUE,
			name				TEXT NOT NULL,
			admin				INTEGER DEFAULT 0 NOT NULL,
			redeemed			INTEGER DEFAULT 0 NOT NULL,
			expiration			INTEGER NOT NULL,
			FOREIGN KEY (final_cidr)
				REFERENCES cidr (id)
		);
	`); err != nil {
		return err
	}

	// Active peers on the main network
	if err := db.InitTable(d, "peer", `
		CREATE TABLE IF NOT EXISTS peer (
			id					INTEGER PRIMARY KEY,
			cidr				INTEGER NOT NULL UNIQUE,
			public_key			TEXT NOT NULL UNIQUE,
			admin				INTEGER DEFAULT 0 NOT NULL,
			disabled			INTEGER DEFAULT 0 NOT NULL,
			confirmed			INTEGER DEFAULT 0 NOT NULL,
			FOREIGN KEY (cidr)
				REFERENCES cidr (id)
		);
	`); err != nil {
		return err
	}

	// Endpoint sightings recorded by peers
	if err := db.InitTable(d, "endpoint", `
		CREATE TABLE IF NOT EXISTS endpoint (
			id					INTEGER PRIMARY KEY,
			witness				INTEGER NOT NULL,
			peer				INTEGER NOT NULL,
			endpoint			TEXT NOT NULL,
			time				INTEGER NOT NULL,
			FOREIGN KEY (peer)
				REFERENCES peer (id),
			FOREIGN KEY (witness)
				REFERENCES peer (id)
		);
	`); err != nil {
		return err
	}

	return nil
}
