package database

import (
	"fmt"
	"net"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func (db *DB) GetCidr(
	network string,
	name string,
) (
	*service.Cidr,
	error,
) {
	row := db.Conn.QueryRow(`
		SELECT name, cidr, length, prefix, terminal
		FROM cidr
		WHERE network_name = ?1
			AND name = ?2`,
		network,
		name,
	)

	return scanCidr(row)
}

func (db *DB) ListCidrs(
	network string,
) (
	[]*service.Cidr,
	error,
) {
	rows, err := db.Conn.Query(`
		SELECT name, cidr, length, prefix, terminal
		FROM cidr
		WHERE network_name = ?1
		ORDER BY name ASC`,
		network,
	)
	if err != nil {
		return nil, CheckSqliteErr("list cidrs", err)
	}
	defer rows.Close()

	var cidrs []*service.Cidr
	for rows.Next() {
		cidr, err := scanCidr(rows)
		if err != nil {
			return nil, err
		}
		cidrs = append(cidrs, cidr)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cidrs: %w", err)
	}

	return cidrs, nil
}

func (db *DB) InsertCidr(
	network string,
	cidr *service.Cidr,
) error {
	_, ipNet, err := net.ParseCIDR(cidr.Cidr)
	if err != nil {
		return fmt.Errorf("insert cidr %q: parse cidr: %w", cidr.Name, err)
	}

	ones, bits := ipNet.Mask.Size()
	first, last := cidrFirstAndLast(ipNet)

	_, err = db.Conn.Exec(`
		INSERT INTO cidr (network_name, name, cidr, length, prefix, base, last, terminal)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)`,
		network,
		cidr.Name,
		cidr.Cidr,
		bits,
		ones,
		first,
		last,
		boolToInt(cidr.Terminal),
	)
	return CheckSqliteErr("insert cidr", err)
}

func (db *DB) UpdateCidr(
	network string,
	name string,
	newName string,
) (
	*service.Cidr,
	error,
) {
	row := db.Conn.QueryRow(`
		UPDATE cidr
		SET name = ?3
		WHERE network_name = ?1
			AND name = ?2
		RETURNING name, cidr, length, prefix, terminal`,
		network,
		name,
		newName,
	)

	return scanCidr(row)
}

func (db *DB) DeleteCidr(
	network string,
	name string,
) error {
	result, err := db.Conn.Exec(`
		DELETE FROM cidr
		WHERE network_name = ?1
			AND name = ?2`,
		network,
		name,
	)
	if err != nil {
		return CheckSqliteErr("delete cidr", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete cidr rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: cidr %q not found", service.ErrNotFound, name)
	}

	return nil
}

func scanCidr(
	s Scanner,
) (
	*service.Cidr,
	error,
) {
	var name string
	var cidrStr string
	var length int
	var prefix int
	var terminal int64

	if err := s.Scan(&name, &cidrStr, &length, &prefix, &terminal); err != nil {
		return nil, CheckSqliteErr("scan cidr", err)
	}

	return &service.Cidr{
		Name:     name,
		Cidr:     cidrStr,
		Prefix:   prefix,
		Bits:     length,
		Terminal: terminal != 0,
	}, nil
}

func cidrFirstAndLast(
	cidr *net.IPNet,
) (
	first net.IP,
	last net.IP,
) {
	first = cidr.IP.Mask(cidr.Mask)
	ones, bits := cidr.Mask.Size()
	if ones == 0 {
		return first, first
	}
	last = make(net.IP, len(first))
	copy(last, first)
	for i := range last {
		last[i] |= ^cidr.Mask[i]
	}
	_ = bits
	return first, last
}
