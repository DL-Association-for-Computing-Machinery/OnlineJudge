package sessionexperiment

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5"
)

const postgresRecoverySchema = `CREATE TABLE IF NOT EXISTS session_pg_recovery_fixture (
 namespace text PRIMARY KEY, generation text NOT NULL CHECK(length(generation)>=26),
 enabled boolean NOT NULL)`

func (s *postgresSessionStore) initializeRecovery(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, postgresRecoverySchema); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO session_pg_recovery_fixture VALUES($1,$2,true)`, s.namespace, rand.Text())
	return err
}

func postgresRecoveryGeneration(b []byte) (string, error) {
	_, values, err := (scs.GobCodec{}).Decode(b)
	if err != nil {
		return "", err
	}
	generation, ok := values["recovery_generation"].(string)
	if !ok || len(generation) < 26 {
		return "", errStale
	}
	return generation, nil
}

func (s *postgresSessionStore) currentRecovery(ctx context.Context) (string, error) {
	var generation string
	var enabled bool
	err := s.pool.QueryRow(ctx, `SELECT generation,enabled FROM session_pg_recovery_fixture WHERE namespace=$1`, s.namespace).Scan(&generation, &enabled)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !enabled) {
		return "", errRecovering
	}
	return generation, err
}

// The global row precedes every account/subject/token lock. An empty expected
// ID is reserved for privileged account invalidation, not session persistence.
func (s *postgresSessionStore) lockRecovery(ctx context.Context, tx pgx.Tx, expected string) error {
	var generation string
	var enabled bool
	err := tx.QueryRow(ctx, `SELECT generation,enabled FROM session_pg_recovery_fixture
		WHERE namespace=$1 FOR SHARE`, s.namespace).Scan(&generation, &enabled)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !enabled) {
		return errRecovering
	}
	if err == nil && expected != "" && generation != expected {
		return errStale
	}
	return err
}

func (s *postgresSessionStore) beginRecovery(ctx context.Context) (string, error) {
	generation := rand.Text()
	err := s.transact(ctx, func(tx pgx.Tx) error {
		return s.beginRecoveryTx(ctx, tx, generation)
	})
	// A returned error can include an unknown COMMIT acknowledgement. Never
	// return an ID as if the transaction were known to have succeeded.
	if err != nil {
		return "", err
	}
	return generation, nil
}

func (s *postgresSessionStore) beginRecoveryTx(ctx context.Context, tx pgx.Tx, generation string) error {
	tag, err := tx.Exec(ctx, `UPDATE session_pg_recovery_fixture SET generation=$2,enabled=false WHERE namespace=$1`, s.namespace, generation)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errRecovering
	}
	// Retain exact-token tombstones; stale Delete and Save cannot target a fresh
	// credential or occupy its slot. Clearing payloads and closing are one tx.
	_, err = tx.Exec(ctx, `UPDATE session_pg_store_fixture SET data=NULL WHERE namespace=$1 AND data IS NOT NULL`, s.namespace)
	return err
}

func (s *postgresSessionStore) finishRecovery(ctx context.Context, expected string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE session_pg_recovery_fixture SET enabled=true
		WHERE namespace=$1 AND generation=$2 AND NOT enabled`, s.namespace, expected)
	if err == nil && tag.RowsAffected() != 1 {
		return errRecovering
	}
	return err
}

func (s *postgresSessionStore) findWithRecovery(ctx context.Context, token string) ([]byte, bool, error) {
	var b []byte
	var subject, recovery string
	var deadline, account int64
	var open, enabled bool
	// Start from the gate, preserving its state even for unknown/expired tokens
	// or missing accounts. All authorization fields come from one MVCC snapshot.
	err := s.pool.QueryRow(ctx, `SELECT r.generation,r.enabled,s.data,coalesce(s.subject,''),
		coalesce(s.deadline_ms,0),coalesce(a.generation,0),coalesce(a.enabled,false)
		FROM session_pg_recovery_fixture r
		LEFT JOIN session_pg_store_fixture s ON s.namespace=r.namespace AND s.token=$2 AND s.data IS NOT NULL
		AND s.expiry_ms>floor(extract(epoch FROM clock_timestamp())*1000)::bigint
		AND s.deadline_ms<=floor(extract(epoch FROM clock_timestamp())*1000)::bigint+$3
		LEFT JOIN session_pg_account_fixture a ON a.namespace=s.namespace AND a.subject=s.subject
		WHERE r.namespace=$1`, s.namespace, digest(token), (12*time.Hour).Milliseconds()).Scan(&recovery, &open, &b, &subject, &deadline, &account, &enabled)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !open) {
		return nil, false, errRecovering
	}
	if err != nil {
		return nil, false, err
	}
	if b == nil {
		return nil, false, nil
	}
	decodedSubject, decodedDeadline, err := ordinaryPayload(b)
	if err != nil || decodedSubject != subject || decodedDeadline != deadline || !enabled {
		return nil, false, errStale
	}
	payloadAccount, err := postgresAccountGeneration(b)
	if err != nil || payloadAccount != account {
		return nil, false, errStale
	}
	payloadRecovery, err := postgresRecoveryGeneration(b)
	if err != nil || payloadRecovery != recovery {
		return nil, false, errStale
	}
	return b, true, nil
}
