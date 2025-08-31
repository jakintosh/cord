package database

import "git.sr.ht/~jakintosh/cord/internal/server"

func (store *SQLiteStore) AssociationList() (
	[]*server.Association,
	error,
) {
	panic("unimplemented")
}

func (store *SQLiteStore) AssociationCreate(
	a string,
	b string,
) error {
	_, err := store.db.Exec(`
		INSERT INTO association (cidr1, cidr2)
		SELECT c1.id, c2.id
		FROM cidr c1, cidr c2
		WHERE c1.name=? AND c2.name=?;
		`,
		a,
		b,
	)
	return CheckSqliteErr("adding association", err)
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
		WHERE a.cidr1=?1 OR a.cidr2<>?1;
		`,
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
		);
		`,
		a,
		b,
	)
	return CheckSqliteErr("deleting association", err)
}
