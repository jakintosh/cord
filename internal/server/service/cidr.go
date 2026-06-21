package service

type Cidr struct {
	Name   string
	Cidr   string
	Length int
	Prefix int
}

type CreateCidrRequest struct {
	Name string
	Cidr string
}

type UpdateCidrRequest struct {
	Name string
}

func (s *Service) GetCidr(
	network string,
	name string,
) (
	*Cidr,
	error,
) {
	return nil, ErrNotImplemented
}

func (s *Service) ListCidrs(
	network string,
) (
	[]*Cidr,
	error,
) {
	return nil, ErrNotImplemented
}

func (s *Service) AddCidr(
	network string,
	req CreateCidrRequest,
) error {
	return ErrNotImplemented
}

func (s *Service) RemoveCidr(
	network string,
	name string,
) error {
	return ErrNotImplemented
}

func (s *Service) UpdateCidr(
	network string,
	name string,
	req UpdateCidrRequest,
) error {
	return ErrNotImplemented
}
