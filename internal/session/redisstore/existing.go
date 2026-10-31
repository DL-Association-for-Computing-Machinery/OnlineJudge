package redisstore

import (
	"context"
	"strconv"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

func (s *Store) existing(ctx context.Context, c Credential, op string) (View, error) {
	if err := s.check(ctx); err != nil {
		return View{}, err
	}
	located, err := s.locate(ctx, c)
	if err != nil {
		return View{}, err
	}
	a := &located.attempt
	var result View
	err = s.authority.WithSession(ctx, a.state.ID, identity.SessionVersion{AuthGeneration: a.state.AuthGeneration, RecoveryID: a.state.RecoveryID}, func(call context.Context, _ identity.SessionState) error {
		// The first read only locates authority. Revalidate/operate on the entire
		// current Redis record inside this gate; never return the preloaded view.
		values, err := s.eval(call, op, s.keys(a), s.args(a))
		if err != nil {
			return err
		}
		if op == "abandon" {
			return nil
		}
		if len(values) != 1 {
			return storageError{ErrConflict}
		}
		text, ok := values[0].(string)
		if !ok {
			return storageError{ErrConflict}
		}
		idle, err := strconv.ParseInt(text, 10, 64)
		if err != nil || idle < a.initial || idle > a.absolute {
			return storageError{ErrConflict}
		}
		result = view(a, idle)
		return nil
	})
	if err != nil {
		return View{}, err
	}
	return result, nil
}

// Read returns a fresh, fully indexed ordinary session under current authority.
// It is read-only and cannot be reused as a later resource-authorization grant.
func (s *Store) Read(ctx context.Context, c Credential) (View, error) {
	return s.existing(ctx, c, "read")
}

// Touch renews only an existing, active session. It never creates a missing or
// revoked record, changes ownership, or exceeds its original absolute deadline.
func (s *Store) Touch(ctx context.Context, c Credential) (View, error) {
	return s.existing(ctx, c, "touch")
}

// Revoke tombstones a precisely located token under current account authority.
// If initial lookup is missing, ErrNotFound does NOT confirm revocation. Abandon
// can handle an absent record when a trusted frozen Attempt is still available.
func (s *Store) Revoke(ctx context.Context, c Credential) error {
	_, err := s.existing(ctx, c, "abandon")
	return err
}
