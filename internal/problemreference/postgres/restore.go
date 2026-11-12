package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres/referencedb"
)

var (
	// ErrRestoreDatabase indicates that the connected database name did not match.
	ErrRestoreDatabase = errors.New("restored reference database name did not match confirmation")
	// ErrRestoreTerminal indicates that a report already ended the selected request.
	ErrRestoreTerminal = errors.New("restored reference request already reported")
)

const (
	restoredReferencePageSize = 20
	restoredReferenceColumns  = `id, owner_subject_id, draft_id, source_id, recovery_id, state, created_at, cancelled_at`
	selectRestoredReference   = `SELECT ` + restoredReferenceColumns + ` FROM problem_reference_requests WHERE id=$1 AND recovery_id<>$2`
	lockRestoredReference     = selectRestoredReference + ` FOR UPDATE`
)

// RestoredReference describes the ledger entry without exposing its retry key,
// lease token, or any plan, report, or material contents.
type RestoredReference struct {
	ID          string     `json:"id"`
	OwnerID     string     `json:"owner_id"`
	DraftID     string     `json:"draft_id"`
	SourceID    string     `json:"source_id"`
	RecoveryID  string     `json:"recovery_id"`
	State       string     `json:"state"`
	CreatedAt   time.Time  `json:"created_at"`
	CancelledAt *time.Time `json:"cancelled_at"`
}

// RestoredReferencePage holds at most twenty stale queued requests.
type RestoredReferencePage struct {
	Requests  []RestoredReference `json:"requests"`
	NextAfter string              `json:"next_after,omitempty"`
}

// ListRestoredReferences discovers stale queued work on an isolated, closed
// recovery copy. Each page observes its own snapshot.
func ListRestoredReferences(ctx context.Context, pool *pgxpool.Pool, database, epoch, after string) (RestoredReferencePage, error) {
	if ctx == nil || (after != "" && !identity.ValidID(after)) {
		return RestoredReferencePage{}, problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := beginReferenceRestore(ctx, pool, database, epoch)
	if err != nil {
		return RestoredReferencePage{}, err
	}
	defer rollback(tx)
	rows, err := tx.Query(ctx, `SELECT `+restoredReferenceColumns+`
		FROM problem_reference_requests WHERE state='queued' AND recovery_id<>$1 AND id COLLATE "C">$2
		ORDER BY id COLLATE "C" LIMIT $3`, epoch, after, restoredReferencePageSize+1)
	if err != nil {
		return RestoredReferencePage{}, storageError{err}
	}
	items, err := pgx.AppendRows(make([]RestoredReference, 0, restoredReferencePageSize+1), rows, func(row pgx.CollectableRow) (RestoredReference, error) {
		return scanRestoredReference(row)
	})
	if err != nil {
		return RestoredReferencePage{}, storageError{err}
	}
	page := RestoredReferencePage{Requests: items}
	if len(items) > restoredReferencePageSize {
		page.Requests = items[:restoredReferencePageSize]
		page.NextAfter = page.Requests[len(page.Requests)-1].ID
	}
	if err := commitTx(ctx, tx, false); err != nil {
		return RestoredReferencePage{}, err
	}
	return page, nil
}

// InspectRestoredReference reads the state of one request from an older epoch.
func InspectRestoredReference(ctx context.Context, pool *pgxpool.Pool, database, epoch, id string) (RestoredReference, error) {
	return restoredReference(ctx, pool, database, epoch, id, false)
}

// CancelRestoredReference terminates only the selected stale queued request.
// Retry on the same closed epoch returns its existing cancellation receipt.
func CancelRestoredReference(ctx context.Context, pool *pgxpool.Pool, database, epoch, id string) (RestoredReference, error) {
	return restoredReference(ctx, pool, database, epoch, id, true)
}

func restoredReference(ctx context.Context, pool *pgxpool.Pool, database, epoch, id string, cancelRequest bool) (RestoredReference, error) {
	if ctx == nil || !identity.ValidID(id) {
		return RestoredReference{}, problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := beginReferenceRestore(ctx, pool, database, epoch)
	if err != nil {
		return RestoredReference{}, err
	}
	defer rollback(tx)
	query := selectRestoredReference
	if cancelRequest {
		// Admission, cancellation and first reports serialize on policy before
		// locking a request. The closed recovery guard is the predecessor lock.
		if _, err := lockPolicy(ctx, referencedb.New(tx)); err != nil {
			return RestoredReference{}, err
		}
		query = lockRestoredReference
	}
	row, err := scanRestoredReference(tx.QueryRow(ctx, query, id, epoch))
	if errors.Is(err, pgx.ErrNoRows) {
		return RestoredReference{}, problemreference.ErrNotFound
	}
	if err != nil {
		return RestoredReference{}, storageError{err}
	}
	wrote := false
	if cancelRequest {
		switch row.State {
		case problemreference.Reported:
			return RestoredReference{}, ErrRestoreTerminal
		case problemreference.Queued:
			row, err = scanRestoredReference(tx.QueryRow(ctx, `UPDATE problem_reference_requests
			SET state='cancelled', cancelled_at=clock_timestamp() WHERE id=$1 AND recovery_id<>$2 AND state='queued'
			RETURNING `+restoredReferenceColumns, id, epoch))
			if err != nil {
				return RestoredReference{}, storageError{err}
			}
			wrote = true
		}
	}
	if err := commitTx(ctx, tx, wrote); err != nil {
		return RestoredReference{}, err
	}
	return row, nil
}

func beginReferenceRestore(ctx context.Context, pool *pgxpool.Pool, database, epoch string) (pgx.Tx, error) {
	if pool == nil || database == "" || !identity.ValidRecoveryID(epoch) {
		return nil, problemreference.ErrInvalid
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, storageError{err}
	}
	var matches bool
	switch err := tx.QueryRow(ctx, `SELECT current_database()::text=$1`, database).Scan(&matches); {
	case err != nil:
		rollback(tx)
		return nil, storageError{err}
	case !matches:
		rollback(tx)
		return nil, ErrRestoreDatabase
	}
	if err := identitypg.LockClosedRecovery(ctx, tx, epoch); err != nil {
		rollback(tx)
		return nil, err
	}
	return tx, nil
}

func scanRestoredReference(row pgx.Row) (RestoredReference, error) {
	var result RestoredReference
	if err := row.Scan(&result.ID, &result.OwnerID, &result.DraftID, &result.SourceID, &result.RecoveryID,
		&result.State, &result.CreatedAt, &result.CancelledAt); err != nil {
		return RestoredReference{}, err
	}
	if !identity.ValidID(result.ID) || !identity.ValidID(result.OwnerID) || !identity.ValidID(result.DraftID) || !identity.ValidRecoveryID(result.RecoveryID) ||
		result.CreatedAt.Year() < 1 || result.CreatedAt.Year() > 9999 ||
		(result.State != problemreference.Queued && result.State != problemreference.Cancelled && result.State != problemreference.Reported) ||
		(result.CancelledAt != nil) != (result.State == problemreference.Cancelled) {
		return RestoredReference{}, problemreference.ErrInvalid
	}
	result.CreatedAt = result.CreatedAt.UTC()
	if result.CancelledAt != nil {
		at := result.CancelledAt.UTC()
		if at.Year() < 1 || at.Year() > 9999 {
			return RestoredReference{}, problemreference.ErrInvalid
		}
		result.CancelledAt = &at
	}
	return result, nil
}
