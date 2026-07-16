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
		SELECT g1.name, g2.name
		FROM association a
		JOIN "group" g1 ON g1.id = a.group1_id
		JOIN "group" g2 ON g2.id = a.group2_id
		WHERE a.network_name = ?1
		ORDER BY g1.name, g2.name`,
		network,
	)
	if err != nil {
		return nil, CheckSqliteErr("list associations", err)
	}
	defer rows.Close()

	var associations []*service.Association
	for rows.Next() {
		var group1, group2 string
		if err := rows.Scan(&group1, &group2); err != nil {
			return nil, CheckSqliteErr("scan association", err)
		}
		associations = append(associations, &service.Association{
			Group1: group1,
			Group2: group2,
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
		INSERT INTO association (network_name, group1_id, group2_id)
		SELECT
			?1,
			MIN(g1.id, g2.id),
			MAX(g1.id, g2.id)
		FROM "group" g1, "group" g2
		WHERE g1.network_name = ?1
			AND g2.network_name = ?1
			AND g1.name = ?2
			AND g2.name = ?3`,
		network,
		a.Group1,
		a.Group2,
	)

	if err != nil {
		return CheckSqliteErr("insert association", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("insert association rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: unknown group names or association already exists", service.ErrConflict)
	}

	return nil
}

func (db *DB) DeleteAssociation(
	network string,
	group1 string,
	group2 string,
) error {
	_, err := db.Conn.Exec(`
		DELETE FROM association
		WHERE network_name = ?1
			AND id IN (
				SELECT a.id
				FROM association a
				JOIN "group" g1 ON g1.id = a.group1_id
				JOIN "group" g2 ON g2.id = a.group2_id
				WHERE (g1.name = ?2 AND g2.name = ?3)
					OR (g1.name = ?3 AND g2.name = ?2)
			)`,
		network,
		group1,
		group2,
	)
	return CheckSqliteErr("delete association", err)
}
