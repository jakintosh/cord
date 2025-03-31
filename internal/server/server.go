package server

import (
	"database/sql"
	"fmt"
	"os"

	db "git.sr.ht/~jakintosh/innernet-go/internal/database"
	_ "modernc.org/sqlite"
)

type BackendType int

const (
	UndefinedBackend BackendType = iota
	KernelBackend
	UserspaceBackend
)

type Context struct {
	Db        *sql.DB
	Name      string
	ConfigDir string
	DataDir   string
}

func NewContext(
	network string,
	configDir string,
	dataDir string,
) (*Context, error) {

	os.MkdirAll(configDir, 0755)
	os.MkdirAll(dataDir, 0755)

	database, err := db.Open(network, dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	return &Context{
		Db:        database,
		Name:      network,
		ConfigDir: configDir,
		DataDir:   dataDir,
	}, nil
}

func (ctx *Context) Serve(
	noRouting bool,
	mtu int,
	backend BackendType,
) error {

	fmt.Println("Serve Network")
	fmt.Printf("network: %s\n", ctx.Name)
	fmt.Printf("configDir: %s\n", ctx.ConfigDir)
	fmt.Printf("dataDir: %s\n", ctx.DataDir)
	fmt.Printf("noRouting: %t\n", noRouting)
	fmt.Printf("mtu: %d\n", mtu)
	fmt.Printf("backend: %v\n", backend)

	return nil
}

func initServerDb(d *sql.DB) error {

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

	if err := db.InitTable(d, "peer", `
		CREATE TABLE IF NOT EXISTS peer (
			id					INTEGER PRIMARY KEY,
			cidr				INTEGER NOT NULL,
			public_key			TEXT NOT NULL UNIQUE,
			admin				INTEGER DEFAULT 0 NOT NULL,
			disabled			INTEGER DEFAULT 0 NOT NULL,
			redeemed			INTEGER DEFAULT 0 NOT NULL,
			invite_expires 		INTEGER,
			FOREIGN KEY (cidr)
				REFERENCES cidr (id)
		);
	`); err != nil {
		return err
	}

	if err := db.InitTable(d, "endpoint", `
		CREATE TABLE IF NOT EXISTS endpoint (
			id					INTEGER PRIMARY KEY,
			peer_ip				BLOB NOT NULL,
			peer_key			TEXT NOT NULL,
			endpoint			TEXT NOT NULL,
			time				INTEGER NOT NULL
		);
	`); err != nil {
		return err
	}

	return nil
}
