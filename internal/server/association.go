package server

type Association struct {
	Cidr1 string `json:"cidr1"`
	Cidr2 string `json:"cidr2"`
}

func (ctx *Context) CreateAssociation(
	cidr1 string,
	cidr2 string,
) error {
	return ctx.Store.AssociationCreate(cidr1, cidr2)
}

func (ctx *Context) DeleteAssociation(
	cidr1 string,
	cidr2 string,
) error {
	return ctx.Store.AssociationDelete(cidr1, cidr2)
}

func (ctx *Context) GetAssociatedCidrIdsForCidrId(
	baseCidrId int64,
) (
	[]int64,
	error,
) {
	return ctx.Store.AssociationListAssociatedCidrIds(baseCidrId)
}
