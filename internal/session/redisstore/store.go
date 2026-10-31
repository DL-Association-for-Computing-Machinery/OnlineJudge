package redisstore

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
)

// Authority supplies the single authoritative identity owner for both new
// credentials and existing sessions. Its callbacks must obey the identity Store
// lock order/uncertain-effect contracts; methods must not route to other tenants.
type Authority interface {
	identity.PersonalCredentialStore
	WithSession(context.Context, string, identity.SessionVersion, func(context.Context, identity.SessionState) error) error
}

// Store holds immutable configuration and caller-owned clients/authority.
// Namespace/policy must match across API replicas. Redis noeviction, a primary
// standalone server and external exclusive restore coordination are continuous
// environmental requirements, not properties proved forever by startup checks.
type Store struct {
	client    *redis.Client
	authority Authority
	gate      *identity.PersonalCredentialGate
	namespace string
	policy    Policy
	random    io.Reader
}

// New validates an existing client and the currently running Redis 7.2 server.
// It neither creates Redis, migrates PostgreSQL, opens recovery nor picks budgets.
// The caller must retain the server/namespace invariants after construction.
func New(ctx context.Context, client *redis.Client, authority Authority, work *password.BoundedHasher, namespace string, policy Policy) (*Store, error) {
	if ctx == nil || !boundedClient(client) || !namespacePattern.MatchString(namespace) || !policy.valid() {
		return nil, ErrInvalid
	}
	gate, err := identity.NewPersonalCredentialGate(authority, work)
	if err != nil {
		return nil, err
	}
	call, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	server, err := client.Info(call, "server").Result()
	if err != nil {
		return nil, storageError{err}
	}
	if !strings.Contains(server, "\r\nredis_version:7.2.") {
		return nil, ErrInvalid
	}
	cluster, err := client.Info(call, "cluster").Result()
	if err != nil {
		return nil, storageError{err}
	}
	if !strings.Contains(cluster, "cluster_enabled:0") {
		return nil, ErrInvalid
	}
	role, err := client.Do(call, "ROLE").Slice()
	if err != nil {
		return nil, storageError{err}
	}
	if len(role) == 0 || role[0] != "master" {
		return nil, ErrInvalid
	}
	config, err := client.ConfigGet(call, "maxmemory-policy").Result()
	if err != nil {
		return nil, storageError{err}
	}
	if config["maxmemory-policy"] != "noeviction" {
		return nil, ErrInvalid
	}
	return &Store{client: client, authority: authority, gate: gate, namespace: namespace, policy: policy, random: rand.Reader}, nil
}

func (s *Store) check(ctx context.Context) error {
	if s == nil || s.client == nil || s.authority == nil || s.gate == nil || ctx == nil {
		return ErrInvalid
	}
	return ctx.Err()
}

// Create authenticates and freezes one attempt before its first possible write.
// Entropy and KDF run outside database locks. After freezing, ALL later errors
// return the same Attempt, including unknown Redis/PG commit outcomes. No token
// can be published until a separate successful Check. Never blindly call Create
// again on an error; Retry/Check/Abandon operate on the original frozen attempt.
func (s *Store) Create(ctx context.Context, email identity.Email, secret []byte) (*Attempt, error) {
	if err := s.check(ctx); err != nil {
		return nil, err
	}
	if !email.Valid() {
		return nil, ErrInvalid
	}
	var entropy [32]byte
	if _, err := io.ReadFull(s.random, entropy[:]); err != nil {
		return nil, ErrRandom
	}
	credential := Credential{token: base64.RawURLEncoding.EncodeToString(entropy[:])}
	var attempt *Attempt
	err := s.gate.WithPersonalCredentials(ctx, email, secret, func(call context.Context, state identity.SessionState) error {
		now, err := s.client.Time(call).Result()
		if err != nil {
			return storageError{err}
		}
		created := now.UnixMilli()
		candidate := &Attempt{namespace: s.namespace, credential: credential, state: state, created: created,
			absolute: created + s.policy.Absolute.Milliseconds(), initial: created + s.policy.Idle.Milliseconds()}
		if !candidate.valid(s.namespace, s.policy) {
			return ErrConflict
		}
		attempt = candidate
		// Already inside the identity gate: never acquire it again here.
		return s.runAttempt(call, "retry", attempt)
	})
	return attempt, err
}

func (s *Store) withAttempt(ctx context.Context, a *Attempt, credentials bool, op string) error {
	if err := s.check(ctx); err != nil {
		return err
	}
	if !a.valid(s.namespace, s.policy) {
		return ErrInvalid
	}
	operation := func(call context.Context, _ identity.SessionState) error { return s.runAttempt(call, op, a) }
	if credentials {
		return s.authority.WithSessionCredentials(ctx, a.state.ID, a.state.Revision, a.state.RecoveryID, operation)
	}
	return s.authority.WithSession(ctx, a.state.ID, identity.SessionVersion{AuthGeneration: a.state.AuthGeneration, RecoveryID: a.state.RecoveryID}, operation)
}

// Retry repeats only the original frozen admission. It never touches/overwrites
// an existing record; a record changed by Touch conflicts without being changed.
func (s *Store) Retry(ctx context.Context, a *Attempt) error {
	return s.withAttempt(ctx, a, true, "retry")
}

// Check yields a credential only after a complete positive receipt and final
// authority commit. Missing means ErrUnknown, not unwritten. A Touch changes the
// initial receipt (ErrConflict) even though the current session may remain valid.
func (s *Store) Check(ctx context.Context, a *Attempt) (Credential, error) {
	if err := s.withAttempt(ctx, a, true, "check"); err != nil {
		return Credential{}, err
	}
	return a.credential, nil
}

// Abandon installs/retains a tombstone even before a delayed write. It tolerates
// credential-only rehashes and revokes a touched matching token through its
// original absolute deadline. An error never confirms cleanup or rollback.
func (s *Store) Abandon(ctx context.Context, a *Attempt) error {
	return s.withAttempt(ctx, a, false, "abandon")
}
