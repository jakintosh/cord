package database

import (
	"fmt"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func (db *DB) ListGroups(
	network string,
) (
	[]*service.Group,
	error,
) {
	rows, err := db.Conn.Query(`
		SELECT id, name
		FROM "group"
		WHERE network_name = ?1
		ORDER BY name ASC`,
		network,
	)
	if err != nil {
		return nil, CheckSqliteErr("list groups", err)
	}
	defer rows.Close()

	var groups []*service.Group
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, CheckSqliteErr("scan group", err)
		}
		groups = append(groups, &service.Group{ID: id, Name: name})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate groups: %w", err)
	}

	return groups, nil
}

func (db *DB) InsertGroup(
	network string,
	name string,
) (
	*service.Group,
	error,
) {
	result, err := db.Conn.Exec(`
		INSERT INTO "group" (network_name, name)
		VALUES (?1, ?2)`,
		network,
		name,
	)
	if err != nil {
		return nil, CheckSqliteErr("insert group", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get group id: %w", err)
	}

	return &service.Group{ID: id, Name: name}, nil
}

func (db *DB) DeleteGroup(
	network string,
	name string,
) error {
	result, err := db.Conn.Exec(`
		DELETE FROM "group"
		WHERE network_name = ?1
			AND name = ?2`,
		network,
		name,
	)
	if err != nil {
		return CheckSqliteErr("delete group", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete group rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: group %q not found", service.ErrNotFound, name)
	}
	return nil
}
