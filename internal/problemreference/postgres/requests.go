package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres/referencedb"
	"github.com/jackc/pgx/v5"
)

// Enqueue first recovers an original key, even if the catalog/policy changed.
// New work locks author -> draft -> policy -> request; the policy serializes all
// admission/state changes. A second key read after waiting is essential when
// the winner has just filled the queue. No material I/O occurs under these locks.
func (s *Store) Enqueue(ctx context.Context, actor identity.ProblemActor, key, draftID string, catalog *problemreference.Catalog) (problemreference.Request, error) {
	if ctx == nil || !problem.ValidKey(key) || !identity.ValidID(draftID) || catalog == nil {
		return problemreference.Request{}, problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, authority, err := s.begin(ctx, actor)
	if err != nil {
		return problemreference.Request{}, err
	}
	defer rollback(tx)
	q := referencedb.New(tx)
	if old, found, e := existing(ctx, q, actor, key, draftID); e != nil {
		return problemreference.Request{}, e
	} else if found {
		return commit(ctx, tx, old, false)
	}
	draft, err := s.access.LockOwnDraftIdentity(ctx, tx, actor, draftID)
	if errors.Is(err, problem.ErrNotFound) {
		return problemreference.Request{}, problemreference.ErrNotFound
	}
	if err != nil {
		return problemreference.Request{}, err
	}
	if draft.ID != draftID || draft.Owner != actor.SubjectID {
		return problemreference.Request{}, storageError{problemreference.ErrInvalid}
	}
	limits, err := lockPolicy(ctx, q)
	if err != nil {
		return problemreference.Request{}, err
	}
	if old, found, e := existing(ctx, q, actor, key, draftID); e != nil {
		return problemreference.Request{}, e
	} else if found {
		return commit(ctx, tx, old, false)
	}
	spec, err := catalog.Select(draft)
	if err != nil {
		return problemreference.Request{}, err
	}
	counts, err := q.ReferenceQueuedCounts(ctx, referencedb.ReferenceQueuedCountsParams{Maximum: limits.Global, Owner: actor.SubjectID})
	if err != nil {
		return problemreference.Request{}, storageError{err}
	}
	if counts.Total >= limits.Global || counts.Personal >= limits.Owner {
		return problemreference.Request{}, problemreference.ErrFull
	}
	var id [16]byte
	if _, err = rand.Read(id[:]); err != nil {
		return problemreference.Request{}, storageError{err}
	}
	row, err := q.CreateReferenceRequest(ctx, referencedb.CreateReferenceRequestParams{ID: hex.EncodeToString(id[:]), Owner: actor.SubjectID, RequestKey: key, DraftID: draftID,
		SourceID: spec.Identity.SourceID, ContentVersion: spec.Identity.Version, ImportRevision: spec.Identity.ImportRevision, SourceLockSha256: spec.Identity.SourceLockSHA256,
		IdentityProtocol: spec.Protocol, ProfileEncoding: spec.ProfileJSON, ProfileSha256: spec.ProfileSHA256,
		AuthGeneration: authority.Actor.AuthGeneration, RecoveryID: authority.Actor.RecoveryID, AuthorRevision: authority.RoleRevision})
	if errors.Is(err, pgx.ErrNoRows) {
		// The unique key remains the final defense even for another trusted writer.
		old, found, e := existing(ctx, q, actor, key, draftID)
		if e != nil {
			return problemreference.Request{}, e
		}
		if !found {
			return problemreference.Request{}, storageError{problemreference.ErrInvalid}
		}
		return commit(ctx, tx, old, false)
	}
	if err != nil {
		return problemreference.Request{}, storageError{err}
	}
	result, err := readRequest(row)
	if err != nil {
		return problemreference.Request{}, err
	}
	return commit(ctx, tx, result, true)
}

// Receipt is a read-only owner/key observation under current author authority.
// A missing row does not prove another request is not currently in flight.
func (s *Store) Receipt(ctx context.Context, actor identity.ProblemActor, key string) (problemreference.Request, error) {
	if ctx == nil || !problem.ValidKey(key) {
		return problemreference.Request{}, problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, _, err := s.begin(ctx, actor)
	if err != nil {
		return problemreference.Request{}, err
	}
	defer rollback(tx)
	result, found, err := existing(ctx, referencedb.New(tx), actor, key, "")
	if err != nil {
		return problemreference.Request{}, err
	}
	if !found {
		return problemreference.Request{}, problemreference.ErrNotFound
	}
	return commit(ctx, tx, result, false)
}

// Cancel is idempotent and preserves original config/authority. Regranted
// authors can cancel old queued work; this does not renew its execution rights.
func (s *Store) Cancel(ctx context.Context, actor identity.ProblemActor, key string) (problemreference.Request, error) {
	if ctx == nil || !problem.ValidKey(key) {
		return problemreference.Request{}, problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, _, err := s.begin(ctx, actor)
	if err != nil {
		return problemreference.Request{}, err
	}
	defer rollback(tx)
	q := referencedb.New(tx)
	old, found, err := existing(ctx, q, actor, key, "")
	if err != nil {
		return problemreference.Request{}, err
	}
	if !found {
		return problemreference.Request{}, problemreference.ErrNotFound
	}
	if _, err = s.access.LockOwnDraftIdentity(ctx, tx, actor, old.DraftID); err != nil {
		return problemreference.Request{}, err
	}
	if _, err = lockPolicy(ctx, q); err != nil {
		return problemreference.Request{}, err
	}
	row, err := q.ReferenceRequestForUpdate(ctx, referencedb.ReferenceRequestForUpdateParams{Owner: actor.SubjectID, RequestKey: key})
	if err != nil {
		return problemreference.Request{}, storageError{err}
	}
	if row.DraftID != old.DraftID {
		return problemreference.Request{}, storageError{problemreference.ErrInvalid}
	}
	result, err := readRequest(row)
	if err != nil {
		return problemreference.Request{}, err
	}
	if result.State == problemreference.Cancelled {
		return commit(ctx, tx, result, false)
	}
	row, err = q.CancelReferenceRequest(ctx, referencedb.CancelReferenceRequestParams{Owner: actor.SubjectID, RequestKey: key})
	if err != nil {
		return problemreference.Request{}, storageError{err}
	}
	result, err = readRequest(row)
	if err != nil {
		return problemreference.Request{}, err
	}
	return commit(ctx, tx, result, true)
}
