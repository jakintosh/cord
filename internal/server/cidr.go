package server

import (
	"fmt"
	"net"

	db "git.sr.ht/~jakintosh/innernet-go/internal/database"
	"git.sr.ht/~jakintosh/innernet-go/internal/utils"
)

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

func (ctx *Context) getPeerAndParentCidrIdsForPeerNamed(
	peerName string,
) (
	int64,
	int64,
	error,
) {
	// query the peer and parent cidr ids given the peer name
	row := ctx.Db.QueryRow(`
		SELECT client.id as client, parent.id as parent
		FROM cidr parent
		INNER JOIN (
			SELECT c.id, c.length, c.prefix, c.base 
			FROM peer p
			JOIN cidr c
			ON c.id=p.cidr
			WHERE c.name=?
		) as client
		WHERE parent.length=client.length
			AND parent.base<=client.base
			AND client.base<parent.last
			AND parent.prefix<client.prefix
			ORDER BY parent.prefix DESC
		LIMIT 1;
		`,
		peerName,
	)

	// scan the peer and parent cidr ids
	var peerCidrId int64
	var parentCidrId int64
	if err := row.Scan(&peerCidrId, &parentCidrId); err != nil {
		return -1, -1, db.CheckSqliteErr("getting peer and parent cidrs", err)
	}
	return peerCidrId, parentCidrId, nil
}
