package database

import (
	"fmt"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func (db *DB) ListAssociations(
	network string,
) (
	[]*service.Association,
	error,
) {
	rows, err := db.Conn.Query(`
		SELECT c1.name, c2.name
		FROM association a
		JOIN cidr c1 ON c1.id = a.cidr1
		JOIN cidr c2 ON c2.id = a.cidr2
		WHERE a.network_name = ?1
		ORDER BY c1.name, c2.name`,
		network,
	)
	if err != nil {
		return nil, CheckSqliteErr("list associations", err)
	}
	defer rows.Close()

	var associations []*service.Association
	for rows.Next() {
		var cidr1, cidr2 string
		if err := rows.Scan(&cidr1, &cidr2); err != nil {
			return nil, CheckSqliteErr("scan association", err)
		}
		associations = append(associations, &service.Association{
			Cidr1: cidr1,
			Cidr2: cidr2,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate associations: %w", err)
	}

	return associations, nil
}

func (db *DB) InsertAssociation(
	network string,
	a *service.Association,
) error {
	result, err := db.Conn.Exec(`
		INSERT INTO association (network_name, cidr1, cidr2)
		SELECT
			?1,
			MIN(c1.id, c2.id),
			MAX(c1.id, c2.id)
		FROM cidr c1, cidr c2
		WHERE c1.network_name = ?1
			AND c2.network_name = ?1
			AND c1.name = ?2
			AND c2.name = ?3`,
		network,
		a.Cidr1,
		a.Cidr2,
	)

	if err != nil {
		return CheckSqliteErr("insert association", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("insert association rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: unknown cidr names or association already exists", service.ErrConflict)
	}

	return nil
}

func (db *DB) DeleteAssociation(
	network string,
	cidr1 string,
	cidr2 string,
) error {
	_, err := db.Conn.Exec(`
		DELETE FROM association
		WHERE network_name = ?1
			AND id IN (
				SELECT a.id
				FROM association a
				JOIN cidr c1 ON c1.id = a.cidr1
				JOIN cidr c2 ON c2.id = a.cidr2
				WHERE (c1.name = ?2 AND c2.name = ?3)
					OR (c1.name = ?3 AND c2.name = ?2)
			)`,
		network,
		cidr1,
		cidr2,
	)
	return CheckSqliteErr("delete association", err)
}
