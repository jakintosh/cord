package database

import (
	"database/sql"
	"fmt"
	"net"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
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

func (db *DB) CreateCidr(
	network string,
	cidr *service.Cidr,
) error {
	_, ipNet, err := net.ParseCIDR(cidr.Cidr)
	if err != nil {
		return fmt.Errorf("insert cidr %q: parse cidr: %w", cidr.Name, err)
	}
	ones, bits := ipNet.Mask.Size()
	first, last := netaddr.Range(ipNet)

	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin insert CIDR tx: %w", err)
	}
	defer tx.Rollback()

	mainCIDR, err := sqlGetNetworkMainCidrTx(tx, network)
	if err != nil {
		return err
	}
	_, mainNet, err := net.ParseCIDR(mainCIDR)
	if err != nil {
		return fmt.Errorf("parse persisted main CIDR %q: %w", mainCIDR, err)
	}

	if !netaddr.Contains(mainNet, ipNet) {
		return fmt.Errorf(
			"%w: CIDR %q is not contained within main CIDR %q",
			service.ErrInvalidInput,
			cidr.Cidr,
			mainCIDR,
		)
	}

	if err := sqlCheckRegistrationReservationTx(tx, network, cidr.Name, ipNet.String()); err != nil {
		return err
	}
	if err := sqlInsertCidrTx(tx, network, cidr, bits, ones, first, last); err != nil {
		return err
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

	if err := sqlCheckRegistrationNameReservedTx(tx, network, newName); err != nil {
		return nil, err
	}
	cidr, err := sqlRenameCidrTx(tx, network, name, newName)
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
	if name == network {
		return fmt.Errorf("%w: cannot delete the root CIDR", service.ErrConflict)
	}

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
	tx, err := db.Conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin list CIDR groups tx: %w", err)
	}
	defer tx.Rollback()

	cidrID, err := sqlGetCidrIdTx(tx, network, cidrName, "find CIDR for group listing")
	if err != nil {
		return nil, err
	}
	groups, err := sqlListCidrGroupsTx(tx, cidrID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit list CIDR groups tx: %w", err)
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

	cidrID, provisional, err := sqlGetCidrAssignmentStateTx(tx, network, cidrName)
	if err != nil {
		return err
	}
	if provisional {
		return fmt.Errorf(
			"%w: assign groups to an unconfirmed peer through its registration",
			service.ErrConflict,
		)
	}

	groupID, err := sqlGetGroupIDTx(tx, network, groupName, "find group for assignment")
	if err != nil {
		return err
	}
	if err := sqlInsertCidrGroupAssignmentTx(tx, cidrID, groupID); err != nil {
		return err
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
	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin remove CIDR group tx: %w", err)
	}
	defer tx.Rollback()

	cidrID, err := sqlGetCidrIdTx(tx, network, cidrName, "find CIDR for group removal")
	if err != nil {
		return err
	}
	groupID, err := sqlGetGroupIDTx(tx, network, groupName, "find group for CIDR removal")
	if err != nil {
		return err
	}
	if err := sqlDeleteCidrGroupAssignmentTx(tx, cidrID, groupID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit remove CIDR group tx: %w", err)
	}
	return nil
}

func sqlGetCidrIdTx(
	tx *sql.Tx,
	network string,
	name string,
	context string,
) (
	int64,
	error,
) {
	var cidrID int64
	if err := tx.QueryRow(`
		SELECT id FROM cidr
		WHERE network_name = ?1 AND name = ?2`,
		network,
		name,
	).Scan(&cidrID); err != nil {
		return 0, CheckSqliteErr(context, err)
	}
	return cidrID, nil
}

func sqlGetCidrTx(
	tx *sql.Tx,
	cidrID int64,
) (
	string,
	string,
	error,
) {
	var cidrName string
	var cidrStr string
	if err := tx.QueryRow(`
		SELECT name, cidr
		FROM cidr
		WHERE id = ?1`,
		cidrID,
	).Scan(
		&cidrName,
		&cidrStr,
	); err != nil {
		return "", "", fmt.Errorf("lookup cidr after update: %w", err)
	}
	return cidrName, cidrStr, nil
}

func sqlInsertCidrTx(
	tx *sql.Tx,
	network string,
	cidr *service.Cidr,
	bits int,
	ones int,
	first net.IP,
	last net.IP,
) error {
	_, err := tx.Exec(`
		INSERT INTO cidr (
			network_name,
			name,
			cidr,
			length,
			prefix,
			base,
			last,
			terminal
		) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)`,
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

func sqlInsertRootCidrTx(
	tx *sql.Tx,
	network string,
	cidr *service.Cidr,
	bits int,
	ones int,
	first net.IP,
	last net.IP,
) error {
	if _, err := tx.Exec(`
		INSERT INTO cidr (network_name, name, cidr, length, prefix, base, last, terminal)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, 0)`,
		network,
		cidr.Name,
		cidr.Cidr,
		bits,
		ones,
		first,
		last,
	); err != nil {
		return CheckSqliteErr("insert root cidr", err)
	}
	return nil
}

func sqlInsertServerCidrTx(
	tx *sql.Tx,
	network string,
	cidr *service.Cidr,
	bits int,
	ones int,
	first net.IP,
	last net.IP,
) (
	int64,
	error,
) {
	result, err := tx.Exec(`
		INSERT INTO cidr (network_name, name, cidr, length, prefix, base, last, terminal)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, 1)`,
		network,
		cidr.Name,
		cidr.Cidr,
		bits,
		ones,
		first,
		last,
	)
	if err != nil {
		return 0, CheckSqliteErr("insert server cidr", err)
	}

	cidrID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get server cidr id: %w", err)
	}
	return cidrID, nil
}

func sqlInsertRedeemedCidrTx(
	tx *sql.Tx,
	network string,
	registration *registrationRedemption,
) (
	int64,
	error,
) {
	_, cidrNet, err := net.ParseCIDR(registration.mainRoute)
	if err != nil {
		return 0, fmt.Errorf("parse main route %q: %w", registration.mainRoute, err)
	}
	ones, bits := cidrNet.Mask.Size()
	first, last := netaddr.Range(cidrNet)

	result, err := tx.Exec(`
		INSERT INTO cidr (
			network_name,
			name,
			cidr,
			length,
			prefix,
			base,
			last,
			terminal
		)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, 1)`,
		network,
		registration.name,
		registration.mainRoute,
		bits,
		ones,
		first,
		last,
	)
	if err != nil {
		return 0, CheckSqliteErr("redeem create CIDR", err)
	}

	cidrID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get redeemed CIDR id: %w", err)
	}

	return cidrID, nil
}

func sqlRenameCidrTx(
	tx *sql.Tx,
	network string,
	name string,
	newName string,
) (
	*service.Cidr,
	error,
) {
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
	return scanCidr(row)
}

func sqlDeleteCidrTx(
	tx *sql.Tx,
	cidrID int64,
) error {
	_, err := tx.Exec(`
		DELETE FROM cidr
		WHERE id = ?1`,
		cidrID,
	)
	return CheckSqliteErr("delete cidr", err)
}
func sqlListCidrGroupsTx(
	tx *sql.Tx,
	cidrID int64,
) (
	[]*service.Group,
	error,
) {
	rows, err := tx.Query(`
		SELECT g.id, g.name
		FROM cidr_assignment a
		JOIN "group" g ON g.id = a.group_id
		WHERE a.cidr_id = ?1
		ORDER BY g.name`,
		cidrID,
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

func sqlGetCidrAssignmentStateTx(
	tx *sql.Tx,
	network string,
	name string,
) (
	int64,
	bool,
	error,
) {
	var cidrID int64
	var provisional int
	if err := tx.QueryRow(`
		SELECT c.id, EXISTS (
			SELECT 1 FROM peer p
			WHERE p.cidr_id = c.id AND p.confirmed = 0
		)
		FROM cidr c
		WHERE c.network_name = ?1 AND c.name = ?2`,
		network,
		name,
	).Scan(&cidrID, &provisional); err != nil {
		return 0, false, CheckSqliteErr("find CIDR for assignment", err)
	}
	return cidrID, provisional != 0, nil
}

func sqlInsertCidrGroupAssignmentTx(
	tx *sql.Tx,
	cidrID int64,
	groupID int64,
) error {
	_, err := tx.Exec(`
		INSERT INTO cidr_assignment (cidr_id, group_id)
		VALUES (?1, ?2)`,
		cidrID,
		groupID,
	)
	return CheckSqliteErr("assign group", err)
}

func sqlDeleteCidrGroupAssignmentTx(
	tx *sql.Tx,
	cidrID int64,
	groupID int64,
) error {
	_, err := tx.Exec(`
		DELETE FROM cidr_assignment
		WHERE cidr_id = ?1 AND group_id = ?2`,
		cidrID,
		groupID,
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

	if err := s.Scan(
		&name,
		&cidrStr,
		&length,
		&prefix,
		&terminal,
	); err != nil {
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
