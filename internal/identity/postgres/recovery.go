package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

// RecoveryState reads maintenance metadata without opening or repairing it.
// A missing singleton fails closed; the initial placeholder is empty/disabled.
func (s *Store) RecoveryState(ctx context.Context) (identity.RecoveryState, error) {
	if err := s.checkRecovery(ctx); err != nil {
		return identity.RecoveryState{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	row, err := s.q.RecoveryState(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.RecoveryState{}, identity.ErrRecovering
	}
	if err != nil {
		return identity.RecoveryState{}, storageError{err}
	}
	return recoveryState(row.Generation, row.Enabled)
}

// BeginRecovery explicitly rotates the existing singleton to a new closed epoch.
// It is trusted maintenance, not a Redis cleanup or process-isolation workflow.
// Missing rows are never recreated; errors may follow a committed change and
// must not trigger an automatic retry with another random generation.
func (s *Store) BeginRecovery(ctx context.Context) (string, error) {
	if err := s.checkRecovery(ctx); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.beginRecovery(ctx, rand.Reader)
}

func (s *Store) beginRecovery(ctx context.Context, source io.Reader) (string, error) {
	var random [16]byte
	if _, err := io.ReadFull(source, random[:]); err != nil {
		return "", identity.ErrRecoveryRandom
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	generation := hex.EncodeToString(random[:])
	row, err := s.q.BeginRecovery(ctx, generation)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", identity.ErrRecovering
	}
	if err != nil {
		return "", storageError{err}
	}
	if row.Generation != generation || row.Enabled {
		return "", storageError{identity.ErrInvalid}
	}
	return generation, nil
}

// FinishRecovery opens exactly one still-closed expected epoch. The trusted
// caller must have completed external isolation, Redis cleanup and latest
// account-fact checks; this conditional update does not verify those facts.
func (s *Store) FinishRecovery(ctx context.Context, expected string) error {
	if err := s.checkRecovery(ctx); err != nil {
		return err
	}
	if !identity.ValidRecoveryID(expected) {
		return identity.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	n, err := s.q.FinishRecovery(ctx, expected)
	if err != nil {
		return storageError{err}
	}
	if n != 1 {
		return identity.ErrConflict
	}
	return nil
}

func (s *Store) checkRecovery(ctx context.Context) error {
	if s == nil || s.pool == nil || s.q == nil || ctx == nil {
		return identity.ErrInvalid
	}
	return ctx.Err()
}

func recoveryState(generation string, enabled bool) (identity.RecoveryState, error) {
	if (generation != "" && !identity.ValidRecoveryID(generation)) || (enabled && generation == "") {
		return identity.RecoveryState{}, storageError{identity.ErrInvalid}
	}
	return identity.RecoveryState{Generation: generation, Enabled: enabled}, nil
}
