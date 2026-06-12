package server

type Association struct {
	Cidr1 string `json:"cidr1"`
	Cidr2 string `json:"cidr2"`
}

func (srv *Server) ListAssociations() (
	[]*Association,
	error,
) {
	return srv.Store.AssociationList()
}

func (srv *Server) CreateAssociation(
	cidr1 string,
	cidr2 string,
) error {
	return srv.Store.AssociationCreate(cidr1, cidr2)
}

func (srv *Server) DeleteAssociation(
	cidr1 string,
	cidr2 string,
) error {
	return srv.Store.AssociationDelete(cidr1, cidr2)
}
