package server

import (
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
	Config Config
	Store  NetworkStore
}

func NewContext(
	network string,
	config Config,
	store NetworkStore,
) (*Context, error) {

	ctx := &Context{
		Name:   network,
		Config: config,
		Store:  store,
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
	fmt.Printf("noRouting: %t\n", noRouting)
	fmt.Printf("mtu: %d\n", mtu)
	fmt.Printf("backend: %v\n", backend)

	return nil
}
