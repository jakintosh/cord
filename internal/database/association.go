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
		return fmt.Errorf("invalid CIDR names or association already exists")
	}

	return nil
}

func (store *SQLiteStore) AssociationListAssociatedCidrIds(
	baseCidrId int64,
) (
	[]int64,
	error,
) {
	// query the cidrs associated with the base cidr
	rows, err := store.db.Query(`
		SELECT DISTINCT COALESCE(c1.id, c2.id) as cidr
		FROM association a
		LEFT JOIN (
			SELECT * FROM cidr c
			WHERE c.id<>?1
		) AS c1 ON a.cidr1=c1.id
		LEFT JOIN (
			SELECT * FROM cidr c
			WHERE c.id<>?1
		) AS c2 ON a.cidr2=c2.id
		WHERE a.cidr1=?1 OR a.cidr2<>?1;`,
		baseCidrId,
	)
	if err != nil {
		return nil, CheckSqliteErr("getting associated cidrs", err)
	}

	// scan the resulting list of cidr id ints
	defer rows.Close()
	var id int64
	var cidrIds []int64
	for rows.Next() {
		if err := rows.Scan(&id); err != nil {
			return nil, CheckSqliteErr("scanning cidr id", err)
		}
		cidrIds = append(cidrIds, id)
	}
	return cidrIds, nil
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
