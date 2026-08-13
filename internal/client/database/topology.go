package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/topology"
)

type topologyDocument struct {
	Nodes           []topologyNodeDocument        `json:"nodes"`
	Associations    []topologyAssociationDocument `json:"associations"`
	EffectiveGroups []string                      `json:"effective_groups"`
	SubjectPeer     string                        `json:"subject_peer"`
}

type topologyNodeDocument struct {
	Name          string   `json:"name"`
	CIDR          string   `json:"cidr"`
	Terminal      bool     `json:"terminal"`
	DisplayParent string   `json:"display_parent,omitempty"`
	Groups        []string `json:"groups"`
	PeerName      string   `json:"peer_name,omitempty"`
	Subject       bool     `json:"subject"`
}

type topologyAssociationDocument struct {
	Group1 string `json:"group1"`
	Group2 string `json:"group2"`
}

func (db *DB) ApplyNetworkReconciliation(
	network string,
	reconciliation service.NetworkReconciliation,
) error {
	publicKeys, err := validatePeerObservations(reconciliation.Peers)
	if err != nil {
		return err
	}

	viewJSON, err := encodeTopologyView(reconciliation.Topology)
	if err != nil {
		return fmt.Errorf("%w: invalid topology: %v", service.ErrInvalidInput, err)
	}

	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin apply network reconciliation tx: %w", err)
	}
	defer tx.Rollback()

	if err := sqlRequireNetworkTx(tx, network); err != nil {
		return err
	}
	if err := sqlApplyPeerReconciliationTx(
		tx,
		network,
		reconciliation.Peers,
		publicKeys,
		reconciliation.PruneBefore,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(`
		INSERT INTO topology (
			network_name,
			view_json,
			generated_at_unix,
			synced_at_unix
		)
		VALUES (?1, ?2, ?3, ?4)
		ON CONFLICT (network_name) DO UPDATE SET
			view_json = excluded.view_json,
			generated_at_unix = excluded.generated_at_unix,
			synced_at_unix = excluded.synced_at_unix`,
		network,
		string(viewJSON),
		reconciliation.GeneratedAt.Unix(),
		reconciliation.ReceivedAt.Unix(),
	); err != nil {
		return CheckSqliteErr("replace topology", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit apply network reconciliation tx: %w", err)
	}
	return nil
}

func (db *DB) GetNetworkTopology(
	network string,
) (
	*service.CachedTopology,
	error,
) {
	tx, err := db.Conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin get topology tx: %w", err)
	}
	defer tx.Rollback()

	if err := sqlRequireNetworkTx(tx, network); err != nil {
		return nil, err
	}
	var viewJSON string
	var generatedAtUnix, syncedAtUnix int64
	err = tx.QueryRow(`
		SELECT view_json, generated_at_unix, synced_at_unix
		FROM topology
		WHERE network_name = ?1`,
		network,
	).Scan(&viewJSON, &generatedAtUnix, &syncedAtUnix)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: network %q", service.ErrTopologyUnavailable, network)
	}
	if err != nil {
		return nil, CheckSqliteErr("get topology", err)
	}

	view, err := decodeTopologyView([]byte(viewJSON))
	if err != nil {
		return nil, fmt.Errorf("decode topology for network %q: %w", network, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit get topology tx: %w", err)
	}
	return &service.CachedTopology{
		View:        view,
		GeneratedAt: time.Unix(generatedAtUnix, 0),
		SyncedAt:    time.Unix(syncedAtUnix, 0),
	}, nil
}

func encodeTopologyView(
	view topology.View,
) (
	[]byte,
	error,
) {
	normalized, err := topology.NormalizeView(view)
	if err != nil {
		return nil, err
	}
	document := topologyDocument{
		Nodes:           make([]topologyNodeDocument, len(normalized.Nodes)),
		Associations:    make([]topologyAssociationDocument, len(normalized.Associations)),
		EffectiveGroups: normalized.EffectiveGroups,
		SubjectPeer:     normalized.SubjectPeer,
	}
	for i, node := range normalized.Nodes {
		document.Nodes[i] = topologyNodeDocument{
			Name:          node.Cidr.Name,
			CIDR:          node.Cidr.Cidr,
			Terminal:      node.Cidr.Terminal,
			DisplayParent: node.DisplayParent,
			Groups:        node.Groups,
			PeerName:      node.PeerName,
			Subject:       node.Subject,
		}
	}
	for i, association := range normalized.Associations {
		document.Associations[i] = topologyAssociationDocument{
			Group1: association.Group1,
			Group2: association.Group2,
		}
	}
	return json.Marshal(document)
}

func decodeTopologyView(
	data []byte,
) (
	topology.View,
	error,
) {
	var document topologyDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return topology.View{}, err
	}
	nodes := make([]topology.ViewNode, len(document.Nodes))
	for i, node := range document.Nodes {
		cidr, err := topology.CidrFromString(node.Name, node.CIDR, node.Terminal)
		if err != nil {
			return topology.View{}, err
		}
		nodes[i] = topology.ViewNode{
			Cidr:          cidr,
			DisplayParent: node.DisplayParent,
			Groups:        node.Groups,
			PeerName:      node.PeerName,
			Subject:       node.Subject,
		}
	}
	associations := make([]topology.Association, len(document.Associations))
	for i, association := range document.Associations {
		associations[i] = topology.Association{
			Group1: association.Group1,
			Group2: association.Group2,
		}
	}
	return topology.NormalizeView(topology.View{
		Nodes:           nodes,
		Associations:    associations,
		EffectiveGroups: document.EffectiveGroups,
		SubjectPeer:     document.SubjectPeer,
	})
}
