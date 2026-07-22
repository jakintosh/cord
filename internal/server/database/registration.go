package database

import (
	"database/sql"
	"fmt"
	"net"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func (db *DB) GetRegistration(
	network string,
	name string,
) (
	*service.Registration,
	error,
) {
	row := db.Conn.QueryRow(`
		SELECT
			r.name,
			r.temp_public_key,
			r.temp_route,
			r.final_route,
			r.admin,
			r.redeemed,
			r.redeemed_key,
			r.confirmed,
			r.expires_at_unix,
			r.created_at_unix
		FROM registration r
		WHERE r.network_name = ?1
			AND r.name = ?2`,
		network,
		name,
	)

	return scanRegistration(row)
}

func (db *DB) GetRegistrationByIP(
	network string,
	ip net.IP,
	now time.Time,
) (
	*service.Registration,
	error,
) {
	route := netaddr.HostRoute(ip)
	row := db.Conn.QueryRow(`
		SELECT
			r.name,
			r.temp_public_key,
			r.temp_route,
			r.final_route,
			r.admin,
			r.redeemed,
			r.redeemed_key,
			r.confirmed,
			r.expires_at_unix,
			r.created_at_unix
		FROM registration r
		WHERE r.network_name = ?1
			AND r.temp_route = ?2
			AND r.confirmed = 0
			AND r.expires_at_unix > ?3`,
		network,
		route.String(),
		now.Unix(),
	)

	return scanRegistration(row)
}

func (db *DB) ListRegistrations(
	network string,
) (
	[]*service.Registration,
	error,
) {
	rows, err := db.Conn.Query(`
		SELECT
			r.name,
			r.temp_public_key,
			r.temp_route,
			r.final_route,
			r.admin,
			r.redeemed,
			r.redeemed_key,
			r.confirmed,
			r.expires_at_unix,
			r.created_at_unix
		FROM registration r
		WHERE r.network_name = ?1
		ORDER BY r.created_at_unix DESC`,
		network,
	)
	if err != nil {
		return nil, CheckSqliteErr("list registrations", err)
	}
	defer rows.Close()

	var regs []*service.Registration
	for rows.Next() {
		reg, err := scanRegistration(rows)
		if err != nil {
			return nil, err
		}
		regs = append(regs, reg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate registrations: %w", err)
	}

	return regs, nil
}

func (db *DB) ListActiveRegistrations(
	network string,
	now time.Time,
) (
	[]*service.Registration,
	error,
) {
	rows, err := db.Conn.Query(`
		SELECT
			r.name,
			r.temp_public_key,
			r.temp_route,
			r.final_route,
			r.admin,
			r.redeemed,
			r.redeemed_key,
			r.confirmed,
			r.expires_at_unix,
			r.created_at_unix
		FROM registration r
		WHERE r.network_name = ?1
			AND r.confirmed = 0
			AND r.expires_at_unix > ?2
		ORDER BY r.created_at_unix DESC`,
		network,
		now.Unix(),
	)
	if err != nil {
		return nil, CheckSqliteErr("list active registrations", err)
	}
	defer rows.Close()

	var regs []*service.Registration
	for rows.Next() {
		reg, err := scanRegistration(rows)
		if err != nil {
			return nil, err
		}
		regs = append(regs, reg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active registrations: %w", err)
	}

	return regs, nil
}

func (db *DB) CreateRegistration(
	network string,
	params service.CreateRegistrationParams,
	now time.Time,
) (
	*service.Registration,
	error,
) {
	tx, err := db.Conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin create registration tx: %w", err)
	}
	defer tx.Rollback()

	if err := sqlPruneExpiredRegistrationsTx(tx, network, now); err != nil {
		return nil, err
	}

	inviteCIDR, err := sqlGetNetworkInviteCidrTx(tx, network)
	if err != nil {
		return nil, err
	}
	if err := sqlValidateRegistrationMainRouteTx(tx, network, params.MainRoute); err != nil {
		return nil, err
	}

	if err := sqlCheckRegistrationConflictsTx(tx, network, params); err != nil {
		return nil, err
	}

	reservedRoutes, err := sqlListReservedRegistrationRoutesTx(tx, network, now)
	if err != nil {
		return nil, err
	}

	inviteRoute, err := nextFreeRegistrationRoute(inviteCIDR, reservedRoutes)
	if err != nil {
		return nil, err
	}

	registration := &service.Registration{
		Name:            params.Name,
		InvitePublicKey: params.InvitePublicKey,
		InviteRoute:     inviteRoute,
		MainRoute:       params.MainRoute,
		Admin:           params.Admin,
		ExpiresAt:       params.ExpiresAt,
		CreatedAt:       params.CreatedAt,
	}
	if err := sqlInsertRegistrationTx(tx, network, registration); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit create registration tx: %w", err)
	}

	return registration, nil
}

func (db *DB) RedeemRegistration(
	network string,
	tempPubKey string,
	permPubKey string,
	now time.Time,
) (
	*service.Peer,
	error,
) {
	tx, err := db.Conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin redeem tx: %w", err)
	}
	defer tx.Rollback()

	registration, err := sqlGetRegistrationForRedemptionTx(
		tx,
		network,
		tempPubKey,
	)
	if err != nil {
		return nil, err
	}

	if registration.redeemed {
		if registration.redeemedKey != permPubKey {
			return nil, fmt.Errorf(
				"%w: registration %q was redeemed with another key",
				service.ErrRegistrationRedeemed,
				registration.name,
			)
		}
	} else {
		if registration.confirmed {
			return nil, fmt.Errorf(
				"%w: registration %q is already confirmed",
				service.ErrRegistrationRedeemed,
				registration.name,
			)
		}
		if registration.expiresAtUnix <= now.Unix() {
			return nil, fmt.Errorf(
				"%w: registration %q",
				service.ErrRegistrationExpired,
				registration.name,
			)
		}

		cidrID, err := sqlInsertRedeemedCidrTx(tx, network, registration)
		if err != nil {
			return nil, err
		}

		if err := sqlInsertRedeemedPeerTx(
			tx,
			network,
			registration,
			cidrID,
			permPubKey,
		); err != nil {
			return nil, err
		}

		if err := sqlMarkRegistrationRedeemedTx(
			tx,
			registration.id,
			permPubKey,
		); err != nil {
			return nil, err
		}
	}

	peer, err := sqlGetRedeemedPeerTx(tx, registration.id, permPubKey)
	if err != nil {
		return nil, err
	}

	if registration.redeemed && peer.Confirmed {
		return nil, fmt.Errorf(
			"%w: registration %q is already confirmed",
			service.ErrRegistrationRedeemed,
			registration.name,
		)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit redeem tx: %w", err)
	}

	return peer, nil
}

func (db *DB) DeleteRegistration(
	network string,
	name string,
) error {
	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin delete registration tx: %w", err)
	}
	defer tx.Rollback()

	registration, err := sqlGetRegistrationForDeletionTx(tx, network, name)
	if err != nil {
		return err
	}
	if registration.confirmed {
		return fmt.Errorf(
			"%w: confirmed registration %q cannot be revoked",
			service.ErrConflict,
			name,
		)
	}

	var cidrID int64
	if registration.redeemedKey != "" {
		var found bool
		cidrID, found, err = sqlLookupProvisionalPeerCidrTx(tx, network, registration.redeemedKey)
		if err != nil {
			return err
		}
		if found {
			if err := sqlDeleteRevokedPeerTx(tx, network, registration.redeemedKey); err != nil {
				return err
			}
		}
	}

	if err := sqlDeleteRegistrationTx(tx, registration.id); err != nil {
		return err
	}

	if cidrID != 0 {
		if err := sqlDeleteCidrTx(tx, cidrID); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete registration tx: %w", err)
	}

	return nil
}

func (db *DB) ConfirmPeer(
	network string,
	name string,
	now time.Time,
) error {
	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin confirm peer tx: %w", err)
	}
	defer tx.Rollback()

	peer, err := sqlGetPeerForConfirmationTx(tx, network, name)
	if err != nil {
		return err
	}
	if peer.confirmed {
		return nil
	}

	registration, err := sqlGetRegistrationByRedeemedKeyTx(tx, network, peer.publicKey)
	if err != nil {
		return err
	}

	if registration.expiresAtUnix <= now.Unix() {
		return fmt.Errorf(
			"%w: registration for peer %q expired",
			service.ErrRegistrationExpired,
			name,
		)
	}

	if err := sqlTransferRegistrationGroupsTx(tx, peer.cidrID, registration.id); err != nil {
		return err
	}

	if err := sqlClearRegistrationGroupsTx(tx, registration.id); err != nil {
		return err
	}

	if err := sqlMarkPeerConfirmedTx(tx, peer.id); err != nil {
		return err
	}

	if err := sqlMarkRegistrationConfirmedTx(tx, registration.id); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit confirm peer tx: %w", err)
	}

	return nil
}

// PruneExpiredRegistrations removes expired unconfirmed registrations
// and any provisional peer rows that no longer have a live
// registration. A peer is provisional when confirmed = 0; it is kept
// only while its registration is unconfirmed and unexpired. Confirmed
// peers are never pruned here — their registrations are retained as
// audit state.
//
// Endpoint rows referencing pruned peers are removed via the ON DELETE
// CASCADE foreign keys on the endpoint table.
func (db *DB) PruneExpiredRegistrations(
	network string,
	now time.Time,
) error {
	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin prune tx: %w", err)
	}
	defer tx.Rollback()

	if err := sqlPruneExpiredRegistrationsTx(tx, network, now); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit prune tx: %w", err)
	}

	return nil
}

func (db *DB) ListRegistrationGroups(
	network string,
	registration string,
) (
	[]*service.Group,
	error,
) {
	tx, err := db.Conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin list registration groups tx: %w", err)
	}
	defer tx.Rollback()

	registrationID, err := sqlGetRegistrationIDTx(tx, network, registration)
	if err != nil {
		return nil, err
	}

	groups, err := sqlListRegistrationGroupsTx(tx, registrationID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit list registration groups tx: %w", err)
	}

	return groups, nil
}

func (db *DB) AssignRegistrationGroup(
	network string,
	registration string,
	group string,
	now time.Time,
) error {
	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin assign registration group tx: %w", err)
	}
	defer tx.Rollback()

	registrationID, err := sqlLookupMutableRegistrationTx(tx, network, registration, now)
	if err != nil {
		return err
	}

	groupID, err := sqlGetGroupIDTx(tx, network, group, "find group for registration assignment")
	if err != nil {
		return err
	}

	if err := sqlInsertRegistrationGroupTx(tx, registrationID, groupID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit assign registration group tx: %w", err)
	}

	return nil
}

func (db *DB) RemoveRegistrationGroup(
	network string,
	registration string,
	group string,
	now time.Time,
) error {
	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin remove registration group tx: %w", err)
	}
	defer tx.Rollback()

	registrationID, err := sqlLookupMutableRegistrationTx(tx, network, registration, now)
	if err != nil {
		return err
	}

	groupID, err := sqlGetGroupIDTx(tx, network, group, "find group for registration removal")
	if err != nil {
		return err
	}
	if err := sqlDeleteRegistrationGroupTx(tx, registrationID, groupID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit remove registration group tx: %w", err)
	}
	return nil
}

type registrationRedemption struct {
	id            int64
	name          string
	mainRoute     string
	admin         bool
	redeemed      bool
	redeemedKey   string
	confirmed     bool
	expiresAtUnix int64
}

type registrationConfirmationState struct {
	id            int64
	expiresAtUnix int64
}

type registrationDeletionState struct {
	id          int64
	redeemedKey string
	confirmed   bool
}

func sqlInsertRegistrationGroupTx(
	tx *sql.Tx,
	registrationID int64,
	groupID int64,
) error {
	_, err := tx.Exec(`
		INSERT INTO registration_assignment (registration_id, group_id)
		VALUES (?1, ?2)`,
		registrationID,
		groupID,
	)
	return CheckSqliteErr("assign registration group", err)
}

func sqlGetRegistrationIDTx(
	tx *sql.Tx,
	network string,
	name string,
) (
	int64,
	error,
) {
	var registrationID int64
	if err := tx.QueryRow(`
		SELECT id FROM registration
		WHERE network_name = ?1 AND name = ?2`,
		network,
		name,
	).Scan(&registrationID); err != nil {
		return 0, CheckSqliteErr("find registration for group list", err)
	}
	return registrationID, nil
}

func sqlListRegistrationGroupsTx(
	tx *sql.Tx,
	registrationID int64,
) (
	[]*service.Group,
	error,
) {
	rows, err := tx.Query(`
		SELECT g.name
		FROM registration_assignment a
		JOIN "group" g ON g.id = a.group_id
		WHERE a.registration_id = ?1
		ORDER BY g.name`,
		registrationID,
	)
	if err != nil {
		return nil, CheckSqliteErr("list registration groups", err)
	}
	defer rows.Close()

	var groups []*service.Group
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, CheckSqliteErr("scan registration group", err)
		}
		groups = append(groups, &service.Group{Name: name})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate registration groups: %w", err)
	}

	return groups, nil
}

func sqlDeleteRegistrationGroupTx(
	tx *sql.Tx,
	registrationID int64,
	groupID int64,
) error {
	_, err := tx.Exec(`
		DELETE FROM registration_assignment
		WHERE registration_id = ?1 AND group_id = ?2`,
		registrationID,
		groupID,
	)
	return CheckSqliteErr("remove registration group", err)
}

func sqlGetRegistrationForDeletionTx(
	tx *sql.Tx,
	network string,
	name string,
) (
	registrationDeletionState,
	error,
) {
	var state registrationDeletionState
	var confirmed int64
	if err := tx.QueryRow(`
		SELECT id, redeemed_key, confirmed
		FROM registration
		WHERE network_name = ?1 AND name = ?2`,
		network,
		name,
	).Scan(
		&state.id,
		&state.redeemedKey,
		&confirmed,
	); err != nil {
		return registrationDeletionState{}, CheckSqliteErr("find registration to delete", err)
	}
	state.confirmed = confirmed != 0
	return state, nil
}

func sqlDeleteRegistrationTx(
	tx *sql.Tx,
	registrationID int64,
) error {
	_, err := tx.Exec(`
		DELETE FROM registration
		WHERE id = ?1`,
		registrationID,
	)
	return CheckSqliteErr("delete registration", err)
}

func sqlGetRegistrationByRedeemedKeyTx(
	tx *sql.Tx,
	network string,
	publicKey string,
) (
	registrationConfirmationState,
	error,
) {
	var state registrationConfirmationState
	if err := tx.QueryRow(`
		SELECT id, expires_at_unix FROM registration
		WHERE network_name = ?1
			AND redeemed_key = ?2
			AND redeemed = 1
			AND confirmed = 0`,
		network,
		publicKey,
	).Scan(
		&state.id,
		&state.expiresAtUnix,
	); err != nil {
		return registrationConfirmationState{}, CheckSqliteErr("find registration to confirm", err)
	}
	return state, nil
}

func sqlTransferRegistrationGroupsTx(
	tx *sql.Tx,
	cidrID int64,
	registrationID int64,
) error {
	_, err := tx.Exec(`
		INSERT OR IGNORE INTO cidr_assignment (cidr_id, group_id)
		SELECT ?1, group_id
		FROM registration_assignment
		WHERE registration_id = ?2`,
		cidrID,
		registrationID,
	)
	return CheckSqliteErr("transfer registration groups", err)
}

func sqlClearRegistrationGroupsTx(
	tx *sql.Tx,
	registrationID int64,
) error {
	_, err := tx.Exec(`
		DELETE FROM registration_assignment
		WHERE registration_id = ?1`,
		registrationID,
	)
	return CheckSqliteErr("clear registration groups", err)
}

func sqlMarkRegistrationConfirmedTx(
	tx *sql.Tx,
	registrationID int64,
) error {
	_, err := tx.Exec(`
		UPDATE registration
		SET confirmed = 1
		WHERE id = ?1`,
		registrationID,
	)
	return CheckSqliteErr("confirm registration", err)
}

func sqlPruneExpiredRegistrationsTx(
	tx *sql.Tx,
	network string,
	now time.Time,
) error {
	cidrIDs, err := sqlListPrunablePeerCidrIDsTx(tx, network, now)
	if err != nil {
		return err
	}

	if err := sqlDeletePrunablePeersTx(tx, network, now); err != nil {
		return err
	}

	if err := sqlDeleteExpiredRegistrationsTx(tx, network, now); err != nil {
		return err
	}

	for _, cidrID := range cidrIDs {
		if err := sqlDeleteCidrTx(tx, cidrID); err != nil {
			return err
		}
	}

	return nil
}

func sqlDeleteExpiredRegistrationsTx(
	tx *sql.Tx,
	network string,
	now time.Time,
) error {
	_, err := tx.Exec(`
		DELETE FROM registration
		WHERE network_name = ?1
			AND confirmed = 0
			AND expires_at_unix <= ?2`,
		network,
		now.Unix(),
	)
	return CheckSqliteErr("prune expired registrations", err)
}

func sqlCheckRegistrationConflictsTx(
	tx *sql.Tx,
	network string,
	params service.CreateRegistrationParams,
) error {
	_, mainNet, err := net.ParseCIDR(params.MainRoute)
	if err != nil {
		return fmt.Errorf("parse registration main route %q: %w", params.MainRoute, err)
	}
	ones, bits := mainNet.Mask.Size()
	base, _ := netaddr.Range(mainNet)

	var conflict int
	if err := tx.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM peer
			WHERE network_name = ?1 AND name = ?2
			UNION ALL
			SELECT 1 FROM cidr
			WHERE network_name = ?1
				AND (
					name = ?2
					OR (base = ?3 AND prefix = ?4 AND length = ?5)
				)
			UNION ALL
			SELECT 1 FROM registration
			WHERE network_name = ?1
				AND (
					name = ?2
					OR final_route = ?6
					OR temp_public_key = ?7
				)
		)`,
		network,
		params.Name,
		base,
		ones,
		bits,
		params.MainRoute,
		params.InvitePublicKey,
	).Scan(&conflict); err != nil {
		return CheckSqliteErr("check registration conflicts", err)
	}

	if conflict != 0 {
		return fmt.Errorf(
			"%w: registration name, key, or route is already reserved",
			service.ErrConflict,
		)
	}

	return nil
}

func sqlListReservedRegistrationRoutesTx(
	tx *sql.Tx,
	network string,
	now time.Time,
) (
	[]string,
	error,
) {
	rows, err := tx.Query(`
		SELECT temp_route
		FROM registration
		WHERE network_name = ?1
			AND confirmed = 0
			AND expires_at_unix > ?2`,
		network,
		now.Unix(),
	)
	if err != nil {
		return nil, CheckSqliteErr("list reserved registration routes", err)
	}
	defer rows.Close()

	var routes []string
	for rows.Next() {
		var route string
		if err := rows.Scan(&route); err != nil {
			return nil, CheckSqliteErr("scan reserved registration route", err)
		}
		routes = append(routes, route)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reserved registration routes: %w", err)
	}

	return routes, nil
}

func sqlInsertRegistrationTx(
	tx *sql.Tx,
	network string,
	registration *service.Registration,
) error {
	if _, err := tx.Exec(`
		INSERT INTO registration (
			network_name,
			name,
			temp_public_key,
			temp_route,
			final_route,
			admin,
			redeemed,
			redeemed_key,
			confirmed,
			expires_at_unix,
			created_at_unix
		)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, 0, '', 0, ?7, ?8)`,
		network,
		registration.Name,
		registration.InvitePublicKey,
		registration.InviteRoute,
		registration.MainRoute,
		boolToInt(registration.Admin),
		registration.ExpiresAt.Unix(),
		registration.CreatedAt.Unix(),
	); err != nil {
		return CheckSqliteErr("insert registration", err)
	}
	return nil
}

func sqlGetRegistrationForRedemptionTx(
	tx *sql.Tx,
	network string,
	tempPubKey string,
) (
	*registrationRedemption,
	error,
) {
	row := tx.QueryRow(`
		SELECT
			id,
			name,
			final_route,
			admin,
			redeemed,
			redeemed_key,
			confirmed,
			expires_at_unix
		FROM registration
		WHERE network_name = ?1
			AND temp_public_key = ?2`,
		network,
		tempPubKey,
	)

	var registration registrationRedemption
	var admin int64
	var redeemed int64
	var confirmed int64
	if err := row.Scan(
		&registration.id,
		&registration.name,
		&registration.mainRoute,
		&admin,
		&redeemed,
		&registration.redeemedKey,
		&confirmed,
		&registration.expiresAtUnix,
	); err != nil {
		return nil, CheckSqliteErr("find registration to redeem", err)
	}

	registration.admin = admin != 0
	registration.redeemed = redeemed != 0
	registration.confirmed = confirmed != 0
	return &registration, nil
}

func sqlMarkRegistrationRedeemedTx(
	tx *sql.Tx,
	registrationID int64,
	permPubKey string,
) error {
	if _, err := tx.Exec(`
		UPDATE registration
		SET
			redeemed = 1,
			redeemed_key = ?2
		WHERE id = ?1 AND redeemed = 0`,
		registrationID,
		permPubKey,
	); err != nil {
		return CheckSqliteErr("redeem mark registration", err)
	}

	return nil
}

func sqlGetRedeemedPeerTx(
	tx *sql.Tx,
	registrationID int64,
	permPubKey string,
) (
	*service.Peer,
	error,
) {
	row := tx.QueryRow(`
		SELECT
			p.name,
			p.public_key,
			c.name,
			c.cidr,
			p.admin,
			p.enabled,
			p.confirmed
		FROM registration r
		JOIN peer p
			ON p.network_name = r.network_name
			AND p.public_key = r.redeemed_key
		JOIN cidr c ON c.id = p.cidr_id
		WHERE r.id = ?1
			AND r.redeemed_key = ?2`,
		registrationID,
		permPubKey,
	)

	peer, err := scanPeer(row)
	if err != nil {
		return nil, fmt.Errorf("get redeemed peer: %w", err)
	}

	return peer, nil
}

func sqlLookupMutableRegistrationTx(
	tx *sql.Tx,
	network string,
	registration string,
	now time.Time,
) (
	int64,
	error,
) {
	var registrationID int64
	var confirmed int64
	var expiresAtUnix int64
	if err := tx.QueryRow(`
		SELECT id, confirmed, expires_at_unix FROM registration
		WHERE network_name = ?1 AND name = ?2`,
		network,
		registration,
	).Scan(
		&registrationID,
		&confirmed,
		&expiresAtUnix,
	); err != nil {
		return 0, CheckSqliteErr("find mutable registration", err)
	}

	if confirmed != 0 {
		return 0, fmt.Errorf(
			"%w: confirmed registration %q cannot be modified",
			service.ErrConflict,
			registration,
		)
	}

	if expiresAtUnix <= now.Unix() {
		return 0, fmt.Errorf(
			"%w: registration %q",
			service.ErrRegistrationExpired,
			registration,
		)
	}

	return registrationID, nil
}

func sqlCheckRegistrationReservationTx(
	tx *sql.Tx,
	network string,
	name string,
	route string,
) error {
	var conflict int
	err := tx.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM registration
			WHERE network_name = ?1
				AND confirmed = 0
				AND (name = ?2 OR final_route = ?3)
		)`,
		network,
		name,
		route,
	).Scan(&conflict)
	if err != nil {
		return CheckSqliteErr("check CIDR registration reservation", err)
	}
	if conflict != 0 {
		return fmt.Errorf(
			"%w: CIDR name or route conflicts with a registration",
			service.ErrConflict,
		)
	}
	return nil
}

func sqlCheckRegistrationNameReservedTx(
	tx *sql.Tx,
	network string,
	newName string,
) error {
	var conflict int
	err := tx.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM registration
			WHERE network_name = ?1
				AND confirmed = 0
				AND name = ?2
		)`,
		network,
		newName,
	).Scan(&conflict)
	if err != nil {
		return CheckSqliteErr("check CIDR rename registration reservation", err)
	}
	if conflict != 0 {
		return fmt.Errorf(
			"%w: CIDR name conflicts with a registration",
			service.ErrConflict,
		)
	}
	return nil
}

func sqlDeleteRegistrationByKeyTx(
	tx *sql.Tx,
	network string,
	publicKey string,
) error {
	_, err := tx.Exec(`
		DELETE FROM registration
		WHERE network_name = ?1 AND redeemed_key = ?2`,
		network,
		publicKey,
	)
	return CheckSqliteErr("delete peer registration", err)
}

func nextFreeRegistrationRoute(
	inviteCIDR string,
	reservedRoutes []string,
) (
	string,
	error,
) {
	_, inviteNet, err := net.ParseCIDR(inviteCIDR)
	if err != nil {
		return "", fmt.Errorf("parse invite CIDR %q: %w", inviteCIDR, err)
	}

	reserved := make(map[string]struct{}, len(reservedRoutes))
	for _, route := range reservedRoutes {
		parsed, err := netaddr.ParseRoute(route)
		if err != nil {
			return "", fmt.Errorf("parse reserved registration route %q: %w", route, err)
		}
		reserved[netaddr.Normalize(parsed.IP).String()] = struct{}{}
	}

	candidate := netaddr.Increment(netaddr.FirstAssignable(inviteNet))
	_, last := netaddr.Range(inviteNet)
	for inviteNet.Contains(candidate) && !candidate.Equal(last) {
		normalized := netaddr.Normalize(candidate)
		if _, found := reserved[normalized.String()]; !found {
			route := netaddr.HostRoute(normalized)
			return route.String(), nil
		}
		candidate = netaddr.Increment(candidate)
	}

	return "", fmt.Errorf(
		"%w: invite CIDR %s",
		service.ErrRegistrationAddressExhausted,
		inviteCIDR,
	)
}

func scanRegistration(
	s Scanner,
) (
	*service.Registration,
	error,
) {
	var name string
	var tempPubKey string
	var tempRoute string
	var finalRoute string
	var admin int64
	var redeemed int64
	var redeemedKey string
	var confirmed int64
	var expiresUnix int64
	var createdUnix int64

	if err := s.Scan(
		&name,
		&tempPubKey,
		&tempRoute,
		&finalRoute,
		&admin,
		&redeemed,
		&redeemedKey,
		&confirmed,
		&expiresUnix,
		&createdUnix,
	); err != nil {
		return nil, CheckSqliteErr("scan registration", err)
	}

	return &service.Registration{
		Name:            name,
		InvitePublicKey: tempPubKey,
		InviteRoute:     tempRoute,
		MainRoute:       finalRoute,
		Admin:           admin != 0,
		Redeemed:        redeemed != 0,
		RedeemedKey:     redeemedKey,
		Confirmed:       confirmed != 0,
		ExpiresAt:       time.Unix(expiresUnix, 0),
		CreatedAt:       time.Unix(createdUnix, 0),
	}, nil
}
