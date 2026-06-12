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

func (srv *Server) GetCidr(
	name string,
) (
	*Cidr,
	error,
) {
	return srv.Store.CidrGet(name)
}

func (srv *Server) ListCidrs() (
	[]*Cidr,
	error,
) {
	return srv.Store.CidrList()
}

func (srv *Server) CreateCidr(
	req CreateCidrRequest,
) error {
	cidr, err := parseCidr(req.Cidr)
	if err != nil {
		return err
	}

	return srv.Store.CidrCreate(req.Name, cidr)
}

func (srv *Server) UpdateCidr(
	cidr string,
	req UpdateCidrRequest,
) error {
	return srv.Store.CidrUpdate(cidr, req)
}

func (srv *Server) DeleteCidr(
	cidr string,
) error {
	return srv.Store.CidrDelete(cidr)
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
