package server

import (
	"fmt"
	"net"

	db "git.sr.ht/~jakintosh/innernet-go/internal/database"
	"git.sr.ht/~jakintosh/innernet-go/internal/utils"
)

type CidrDesc struct {
	Name string
	Cidr string
}

func (ctx *Context) CreateCidr(
	name string,
	cidr *net.IPNet,
) error {

	prefix, length := cidr.Mask.Size()
	base, last := utils.GetIpRangeFromCidr(cidr)

	result, err := ctx.Db.Exec(`
		INSERT INTO cidr (name, cidr, length, prefix, base, last)
		SELECT ?1, ?2, ?3, ?4, ?5, ?6
		FROM cidr c
		WHERE c.id = 1
			AND c.base <= ?5
			AND ?5 <= c.last;
		`,
		name, cidr.String(), length, prefix, base, last,
	)

	if err != nil {
		return db.CheckSqliteErr("adding cidr", err)
	}

	if db.ResultsEmpty(result) {
		return fmt.Errorf("Invalid CIDR")
	}

	return nil
}

func (ctx *Context) CreateRootCidr(
	cidr *net.IPNet,
) error {
	prefix, length := cidr.Mask.Size()
	base, last := utils.GetIpRangeFromCidr(cidr)

	_, err := ctx.Db.Exec(`
		INSERT INTO cidr (id, name, cidr, length, prefix, base, last)
		VALUES (1, ?, ?, ?, ?, ?, ?);
		`,
		ctx.Name, cidr.String(), length, prefix, base, last,
	)

	return db.CheckSqliteErr("adding root cidr", err)
}

func (ctx *Context) RenameCidr(
	cidr string,
	newName string,
) error {

	// TODO: what if you call rename CIDR on a Peer cidr?
	// 		 probably need to check, because that is prob not intended
	// 		 however, right now, RenamePeer points to this func, so more
	// 			work is needed before changing

	_, err := ctx.Db.Exec(`
		UPDATE cidr
		SET name=?2
		WHERE name=?1;
		`,
		cidr, newName,
	)
	return db.CheckSqliteErr("renaming cidr", err)
}

func (ctx *Context) DeleteCidr(
	cidr string,
) error {

	_, err := ctx.Db.Exec(`
		DELETE FROM cidr
		WHERE name = ?;
		`,
		cidr,
	)
	return db.CheckSqliteErr("deleting cidr", err)
}
