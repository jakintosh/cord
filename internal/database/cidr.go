package database

import (
	"fmt"
	"net"

	"git.sr.ht/~jakintosh/cord/internal/server"
	"git.sr.ht/~jakintosh/cord/internal/utils"
)

func (store *ServerDB) CidrList() (
	[]*server.Cidr,
	error,
) {
	rows, err := store.Conn.Query(`
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

func (store *ServerDB) CidrGet(
	name string,
) (
	*server.Cidr,
	error,
) {
	row := store.Conn.QueryRow(`
		SELECT name, cidr, length, prefix
		FROM cidr
		WHERE name = ?1;`,
		name,
	)

	return scanCidr(row)
}

func (s *ServerDB) CidrCreate(
	name string,
	cidr *net.IPNet,
) error {
	prefix, length := cidr.Mask.Size()
	base, last := utils.GetIpRangeFromCidr(cidr)
	result, err := s.Conn.Exec(`
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
		return fmt.Errorf("%w: cidr is outside the root range", server.ErrInvalid)
	}

	return nil
}

func (s *ServerDB) CidrCreateRoot(
	name string,
	cidr *net.IPNet,
) error {
	prefix, length := cidr.Mask.Size()
	base, last := utils.GetIpRangeFromCidr(cidr)
	_, err := s.Conn.Exec(`
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

func (s *ServerDB) CidrUpdate(
	name string,
	req server.UpdateCidrRequest,
) error {
	_, err := s.Conn.Exec(`
		UPDATE cidr
		SET name = ?2
		WHERE name = ?1;`,
		name,
		req.Name,
	)
	return CheckSqliteErr("renaming cidr", err)
}

// CidrDelete removes a CIDR and its associations. The root CIDR
// (id=1) cannot be deleted.
func (s *ServerDB) CidrDelete(
	name string,
) error {
	tx, err := s.Conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin delete tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`
		DELETE FROM association
		WHERE cidr1 IN (SELECT id FROM cidr WHERE name = ?1 AND id != 1)
		   OR cidr2 IN (SELECT id FROM cidr WHERE name = ?1 AND id != 1);`,
		name,
	); err != nil {
		return CheckSqliteErr("deleting cidr associations", err)
	}

	res, err := tx.Exec(`
		DELETE FROM cidr
		WHERE name = ?1
		  AND id != 1;`,
		name,
	)
	if err != nil {
		return CheckSqliteErr("deleting cidr", err)
	}
	if ResultsEmpty(res) {
		var id int
		if lookupErr := tx.QueryRow(`
			SELECT id FROM cidr WHERE name = ?1;`,
			name,
		).Scan(&id); lookupErr == nil {
			return fmt.Errorf("%w: the root cidr cannot be deleted", server.ErrConflict)
		}
		return fmt.Errorf("%w: no cidr named '%s'", server.ErrNotFound, name)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit delete tx: %w", err)
	}

	return nil
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
