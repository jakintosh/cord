package database

import (
	"fmt"
	"git.sr.ht/~jakintosh/cord/internal/server"
)

func (store *SQLiteStore) AssociationList() (
	[]*server.Association,
	error,
) {
	rows, err := store.db.Query(`
		SELECT c1.name, c2.name
		FROM association a
		JOIN cidr c1 ON c1.id = a.cidr1
		JOIN cidr c2 ON c2.id = a.cidr2
		ORDER BY c1.name, c2.name;`,
	)
	if err != nil {
		return nil, CheckSqliteErr("querying associations", err)
	}
	defer rows.Close()

	var associations []*server.Association
	for rows.Next() {
		var cidr1, cidr2 string
		err := rows.Scan(&cidr1, &cidr2)
		if err != nil {
			return nil, CheckSqliteErr("scanning association", err)
		}
		associations = append(associations, &server.Association{
			Cidr1: cidr1,
			Cidr2: cidr2,
		})
	}

	return associations, nil
}

func (store *SQLiteStore) AssociationCreate(
	a string,
	b string,
) error {
	result, err := store.db.Exec(`
		INSERT INTO association (cidr1, cidr2)
		SELECT
			MIN(c1.id, c2.id),
			MAX(c1.id, c2.id)
		FROM cidr c1, cidr c2
		WHERE c1.name = ?1 AND c2.name = ?2;
		`,
		a,
		b,
	)

	if err != nil {
		return CheckSqliteErr("adding association", err)
	}

	if ResultsEmpty(result) {
		return fmt.Errorf("%w: unknown cidr names or association already exists", server.ErrConflict)
	}

	return nil
}

func (store *SQLiteStore) AssociationDelete(
	a string,
	b string,
) error {
	_, err := store.db.Exec(`
		DELETE FROM association
		WHERE id in (
			SELECT a.id
			FROM association a
			JOIN cidr c1 ON c1.id=a.cidr1
			JOIN cidr c2 ON c2.id=a.cidr2
			WHERE (c1.name=?1 AND c2.name=?2)
			OR (c1.name=?2 AND c2.name=?1)
		);`,
		a,
		b,
	)
	return CheckSqliteErr("deleting association", err)
}
