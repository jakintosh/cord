package database

import (
	"fmt"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func (db *DB) ListAssignments(
	network string,
) (
	[]*service.Assignment,
	error,
) {
	rows, err := db.Conn.Query(`
		SELECT c.name, g.name
		FROM assignment a
		JOIN cidr c ON c.id = a.cidr_id
		JOIN "group" g ON g.id = a.group_id
		WHERE c.network_name = ?1
		ORDER BY c.name, g.name`,
		network,
	)
	if err != nil {
		return nil, CheckSqliteErr("list assignments", err)
	}
	defer rows.Close()

	var assignments []*service.Assignment
	for rows.Next() {
		var cidrName, groupName string
		if err := rows.Scan(&cidrName, &groupName); err != nil {
			return nil, CheckSqliteErr("scan assignment", err)
		}
		assignments = append(assignments, &service.Assignment{
			CidrName:  cidrName,
			GroupName: groupName,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate assignments: %w", err)
	}

	return assignments, nil
}

func (db *DB) AssignGroup(
	network string,
	cidrName string,
	groupName string,
) error {
	_, err := db.Conn.Exec(`
		INSERT INTO assignment (cidr_id, group_id)
		SELECT c.id, g.id
		FROM cidr c, "group" g
		WHERE c.network_name = ?1
			AND c.name = ?2
			AND g.network_name = ?1
			AND g.name = ?3`,
		network,
		cidrName,
		groupName,
	)
	return CheckSqliteErr("assign group", err)
}

func (db *DB) RemoveGroup(
	network string,
	cidrName string,
	groupName string,
) error {
	_, err := db.Conn.Exec(`
		DELETE FROM assignment
		WHERE cidr_id = (SELECT id FROM cidr WHERE network_name = ?1 AND name = ?2)
			AND group_id = (SELECT id FROM "group" WHERE network_name = ?1 AND name = ?3)`,
		network,
		cidrName,
		groupName,
	)
	return CheckSqliteErr("remove group", err)
}
