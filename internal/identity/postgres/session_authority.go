package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres/identitydb"
)

// WithSessionCredentials coordinates a new candidate using the epoch captured
// BEFORE password verification and the resulting exact credential revisions.
// Never relabel an old verification with an epoch read after recovery/conflict.
// This API cannot itself prove the caller followed that capture ordering.
// Operations obey WithAccount's cooperative/uncertain-effect restrictions.
func (s *Store) WithSessionCredentials(ctx context.Context, id string, want identity.Revision, recoveryID string, operation func(context.Context, identity.SessionState) error) error {
	if !want.Valid() || !identity.ValidRecoveryID(recoveryID) || operation == nil {
		return identity.ErrInvalid
	}
	return s.withAuthority(ctx, id, want, true, sessionScope, recoveryID, func(call context.Context, state identity.State) error {
		return operation(call, identity.SessionState{State: state, RecoveryID: recoveryID})
	})
}

// WithSession coordinates an existing session against open recovery and current
// account state. It deliberately ignores credential version, but requires both
// auth generation and recovery epoch. It neither checks session lifetime/slots
// nor makes external effects transactional; full Redis protocols remain needed.
func (s *Store) WithSession(ctx context.Context, id string, want identity.SessionVersion, operation func(context.Context, identity.SessionState) error) error {
	if !want.Valid() || operation == nil {
		return identity.ErrInvalid
	}
	return s.withAuthority(ctx, id, identity.Revision{AuthGeneration: want.AuthGeneration}, false, sessionScope, want.RecoveryID, func(call context.Context, state identity.State) error {
		return operation(call, identity.SessionState{State: state, RecoveryID: want.RecoveryID})
	})
}

type authorityScope uint8

const (
	subjectScope authorityScope = iota
	sessionScope
)

func lockRecovery(ctx context.Context, q *identitydb.Queries, expected string) error {
	row, err := q.RecoveryStateForShare(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.ErrRecovering
	}
	if err != nil {
		return storageError{err}
	}
	state, err := recoveryState(row.Generation, row.Enabled)
	if err != nil {
		return err
	}
	if !state.Enabled {
		return identity.ErrRecovering
	}
	if state.Generation != expected {
		return identity.ErrConflict
	}
	return nil
}
