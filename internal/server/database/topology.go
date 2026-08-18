package database

import (
	"context"
	"database/sql"
	"fmt"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/topology"
)

func (db *DB) LoadTopologyState(
	network string,
) (
	*service.TopologyState,
	error,
) {
	opts := &sql.TxOptions{
		ReadOnly: true,
	}
	tx, err := db.Conn.BeginTx(context.Background(), opts)
	if err != nil {
		return nil, fmt.Errorf("begin read tx: %w", err)
	}
	defer tx.Rollback()

	if err := sqlRequireNetworkTx(tx, network); err != nil {
		return nil, err
	}

	cidrs, err := db.sqlLoadCidrsTx(tx, network)
	if err != nil {
		return nil, err
	}

	assignments, err := db.sqlLoadAssignmentsTx(tx, network)
	if err != nil {
		return nil, err
	}

	associations, err := db.sqlLoadAssociationsTx(tx, network)
	if err != nil {
		return nil, err
	}

	peers, err := db.sqlLoadTopologyPeersTx(tx, network)
	if err != nil {
		return nil, err
	}

	state := &service.TopologyState{
		Cidrs:        cidrs,
		Assignments:  assignments,
		Associations: associations,
		Peers:        peers,
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit topology state read: %w", err)
	}

	return state, nil
}

func (db *DB) sqlLoadCidrsTx(
	tx *sql.Tx,
	network string,
) (
	[]topology.Cidr,
	error,
) {
	var cidrs []topology.Cidr
	rows, err := tx.Query(`
		SELECT name, cidr, base, last, prefix, length, terminal
		FROM cidr
		WHERE network_name = ?1`,
		network,
	)
	if err != nil {
		return nil, fmt.Errorf("load cidrs: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name, cidr string
		var base, last []byte
		var prefix, bits int
		var terminal int64
		if err := rows.Scan(
			&name,
			&cidr,
			&base,
			&last,
			&prefix,
			&bits,
			&terminal,
		); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan cidr: %w", err)
		}
		cidrs = append(cidrs, topology.Cidr{
			Name:     name,
			Cidr:     cidr,
			Base:     base,
			Last:     last,
			Prefix:   prefix,
			Bits:     bits,
			Terminal: terminal != 0,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cidrs: %w", err)
	}

	return cidrs, nil
}

func (db *DB) sqlLoadAssignmentsTx(
	tx *sql.Tx,
	network string,
) (
	map[string][]string,
	error,
) {
	assignments := make(map[string][]string)
	rows, err := tx.Query(`
		SELECT c.name, g.name
		FROM cidr_assignment a
		JOIN cidr c ON c.id = a.cidr_id
		JOIN "group" g ON g.id = a.group_id
		WHERE c.network_name = ?1`,
		network,
	)
	if err != nil {
		return nil, fmt.Errorf("load assignments: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cidrName, groupName string
		if err := rows.Scan(&cidrName, &groupName); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan assignment: %w", err)
		}
		assignments[cidrName] = append(assignments[cidrName], groupName)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate assignments: %w", err)
	}

	return assignments, nil
}

func (db *DB) sqlLoadAssociationsTx(
	tx *sql.Tx,
	network string,
) (
	map[string]map[string]bool,
	error,
) {
	associations := make(map[string]map[string]bool)
	rows, err := tx.Query(`
		SELECT g1.name, g2.name
		FROM association a
		JOIN "group" g1 ON g1.id = a.group1_id
		JOIN "group" g2 ON g2.id = a.group2_id
		WHERE a.network_name = ?1`,
		network,
	)
	if err != nil {
		return nil, fmt.Errorf("load associations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var g1, g2 string
		if err := rows.Scan(&g1, &g2); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan association: %w", err)
		}
		if associations[g1] == nil {
			associations[g1] = make(map[string]bool)
		}
		if associations[g2] == nil {
			associations[g2] = make(map[string]bool)
		}
		associations[g1][g2] = true
		associations[g2][g1] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate associations: %w", err)
	}

	return associations, nil
}

func (db *DB) sqlLoadTopologyPeersTx(
	tx *sql.Tx,
	network string,
) (
	[]*service.Peer,
	error,
) {
	rows, err := tx.Query(`
		SELECT
			p.name,
			p.public_key,
			c.name,
			c.cidr,
			p.admin,
			p.enabled,
			p.confirmed
		FROM peer p
		JOIN cidr c ON c.id = p.cidr_id
		WHERE p.network_name = ?1
		ORDER BY p.name ASC`,
		network,
	)
	if err != nil {
		return nil, fmt.Errorf("load topology peers: %w", err)
	}
	defer rows.Close()

	var peers []*service.Peer
	for rows.Next() {
		peer, err := scanPeer(rows)
		if err != nil {
			return nil, fmt.Errorf("scan topology peer: %w", err)
		}
		peers = append(peers, peer)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate topology peers: %w", err)
	}

	return peers, nil
}
