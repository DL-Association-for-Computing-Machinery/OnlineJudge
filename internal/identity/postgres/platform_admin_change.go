package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres/identitydb"
	"github.com/jackc/pgx/v5"
)

// ChangeAsAdmin appends a revision in the transaction that revalidates a
// freshly verified actor password snapshot. confirm must be a bounded read of
// the original management credential, with no PostgreSQL access or side effects.
// No caller should retry an error that could follow an unknown commit.
func (s *PlatformAdmins) ChangeAsAdmin(ctx context.Context, actor identity.PlatformAdminActor, credentialVersion int64, req identity.PlatformAdminRoleRequest, confirm func(context.Context) error) (identity.PlatformAdminRole, error) {
	var zero identity.PlatformAdminRole
	if s == nil || s.pool == nil || ctx == nil || !actor.Valid() || credentialVersion <= 0 || !req.Valid() || confirm == nil || actor.SubjectID == req.SubjectID {
		return zero, identity.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return zero, storageError{err}
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	q := identitydb.New(tx)
	if err := lockRecovery(ctx, q, actor.RecoveryID); err != nil {
		if errors.Is(err, identity.ErrConflict) {
			return zero, identity.ErrPlatformAdminRequired
		}
		return zero, err
	}
	first, second := actor.SubjectID, req.SubjectID
	if strings.Compare(first, second) > 0 {
		first, second = second, first
	}
	var targetExists, targetEnabled bool
	// Reciprocal role changes must take account locks in identical order.
	// Taking both as UPDATE locks also avoids an actor lock upgrade later.
	for _, id := range []string{first, second} {
		account, err := q.AccountForRoleChange(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			if id == actor.SubjectID {
				return zero, identity.ErrPlatformAdminRequired
			}
			continue
		}
		if err != nil {
			return zero, storageError{err}
		}
		if id == req.SubjectID {
			targetExists = true
			targetEnabled = account.Enabled
		}
	}
	if _, err := lockPlatformAdmin(ctx, q, actor.SubjectID, actor.SessionVersion, credentialVersion, actor.RoleRevision); err != nil {
		// Distinguish actor invalidation from a stale target revision.
		if errors.Is(err, identity.ErrConflict) {
			return zero, identity.ErrPlatformAdminRequired
		}
		return zero, err
	}
	if !targetExists {
		return zero, identity.ErrNotFound
	}
	change := identity.PlatformAdminChange{SubjectID: req.SubjectID, ExpectedRevision: req.ExpectedRevision, Enabled: req.Enabled,
		Operator: identity.PlatformAdminOperatorPrefix + actor.SubjectID, Reason: req.Reason}
	next, err := appendPlatformAdmin(ctx, q, change, targetEnabled, confirm)
	if err != nil {
		return zero, err
	}
	if err := tx.Commit(ctx); err != nil {
		return zero, storageError{err}
	}
	return next, nil
}
