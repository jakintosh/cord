package database

import (
	"fmt"
	"net"

	"git.sr.ht/~jakintosh/cord/internal/server"
	"git.sr.ht/~jakintosh/cord/internal/utils"
)

func (store *SQLiteStore) CidrList() (
	[]*server.Cidr,
	error,
) {
	rows, err := store.db.Query(`
		SELECT name, cidr, length, prefix
		FROM cidr
		ORDER BY name ASC;`,
	)
	if err != nil {
		return nil, CheckSqliteErr("querying cidrs", err)
	}
	defer rows.Close()

	var cidrs []*server.Cidr
	for rows.Next() {
		cidr, err := scanCidr(rows)
		if err != nil {
			return nil, err
		}
		cidrs = append(cidrs, cidr)
	}

	return cidrs, nil
}

func (store *SQLiteStore) CidrGet(
	name string,
) (
	*server.Cidr,
	error,
) {
	row := store.db.QueryRow(`
		SELECT name, cidr, length, prefix
		FROM cidr
		WHERE name = ?1;`,
		name,
	)

	return scanCidr(row)
}

func (s *SQLiteStore) CidrCreate(
	name string,
	cidr *net.IPNet,
) error {
	prefix, length := cidr.Mask.Size()
	base, last := utils.GetIpRangeFromCidr(cidr)
	result, err := s.db.Exec(`
		INSERT INTO cidr (name, cidr, length, prefix, base, last)
		SELECT ?1, ?2, ?3, ?4, ?5, ?6
		FROM cidr c
		WHERE c.id = 1
			AND c.base <= ?5
			AND ?5 <= c.last;`,
		name,
		cidr.String(),
		length,
		prefix,
		base,
		last,
	)

	if err != nil {
		return CheckSqliteErr("adding cidr", err)
	}

	if ResultsEmpty(result) {
		return fmt.Errorf("Invalid CIDR")
	}

	return nil
}

func (s *SQLiteStore) CidrCreateRoot(
	name string,
	cidr *net.IPNet,
) error {
	prefix, length := cidr.Mask.Size()
	base, last := utils.GetIpRangeFromCidr(cidr)
	_, err := s.db.Exec(`
		INSERT INTO cidr (id, name, cidr, length, prefix, base, last)
		VALUES (1, ?, ?, ?, ?, ?, ?);`,
		name,
		cidr.String(),
		length,
		prefix,
		base,
		last,
	)

	return CheckSqliteErr("adding root cidr", err)
}

func (s *SQLiteStore) CidrRename(
	name string,
	newName string,
) error {
	_, err := s.db.Exec(`
		UPDATE cidr
		SET name = ?2
		WHERE name = ?1;`,
		name, newName,
	)
	return CheckSqliteErr("renaming cidr", err)
}

func (s *SQLiteStore) CidrDelete(
	name string,
) error {
	_, err := s.db.Exec(`
		DELETE FROM cidr
		WHERE name = ?;`,
		name,
	)
	return CheckSqliteErr("deleting cidr", err)
}

func scanCidr(s Scanner) (
	*server.Cidr,
	error,
) {
	var name, cidrStr string
	var length, prefix int

	err := s.Scan(
		&name,
		&cidrStr,
		&length,
		&prefix,
	)
	if err != nil {
		return nil, CheckSqliteErr("scanning cidr info", err)
	}

	cidr := &server.Cidr{
		Name:   name,
		Cidr:   cidrStr,
		Length: length,
		Prefix: prefix,
	}

	return cidr, nil
}
