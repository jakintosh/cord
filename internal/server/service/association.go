package service

type Association struct {
	Cidr1 string
	Cidr2 string
}

func (s *Service) ListAssociations(
	network string,
) (
	[]*Association,
	error,
) {
	return nil, ErrNotImplemented
}

func (s *Service) AddAssociation(
	network string,
	cidr1 string,
	cidr2 string,
) error {
	return ErrNotImplemented
}

func (s *Service) RemoveAssociation(
	network string,
	cidr1 string,
	cidr2 string,
) error {
	return ErrNotImplemented
}
