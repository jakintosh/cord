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

	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin insert CIDR tx: %w", err)
	}
	defer tx.Rollback()

	var registrationConflict int
	err = tx.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM registration
			WHERE network_name = ?1
				AND confirmed = 0
				AND (name = ?2 OR final_route = ?3)
		)`,
		network,
		cidr.Name,
		ipNet.String(),
	).Scan(&registrationConflict)
	if err != nil {
		return CheckSqliteErr("check CIDR registration reservation", err)
	}
	if registrationConflict != 0 {
		return fmt.Errorf(
			"%w: CIDR name or route conflicts with a registration",
			service.ErrConflict,
		)
	}

	_, err = tx.Exec(`
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
	if err != nil {
		return CheckSqliteErr("insert cidr", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit insert CIDR tx: %w", err)
	}
	return nil
}

func (db *DB) UpdateCidr(
	network string,
	name string,
	newName string,
) (
	*service.Cidr,
	error,
) {
	tx, err := db.Conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin update CIDR tx: %w", err)
	}
	defer tx.Rollback()

	var registrationConflict int
	err = tx.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM registration
			WHERE network_name = ?1
				AND confirmed = 0
				AND name = ?2
		)`,
		network,
		newName,
	).Scan(&registrationConflict)
	if err != nil {
		return nil, CheckSqliteErr("check CIDR rename registration reservation", err)
	}
	if registrationConflict != 0 {
		return nil, fmt.Errorf(
			"%w: CIDR name conflicts with a registration",
			service.ErrConflict,
		)
	}

	row := tx.QueryRow(`
		UPDATE cidr
		SET name = ?3
		WHERE network_name = ?1
			AND name = ?2
		RETURNING name, cidr, length, prefix, terminal`,
		network,
		name,
		newName,
	)

	cidr, err := scanCidr(row)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit update CIDR tx: %w", err)
	}
	return cidr, nil
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

func (db *DB) ListCidrGroups(
	network string,
	cidrName string,
) (
	[]*service.Group,
	error,
) {
	rows, err := db.Conn.Query(`
		SELECT g.id, g.name
		FROM cidr_assignment a
		JOIN cidr c ON c.id = a.cidr_id
		JOIN "group" g ON g.id = a.group_id
		WHERE c.network_name = ?1
			AND c.name = ?2
		ORDER BY g.name`,
		network,
		cidrName,
	)
	if err != nil {
		return nil, CheckSqliteErr("list CIDR groups", err)
	}
	defer rows.Close()

	var groups []*service.Group
	for rows.Next() {
		var group service.Group
		if err := rows.Scan(&group.ID, &group.Name); err != nil {
			return nil, CheckSqliteErr("scan CIDR group", err)
		}
		groups = append(groups, &group)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate CIDR groups: %w", err)
	}
	return groups, nil
}

func (db *DB) AssignCidrGroup(
	network string,
	cidrName string,
	groupName string,
) error {
	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin assign group tx: %w", err)
	}
	defer tx.Rollback()

	var cidrID int64
	var provisional int
	err = tx.QueryRow(`
		SELECT c.id, EXISTS (
			SELECT 1 FROM peer p
			WHERE p.cidr_id = c.id AND p.confirmed = 0
		)
		FROM cidr c
		WHERE c.network_name = ?1 AND c.name = ?2`,
		network,
		cidrName,
	).Scan(&cidrID, &provisional)
	if err != nil {
		return CheckSqliteErr("find CIDR for assignment", err)
	}
	if provisional != 0 {
		return fmt.Errorf(
			"%w: assign groups to an unconfirmed peer through its registration",
			service.ErrConflict,
		)
	}

	var groupID int64
	err = tx.QueryRow(`
		SELECT id FROM "group"
		WHERE network_name = ?1 AND name = ?2`,
		network,
		groupName,
	).Scan(&groupID)
	if err != nil {
		return CheckSqliteErr("find group for assignment", err)
	}

	if _, err := tx.Exec(`
		INSERT INTO cidr_assignment (cidr_id, group_id)
		VALUES (?1, ?2)`,
		cidrID,
		groupID,
	); err != nil {
		return CheckSqliteErr("assign group", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit assign group tx: %w", err)
	}
	return nil
}

func (db *DB) RemoveCidrGroup(
	network string,
	cidrName string,
	groupName string,
) error {
	_, err := db.Conn.Exec(`
		DELETE FROM cidr_assignment
		WHERE cidr_id = (SELECT id FROM cidr WHERE network_name = ?1 AND name = ?2)
			AND group_id = (SELECT id FROM "group" WHERE network_name = ?1 AND name = ?3)`,
		network,
		cidrName,
		groupName,
	)
	return CheckSqliteErr("remove group", err)
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
