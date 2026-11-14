package redisstore

import (
	"context"
	"encoding/base64"
	"io"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

// Rotate transfers the caller's slot to a fresh credential without extending
// either deadline. It grants no other permissions. After an uncertain write,
// the caller must check its session or sign in; it receives no successor.
func (s *Store) Rotate(ctx context.Context, c Credential) (LoginGrant, error) {
	if err := s.check(ctx); err != nil {
		return LoginGrant{}, err
	}
	found, err := s.locate(ctx, c)
	if err != nil {
		return LoginGrant{}, err
	}
	var entropy [32]byte
	if _, err := io.ReadFull(s.random, entropy[:]); err != nil {
		return LoginGrant{}, ErrRandom
	}
	old := &found.attempt
	next := *old
	next.credential = Credential{token: base64.RawURLEncoding.EncodeToString(entropy[:])}
	keys := append(s.keys(old), s.keys(&next)[0])
	args := append(s.args(old), digest(next.credential.token), binding(&next))
	version := identity.SessionVersion{AuthGeneration: old.state.AuthGeneration, RecoveryID: old.state.RecoveryID}
	err = s.authority.WithSession(ctx, old.state.ID, version, func(call context.Context, _ identity.SessionState) error {
		values, err := s.eval(call, "rotate", keys, args)
		if err == nil && len(values) != 0 {
			return storageError{ErrConflict}
		}
		return err
	})
	if err != nil {
		return LoginGrant{}, err
	}
	if _, err := s.Read(ctx, next.credential); err != nil {
		// The old credential may already be dead, even if confirmation fails.
		return LoginGrant{}, storageError{err}
	}
	return grantFor(&next), nil
}

func grantFor(a *Attempt) LoginGrant {
	result := LoginGrant{Credential: a.credential}
	if a.matchesPolicy(rememberedPolicy) {
		result.PersistentUntil = time.UnixMilli(a.absolute).UTC()
	}
	return result
}
