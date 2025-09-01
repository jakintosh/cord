package server

import (
	"fmt"
	"net"
)

type Cidr struct {
	Name   string `json:"name"`
	Cidr   string `json:"cidr"`
	Length int    `json:"length"`
	Prefix int    `json:"prefix"`
}

type CreateCidrRequest struct {
	Name string `json:"name"`
	Cidr string `json:"cidr"`
}

type UpdateCidrRequest struct {
	Name string `json:"name"`
}

func (ctx *Context) CreateCidr(
	req CreateCidrRequest,
) error {
	cidr, err := parseCidr(req.Cidr)
	if err != nil {
		return err
	}

	return ctx.Store.CidrCreate(req.Name, cidr)
}

func (ctx *Context) CreateRootCidr(
	cidr *net.IPNet,
) error {
	return ctx.Store.CidrCreateRoot(ctx.Name, cidr)
}

func (ctx *Context) UpdateCidr(
	cidr string,
	req UpdateCidrRequest,
) error {
	return ctx.Store.CidrUpdate(cidr, req)
}

func (ctx *Context) DeleteCidr(
	cidr string,
) error {
	return ctx.Store.CidrDelete(cidr)
}

func parseCidr(
	value string,
) (
	*net.IPNet,
	error,
) {
	_, cidr, err := net.ParseCIDR(value)
	if err != nil {
		err = fmt.Errorf("failed to parse cidr '%s': %v", value, err)
	}
	return cidr, err
}
