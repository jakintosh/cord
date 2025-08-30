package server

import (
	"net"
)

type CidrDesc struct {
	Name string
	Cidr string
}

func (ctx *Context) CreateCidr(
	name string,
	cidr *net.IPNet,
) error {
	return ctx.Store.CidrCreate(name, cidr)
}

func (ctx *Context) CreateRootCidr(
	cidr *net.IPNet,
) error {
	return ctx.Store.CidrCreateRoot(ctx.Name, cidr)
}

func (ctx *Context) RenameCidr(
	cidr string,
	newName string,
) error {
	return ctx.Store.CidrRename(cidr, newName)
}

func (ctx *Context) DeleteCidr(
	cidr string,
) error {
	return ctx.Store.CidrDelete(cidr)
}
