// Package mailbudget stores cumulative seal reservations outside the mail
// issuance database. A charge is committed before a seal is permitted.
package mailbudget

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
)

// MaxReservations leaves headroom below AES-GCM's random nonce limit.
const MaxReservations int64 = 1 << 31

// ErrUnavailable conceals database diagnostics, identifiers and budget state.
var ErrUnavailable = errors.New("mail seal budget unavailable")

// LedgerSQL is installed explicitly by the operator in the independent budget
// database. The runtime role must not have INSERT, DELETE or DDL access.
const LedgerSQL = `CREATE TABLE public.oj_mail_seal_budget (
    fingerprint bytea PRIMARY KEY CHECK (octet_length(fingerprint)=32),
    used bigint NOT NULL DEFAULT 0 CHECK (used >= 0),
    max_seals bigint NOT NULL CHECK (max_seals BETWEEN 1 AND 2147483648),
    CHECK (used <= max_seals)
)`

// Initialize creates a ledger in a dedicated database during maintenance.
// An existing relation is an error; this operation never repairs or resets it.
func Initialize(ctx context.Context, pool *pgxpool.Pool) error {
	if ctx == nil || pool == nil {
		return ErrUnavailable
	}
	return write(ctx, pool, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, LedgerSQL); err != nil {
			return ErrUnavailable
		}
		return nil
	})
}

// Provision initializes a budget for demonstrably fresh key material. The pool
// is a trusted maintenance pool; never use this to recover missing state.
// An existing fingerprint keeps its counter and ceiling even under a new ID.
func Provision(ctx context.Context, pool *pgxpool.Pool, protector *emailtoken.Protector, ceiling int64) error {
	fingerprint := protector.CurrentKeyFingerprint()
	if ctx == nil || pool == nil || fingerprint == ([32]byte{}) || ceiling < 1 || ceiling > MaxReservations {
		return ErrUnavailable
	}
	return write(ctx, pool, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO public.oj_mail_seal_budget(fingerprint,max_seals) VALUES($1,$2)
		    ON CONFLICT (fingerprint) DO NOTHING`, fingerprint[:], ceiling)
		if err != nil || tag.RowsAffected() != 1 {
			return ErrUnavailable
		}
		return nil
	})
}

// Budget binds one immutable Protector to the pool for the independent ledger.
// Construct a new budget with each active-key rotation. All issuers must use
// the same independent database and must never restore it to an older point.
type Budget struct {
	pool        *pgxpool.Pool
	id          string
	fingerprint [32]byte
}

// Format redacts the budget's key metadata and pool.
func (Budget) Format(state fmt.State, _ rune) {
	_, _ = state.Write([]byte("[redacted mail seal budget]"))
}

func (b *Budget) matches(protector *emailtoken.Protector) bool {
	return b != nil && b.pool != nil && b.id == protector.CurrentKeyID() &&
		b.fingerprint == protector.CurrentKeyFingerprint()
}

// New binds the active key; it does not initialize or reset any counters.
func New(pool *pgxpool.Pool, protector *emailtoken.Protector) (*Budget, error) {
	fingerprint := protector.CurrentKeyFingerprint()
	if pool == nil || fingerprint == ([32]byte{}) {
		return nil, ErrUnavailable
	}
	return &Budget{pool: pool, id: protector.CurrentKeyID(), fingerprint: fingerprint}, nil
}

// Reserve checks the issuer's active material and label, then reserves one
// seal. It returns nil only after a confirmed commit. Failures never trigger
// a refund, retry or reinitialization.
func (b *Budget) Reserve(ctx context.Context, protector *emailtoken.Protector) error {
	if ctx == nil || !b.matches(protector) {
		return ErrUnavailable
	}
	return write(ctx, b.pool, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE public.oj_mail_seal_budget SET used=used+1
		    WHERE fingerprint=$1 AND used>=0 AND used<max_seals AND max_seals BETWEEN 1 AND 2147483648`, b.fingerprint[:])
		if err != nil || tag.RowsAffected() != 1 {
			return ErrUnavailable
		}
		return nil
	})
}

func write(ctx context.Context, pool *pgxpool.Pool, action func(context.Context, pgx.Tx) error) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	// Never accept a locally asynchronous commit, even if the pool default
	// is off. Storage must still enforce fsync and no rewind on failover.
	if _, err := tx.Exec(ctx, "SET LOCAL synchronous_commit = on"); err != nil {
		return ErrUnavailable
	}
	if err := action(ctx, tx); err != nil {
		return ErrUnavailable
	}
	if err := tx.Commit(ctx); err != nil {
		return ErrUnavailable
	}
	return nil
}
