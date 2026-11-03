// Package postgres implements private draft transactions and owns problem SQL.
package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres/problemdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuthorAccess is implemented by identity's public PostgreSQL adapter. It checks
// identity-owned facts using the caller's SAME transaction and never commits it.
type AuthorAccess interface {
	LockAuthor(context.Context, pgx.Tx, identity.ProblemActor) error
}

// Store uses a caller-owned pool; every operation has an independent 5s bound.
type Store struct {
	pool   *pgxpool.Pool
	access AuthorAccess
}

var _ problem.Repository = (*Store)(nil)

// New requires current identity authorization, not caller-selected permissions.
func New(pool *pgxpool.Pool, access AuthorAccess) (*Store, error) {
	if pool == nil || access == nil {
		return nil, problem.ErrInvalid
	}
	return &Store{pool: pool, access: access}, nil
}

type storageError struct{ cause error }

func (e storageError) Error() string        { return problem.ErrStore.Error() }
func (e storageError) Unwrap() error        { return e.cause }
func (e storageError) Is(target error) bool { return target == problem.ErrStore }

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
func (s *Store) begin(ctx context.Context, a identity.ProblemActor) (pgx.Tx, error) {
	if s == nil || ctx == nil || !a.Valid() {
		return nil, problem.ErrInvalid
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, storageError{err}
	}
	if err = s.access.LockAuthor(ctx, tx, a); err != nil {
		rollback(tx)
		return nil, err
	}
	return tx, nil
}

// CheckAuthor is a short preflight. Every actual operation checks again.
func (s *Store) CheckAuthor(ctx context.Context, a identity.ProblemActor) error {
	if ctx == nil {
		return problem.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.begin(ctx, a)
	if err != nil {
		return err
	}
	defer rollback(tx)
	if err = tx.Commit(ctx); err != nil {
		return storageError{err}
	}
	return nil
}

// Create commits owner, immutable document, all references and retry receipt in
// one authorized transaction. Material persistence is the service's prior step.
func (s *Store) Create(ctx context.Context, a identity.ProblemActor, key string, p *problem.Prepared) (problem.Draft, error) {
	if ctx == nil || !problem.ValidKey(key) || p == nil {
		return problem.Draft{}, problem.ErrInvalid
	}
	c := p.Content()
	if c.RequestSHA256 == "" || len(c.References) == 0 {
		return problem.Draft{}, problem.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.begin(ctx, a)
	if err != nil {
		return problem.Draft{}, err
	}
	defer rollback(tx)
	q := problemdb.New(tx)
	row, err := q.DraftByKeyForShare(ctx, problemdb.DraftByKeyForShareParams{Owner: a.SubjectID, RequestKey: key})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return problem.Draft{}, storageError{err}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		var id [16]byte
		if _, err = rand.Read(id[:]); err != nil {
			return problem.Draft{}, storageError{err}
		}
		row, err = q.CreateDraft(ctx, problemdb.CreateDraftParams{ID: hex.EncodeToString(id[:]), Owner: a.SubjectID, RequestKey: key, RequestSha256: c.RequestSHA256, SourceID: c.SourceID, ContentVersion: c.Version, Document: c.Document})
		if errors.Is(err, pgx.ErrNoRows) {
			// Concurrent identical key: ON CONFLICT waits; this new READ COMMITTED
			// statement observes the winner after its complete transaction commits.
			row, err = q.DraftByKeyForShare(ctx, problemdb.DraftByKeyForShareParams{Owner: a.SubjectID, RequestKey: key})
		} else if err == nil {
			for _, r := range c.References {
				if err = q.AddDraftMaterial(ctx, problemdb.AddDraftMaterialParams{DraftID: row.ID, LogicalName: r.Name, Sha256: r.Object.SHA256, ByteSize: r.Object.Size}); err != nil {
					return problem.Draft{}, storageError{err}
				}
			}
		}
	}
	if err != nil {
		return problem.Draft{}, storageError{err}
	}
	if row.RequestSha256 != c.RequestSHA256 {
		return problem.Draft{}, problem.ErrConflict
	}
	draft, err := readDraft(ctx, q, row)
	if err != nil {
		return problem.Draft{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		if errors.Is(err, pgx.ErrTxCommitRollback) {
			return problem.Draft{}, storageError{err}
		}
		return problem.Draft{}, errors.Join(problem.ErrCommitUnknown, storageError{err})
	}
	return draft, nil
}

// GetOwn returns the same not-found error for absent and other-owner records.
// No existence query or broad list is issued before the owner predicate.
func (s *Store) GetOwn(ctx context.Context, a identity.ProblemActor, id string) (problem.Draft, error) {
	if ctx == nil || !identity.ValidID(id) {
		return problem.Draft{}, problem.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.begin(ctx, a)
	if err != nil {
		return problem.Draft{}, err
	}
	defer rollback(tx)
	q := problemdb.New(tx)
	row, err := q.DraftByOwner(ctx, problemdb.DraftByOwnerParams{ID: id, Owner: a.SubjectID})
	if errors.Is(err, pgx.ErrNoRows) {
		return problem.Draft{}, problem.ErrNotFound
	}
	if err != nil {
		return problem.Draft{}, storageError{err}
	}
	draft, err := readDraft(ctx, q, row)
	if err != nil {
		return problem.Draft{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return problem.Draft{}, storageError{err}
	}
	return draft, nil
}

func readDraft(ctx context.Context, q *problemdb.Queries, row problemdb.ProblemDraft) (problem.Draft, error) {
	refs, err := q.DraftMaterials(ctx, row.ID)
	if err != nil {
		return problem.Draft{}, storageError{err}
	}
	if !row.CreatedAt.Valid || len(refs) == 0 || row.State != "private-draft" || row.ValidationState != "not-run" {
		return problem.Draft{}, storageError{problem.ErrInvalid}
	}
	d := problem.Draft{ID: row.ID, Owner: row.OwnerSubjectID, RequestKey: row.RequestKey, SourceID: row.SourceID, Version: row.ContentVersion, ImportRevision: row.RequestSha256, State: row.State, ValidationState: row.ValidationState, Document: row.Document, CreatedAt: row.CreatedAt.Time}
	for _, r := range refs {
		d.References = append(d.References, problem.Reference{Name: r.LogicalName, Object: materialstore.Object{SHA256: r.Sha256, Size: r.ByteSize}})
	}
	return d, nil
}
