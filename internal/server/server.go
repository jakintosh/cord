package server

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type BackendType int

const (
	UndefinedBackend BackendType = iota
	KernelBackend
	UserspaceBackend
)

type Context struct {
	Name   string
	Db     *sql.DB
	Config Config
	Data   Data
}

func NewContext(
	network string,
	config Config,
	data Data,
) (*Context, error) {

	database, err := data.OpenDatabase(network)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	ctx := &Context{
		Name:   network,
		Db:     database,
		Config: config,
		Data:   data,
	}

	return ctx, nil
}

func (ctx *Context) Serve(
	noRouting bool,
	mtu int,
	backend BackendType,
) error {

	fmt.Println("Serve Network")
	fmt.Printf("network: %s\n", ctx.Name)
	fmt.Printf("configDir: %s\n", ctx.Config)
	fmt.Printf("dataDir: %s\n", ctx.Data)
	fmt.Printf("noRouting: %t\n", noRouting)
	fmt.Printf("mtu: %d\n", mtu)
	fmt.Printf("backend: %v\n", backend)

	return nil
}
