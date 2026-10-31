package sessionexperiment

import (
	"context"
	"errors"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A test-only alternative, never a fallback or second authority for Redis.
// Registry rows serialize admission and revocation for one subject. Retaining
// token rows until the absolute deadline prevents late saves from recreating
// revoked or idle-expired sessions. This experiment does not run a cleanup job.
type postgresSessionStore struct {
	pool      *pgxpool.Pool
	namespace string
	account   bool
	recovery  bool
}

var _ scs.CtxStore = (*postgresSessionStore)(nil)

const postgresSessionSchema = `
CREATE TABLE IF NOT EXISTS session_pg_subject_fixture (
 namespace text NOT NULL, subject text NOT NULL, PRIMARY KEY(namespace,subject)
);
CREATE TABLE IF NOT EXISTS session_pg_store_fixture (
 namespace text NOT NULL, token text NOT NULL, subject text NOT NULL,
 data bytea, deadline_ms bigint NOT NULL CHECK(deadline_ms>0),
 expiry_ms bigint NOT NULL CHECK(expiry_ms>0 AND expiry_ms<=deadline_ms),
 PRIMARY KEY(namespace,token),
 FOREIGN KEY(namespace,subject) REFERENCES session_pg_subject_fixture(namespace,subject)
);
CREATE INDEX IF NOT EXISTS session_pg_active_fixture
 ON session_pg_store_fixture(namespace,subject,expiry_ms) WHERE data IS NOT NULL;
`

func ordinaryPayload(b []byte) (string, int64, error) {
	deadline, values, err := (scs.GobCodec{}).Decode(b)
	if err != nil {
		return "", 0, err
	}
	subject, ok := values["subject"].(string)
	_, scoped := values["session_scope"]
	if !ok || subject == "" || scoped || values["remember_login"] != false || values["__rememberMe"] != false {
		return "", 0, errStale
	}
	// Match the existing Redis experiment's millisecond deadline contract;
	// retain the original Gob bytes, including their sub-millisecond precision.
	return digest(subject), deadline.UnixMilli(), nil
}

func (s *postgresSessionStore) transact(ctx context.Context, operation func(pgx.Tx) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if err := operation(tx); err != nil {
		return err
	}
	// A commit acknowledgement failure is unknown, not proof of rollback.
	return tx.Commit(ctx)
}

func (s *postgresSessionStore) lockSubject(ctx context.Context, tx pgx.Tx, subject string) error {
	if _, err := tx.Exec(ctx, `INSERT INTO session_pg_subject_fixture VALUES($1,$2) ON CONFLICT DO NOTHING`, s.namespace, subject); err != nil {
		return err
	}
	var locked string
	return tx.QueryRow(ctx, `SELECT subject FROM session_pg_subject_fixture
		WHERE namespace=$1 AND subject=$2 FOR UPDATE`, s.namespace, subject).Scan(&locked)
}

func (s *postgresSessionStore) withSubject(ctx context.Context, subject string, version authVersion, operation func(pgx.Tx) error) error {
	return s.transact(ctx, func(tx pgx.Tx) error {
		if s.recovery && version.account > 0 {
			if err := s.lockRecovery(ctx, tx, version.recovery); err != nil {
				return err
			}
		}
		if s.account && version.account > 0 {
			if err := s.lockAccount(ctx, tx, subject, version.account); err != nil {
				return err
			}
		}
		if err := s.lockSubject(ctx, tx, subject); err != nil {
			return err
		}
		return operation(tx)
	})
}

func (s *postgresSessionStore) CommitCtx(ctx context.Context, token string, b []byte, expiry time.Time) error {
	subject, deadline, err := ordinaryPayload(b)
	if err != nil {
		return err
	}
	if token == "" {
		return errStale
	}
	var generation int64
	if s.account {
		generation, err = postgresAccountGeneration(b)
		if err != nil {
			return err
		}
	}
	var recovery string
	if s.recovery {
		recovery, err = postgresRecoveryGeneration(b)
		if err != nil {
			return err
		}
	}
	return s.withSubject(ctx, subject, authVersion{generation, recovery}, func(tx pgx.Tx) error {
		var oldSubject string
		var oldDeadline, oldExpiry int64
		var oldData []byte
		err := tx.QueryRow(ctx, `SELECT subject,deadline_ms,expiry_ms,data FROM session_pg_store_fixture
			WHERE namespace=$1 AND token=$2 FOR UPDATE`, s.namespace, digest(token)).Scan(&oldSubject, &oldDeadline, &oldExpiry, &oldData)
		found := err == nil
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		// Read the database clock after all row locks: contention must not let
		// an already expired session pass a check made before the wait.
		var now int64
		if err := tx.QueryRow(ctx, `SELECT floor(extract(epoch FROM clock_timestamp())*1000)::bigint`).Scan(&now); err != nil {
			return err
		}
		expires := expiry.UnixMilli()
		if expires <= now || deadline <= now || expires > deadline || deadline > now+(12*time.Hour).Milliseconds() {
			return errStale
		}
		if found {
			if oldSubject != subject || oldDeadline != deadline || oldExpiry <= now || oldData == nil {
				return errStale
			}
			if s.account {
				oldGeneration, err := postgresAccountGeneration(oldData)
				if err != nil || oldGeneration != generation {
					return errStale
				}
			}
			if s.recovery {
				oldRecovery, err := postgresRecoveryGeneration(oldData)
				if err != nil || oldRecovery != recovery {
					return errStale
				}
			}
			_, err := tx.Exec(ctx, `UPDATE session_pg_store_fixture SET data=$3,expiry_ms=$4
				WHERE namespace=$1 AND token=$2`, s.namespace, digest(token), b, expires)
			return err
		}
		var active int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM session_pg_store_fixture
			WHERE namespace=$1 AND subject=$2 AND data IS NOT NULL AND expiry_ms>$3`, s.namespace, subject, now).Scan(&active); err != nil {
			return err
		}
		if active >= 3 {
			return errLimit
		}
		_, err = tx.Exec(ctx, `INSERT INTO session_pg_store_fixture VALUES($1,$2,$3,$4,$5,$6)`,
			s.namespace, digest(token), subject, b, deadline, expires)
		return err
	})
}

func (s *postgresSessionStore) FindCtx(ctx context.Context, token string) ([]byte, bool, error) {
	if s.recovery {
		return s.findWithRecovery(ctx, token)
	}
	var b []byte
	var subject string
	var deadline int64
	var generation int64
	var err error
	if s.account {
		err = s.pool.QueryRow(ctx, `SELECT s.data,s.subject,s.deadline_ms,a.generation
			FROM session_pg_store_fixture s JOIN session_pg_account_fixture a
			ON a.namespace=s.namespace AND a.subject=s.subject
			WHERE s.namespace=$1 AND s.token=$2 AND s.data IS NOT NULL AND a.enabled
			AND s.expiry_ms>floor(extract(epoch FROM clock_timestamp())*1000)::bigint
			AND s.deadline_ms<=floor(extract(epoch FROM clock_timestamp())*1000)::bigint+$3`,
			s.namespace, digest(token), (12*time.Hour).Milliseconds()).Scan(&b, &subject, &deadline, &generation)
	} else {
		err = s.pool.QueryRow(ctx, `SELECT data,subject,deadline_ms FROM session_pg_store_fixture
		WHERE namespace=$1 AND token=$2 AND data IS NOT NULL AND expiry_ms>floor(extract(epoch FROM clock_timestamp())*1000)::bigint
		AND deadline_ms<=floor(extract(epoch FROM clock_timestamp())*1000)::bigint+$3`,
			s.namespace, digest(token), (12*time.Hour).Milliseconds()).Scan(&b, &subject, &deadline)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	decodedSubject, decodedDeadline, err := ordinaryPayload(b)
	if err != nil || decodedSubject != subject || decodedDeadline != deadline {
		return nil, false, errStale
	}
	if s.account {
		payloadGeneration, err := postgresAccountGeneration(b)
		if err != nil || payloadGeneration != generation {
			return nil, false, errStale
		}
	}
	return b, true, nil
}

func (s *postgresSessionStore) DeleteCtx(ctx context.Context, token string) error {
	var subject string
	err := s.pool.QueryRow(ctx, `SELECT subject FROM session_pg_store_fixture WHERE namespace=$1 AND token=$2`,
		s.namespace, digest(token)).Scan(&subject)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // Match Redis's idempotent deletion of an unknown token.
	}
	if err != nil {
		return err
	}
	// Deletion only removes this token's payload. It never restores authority
	// or accesses the account lock after acquiring the subject lock.
	return s.withSubject(ctx, subject, authVersion{}, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE session_pg_store_fixture SET data=NULL WHERE namespace=$1 AND token=$2`,
			s.namespace, digest(token))
		return err
	})
}

func (s *postgresSessionStore) Commit(token string, b []byte, expiry time.Time) error {
	return s.CommitCtx(context.Background(), token, b, expiry)
}
func (s *postgresSessionStore) Find(token string) ([]byte, bool, error) {
	return s.FindCtx(context.Background(), token)
}
func (s *postgresSessionStore) Delete(token string) error {
	return s.DeleteCtx(context.Background(), token)
}
