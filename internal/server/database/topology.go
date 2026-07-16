package database

import (
	"context"
	"database/sql"
	"fmt"

	"git.studiopollinator.com/pollinator/cord/internal/topology"
)

func (db *DB) LoadTopologySnapshot(
	network string,
) (
	*topology.Snapshot,
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

	peerCidr, peerInfo, err := db.sqlLoadPeersTx(tx, network)
	if err != nil {
		return nil, err
	}

	return &topology.Snapshot{
		Cidrs:        cidrs,
		Assignments:  assignments,
		Associations: associations,
		PeerCidr:     peerCidr,
		PeerInfo:     peerInfo,
	}, nil
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
	rows.Close()
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
		FROM assignment a
		JOIN cidr c ON c.id = a.cidr_id
		JOIN "group" g ON g.id = a.group_id
		WHERE c.network_name = ?1`,
		network,
	)
	if err != nil {
		return nil, fmt.Errorf("load assignments: %w", err)
	}
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
	rows.Close()
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
	rows.Close()
	return associations, nil
}

func (db *DB) sqlLoadPeersTx(
	tx *sql.Tx,
	network string,
) (
	map[string]string,
	map[string]topology.Peer,
	error,
) {
	peerCidr := make(map[string]string)
	peerInfo := make(map[string]topology.Peer)
	rows, err := tx.Query(`
		SELECT p.name, p.public_key, c.name, c.cidr
		FROM peer p
		JOIN cidr c ON c.id = p.cidr_id
		WHERE p.network_name = ?1`,
		network,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("load peers: %w", err)
	}
	for rows.Next() {
		var peerName, pubKey, cidrName, cidrVal string
		if err := rows.Scan(
			&peerName,
			&pubKey,
			&cidrName,
			&cidrVal,
		); err != nil {
			rows.Close()
			return nil, nil, fmt.Errorf("scan peer: %w", err)
		}
		peerCidr[peerName] = cidrName
		peerInfo[peerName] = topology.Peer{
			Name:      peerName,
			PublicKey: pubKey,
			Route:     cidrVal,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate peers: %w", err)
	}
	rows.Close()
	return peerCidr, peerInfo, nil
}
