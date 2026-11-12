package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrRestoreDatabase indicates that the connected database was not confirmed.
	ErrRestoreDatabase = errors.New("restored database name did not match confirmation")
	// ErrRestoreState indicates an inconsistent archived task and attempt pair.
	ErrRestoreState = errors.New("restored running task has no matching lease")
)

// RestoredLeaseChanges counts changes committed by FenceRestoredLeases.
type RestoredLeaseChanges struct {
	Expired, Requeued int64
}

// FenceRestoredLeases repairs an isolated restored queue before any clients are
// allowed to connect. The operator must stop the old controllers and Workers;
// the table locks do not establish that isolation. Until reopening, repeating a
// committed operation is a no-op. This does not rotate Worker credentials.
func FenceRestoredLeases(ctx context.Context, pool *pgxpool.Pool, database string) (RestoredLeaseChanges, error) {
	if database == "" {
		return RestoredLeaseChanges{}, ErrRestoreDatabase
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return RestoredLeaseChanges{}, err
	}
	defer rollback(tx)
	var sameDatabase bool
	if err := tx.QueryRow(ctx, `SELECT current_database()::text = $1`, database).Scan(&sameDatabase); err != nil {
		return RestoredLeaseChanges{}, err
	}
	if !sameDatabase {
		return RestoredLeaseChanges{}, ErrRestoreDatabase
	}
	// Reject current queue activity rather than waiting for a live controller.
	if _, err := tx.Exec(ctx, `LOCK TABLE judge_tasks, judge_attempts IN ACCESS EXCLUSIVE MODE NOWAIT`); err != nil {
		return RestoredLeaseChanges{}, err
	}
	var inconsistent bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM judge_tasks t WHERE t.state='running' AND NOT EXISTS (
			SELECT 1 FROM judge_attempts a WHERE a.submission_id=t.submission_id
			AND a.judge_generation=t.judge_generation AND a.recovery_generation=t.recovery_generation
			AND a.attempt_number=t.attempt_number AND a.state='leased'
		)
	)`).Scan(&inconsistent); err != nil {
		return RestoredLeaseChanges{}, err
	}
	if inconsistent {
		return RestoredLeaseChanges{}, ErrRestoreState
	}
	// Expire leases from superseded generations too: their node capacity is
	// counted independently of task generation during a fresh claim.
	expired, err := tx.Exec(ctx, `UPDATE judge_attempts SET state='expired' WHERE state='leased'`)
	if err != nil {
		return RestoredLeaseChanges{}, err
	}
	// A restored running task starts a fresh attempt budget under its new
	// recovery generation. Finished, failed and already queued tasks are untouched.
	requeued, err := tx.Exec(ctx, `UPDATE judge_tasks SET state='queued',
		recovery_generation=recovery_generation+1, attempt_number=0 WHERE state='running'`)
	if err != nil {
		return RestoredLeaseChanges{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RestoredLeaseChanges{}, err
	}
	return RestoredLeaseChanges{Expired: expired.RowsAffected(), Requeued: requeued.RowsAffected()}, nil
}
