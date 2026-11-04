package postgres

import (
	"context"
	"errors"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres/problemdb"
	"github.com/jackc/pgx/v5"
)

// AuthorSnapshots is identity's public same-transaction authorization adapter.
type AuthorSnapshots interface {
	LockAuthorSnapshot(context.Context, pgx.Tx, identity.ProblemActor) (identity.ProblemAuthorSnapshot, error)
}

// ReferenceAccess exposes narrow draft identities, never problem's private SQL.
// The caller owns a short READ COMMITTED tx and acquires later business locks.
type ReferenceAccess struct{ authors AuthorSnapshots }

// NewReferenceAccess binds current identity authority without owning its pool.
func NewReferenceAccess(authors AuthorSnapshots) (*ReferenceAccess, error) {
	if authors == nil {
		return nil, problem.ErrInvalid
	}
	return &ReferenceAccess{authors: authors}, nil
}

// LockAuthorSnapshot holds recovery/account/role through the caller's tx end.
func (a *ReferenceAccess) LockAuthorSnapshot(ctx context.Context, tx pgx.Tx, actor identity.ProblemActor) (identity.ProblemAuthorSnapshot, error) {
	if a == nil || a.authors == nil || ctx == nil || tx == nil || !actor.Valid() {
		return identity.ProblemAuthorSnapshot{}, problem.ErrInvalid
	}
	return a.authors.LockAuthorSnapshot(ctx, tx, actor)
}

// LockOwnDraftIdentity checks current authority then locks only the owner's
// fixed draft row. Call BEFORE any policy, request or other later business locks.
// Rechecking is safe only when this tx already holds authority for the SAME actor;
// it does not permit acquiring new predecessor locks after business rows.
func (a *ReferenceAccess) LockOwnDraftIdentity(ctx context.Context, tx pgx.Tx, actor identity.ProblemActor, id string) (problem.DraftIdentity, error) {
	if !identity.ValidID(id) {
		return problem.DraftIdentity{}, problem.ErrInvalid
	}
	if _, err := a.LockAuthorSnapshot(ctx, tx, actor); err != nil {
		return problem.DraftIdentity{}, err
	}
	r, err := problemdb.New(tx).DraftIdentityByOwner(ctx, problemdb.DraftIdentityByOwnerParams{ID: id, Owner: actor.SubjectID})
	if errors.Is(err, pgx.ErrNoRows) {
		return problem.DraftIdentity{}, problem.ErrNotFound
	}
	if err != nil {
		return problem.DraftIdentity{}, storageError{err}
	}
	if r.State != "private-draft" || r.ValidationState != "not-run" || !problem.ValidImportRevision(r.ContentVersion) || !problem.ValidImportRevision(r.RequestSha256) {
		return problem.DraftIdentity{}, storageError{problem.ErrInvalid}
	}
	if err = ctx.Err(); err != nil {
		return problem.DraftIdentity{}, err
	}
	return problem.DraftIdentity{ID: r.ID, Owner: r.OwnerSubjectID, SourceID: r.SourceID, Version: r.ContentVersion, ImportRevision: r.RequestSha256}, nil
}
