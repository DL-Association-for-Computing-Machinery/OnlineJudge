package sessionexperiment

import (
	"context"
	"errors"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5"
)

const postgresAccountSchema = `CREATE TABLE IF NOT EXISTS session_pg_account_fixture (
 namespace text NOT NULL, subject text NOT NULL,
 generation bigint NOT NULL CHECK(generation>0), enabled boolean NOT NULL,
 PRIMARY KEY(namespace,subject))`

func postgresAccountGeneration(b []byte) (int64, error) {
	_, values, err := (scs.GobCodec{}).Decode(b)
	if err != nil {
		return 0, err
	}
	generation, ok := values["auth_generation"].(int64)
	if !ok || generation <= 0 {
		return 0, errStale
	}
	return generation, nil
}

func (s *postgresSessionStore) currentAccount(ctx context.Context, subject string) (int64, error) {
	var generation int64
	var enabled bool
	err := s.pool.QueryRow(ctx, `SELECT generation,enabled FROM session_pg_account_fixture
		WHERE namespace=$1 AND subject=$2`, s.namespace, digest(subject)).Scan(&generation, &enabled)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !enabled) {
		return 0, errStale
	}
	return generation, err
}

func (s *postgresSessionStore) lockAccount(ctx context.Context, tx pgx.Tx, subject string, expected int64) error {
	var generation int64
	var enabled bool
	err := tx.QueryRow(ctx, `SELECT generation,enabled FROM session_pg_account_fixture
		WHERE namespace=$1 AND subject=$2 FOR SHARE`, s.namespace, subject).Scan(&generation, &enabled)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (!enabled || generation != expected)) {
		return errStale
	}
	return err
}

// Test-only privileged operation: invalidate old credentials and free their
// slots atomically with the account change. Account -> subject -> token is the
// same order as Commit. No account/password administration endpoint is exposed.
func (s *postgresSessionStore) invalidateAccount(ctx context.Context, subject string, enabled bool) error {
	if !s.account {
		return errStale
	}
	return s.transact(ctx, func(tx pgx.Tx) error {
		return s.invalidateAccountTx(ctx, tx, subject, enabled)
	})
}

func (s *postgresSessionStore) invalidateAccountTx(ctx context.Context, tx pgx.Tx, subject string, enabled bool) error {
	hash := digest(subject)
	var generation int64
	if err := tx.QueryRow(ctx, `SELECT generation FROM session_pg_account_fixture
			WHERE namespace=$1 AND subject=$2 FOR UPDATE`, s.namespace, hash).Scan(&generation); err != nil {
		return err
	}
	if err := s.lockSubject(ctx, tx, hash); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE session_pg_account_fixture SET generation=generation+1,enabled=$3
			WHERE namespace=$1 AND subject=$2`, s.namespace, hash, enabled); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE session_pg_store_fixture SET data=NULL
			WHERE namespace=$1 AND subject=$2 AND data IS NOT NULL`, s.namespace, hash)
	return err
}
