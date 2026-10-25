// Package postgres adapts submission acceptance to a real PostgreSQL transaction.
package postgres

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres/submissiondb"
)

// Store permits only the explicitly registered immutable M0 fixture.
// Product version authorization belongs to P3/P4, before exposing training APIs.
type Store struct {
	pool                  *pgxpool.Pool
	version, manifestHash string
	manifest              []byte
}

// New binds the store to a canonical JSON material manifest.
func New(pool *pgxpool.Pool, version string, manifest []byte) (*Store, error) {
	canonical, hash, err := canonicalManifest(manifest)
	if err != nil || version == "" {
		return nil, submission.ErrMaterial
	}
	return &Store{pool: pool, version: version, manifestHash: hash, manifest: canonical}, nil
}

func canonicalManifest(data []byte) ([]byte, string, error) {
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, "", err
	}
	if value == nil {
		return nil, "", submission.ErrMaterial
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(canonical)
	return canonical, hex.EncodeToString(sum[:]), nil
}

// RegisterFixture is an explicit setup operation, never called by API startup.
// An existing version cannot be overwritten with different material.
func (s *Store) RegisterFixture(ctx context.Context) error {
	q := submissiondb.New(s.pool)
	if err := q.RegisterMaterial(ctx, submissiondb.RegisterMaterialParams{
		VersionID: s.version, ManifestSha256: s.manifestHash, Manifest: s.manifest,
	}); err != nil {
		return err
	}
	return s.checkMaterial(ctx, q)
}

func (s *Store) checkMaterial(ctx context.Context, q *submissiondb.Queries) error {
	row, err := q.GetMaterial(ctx, s.version)
	if errors.Is(err, pgx.ErrNoRows) {
		return submission.ErrMaterial
	}
	if err != nil {
		return err
	}
	_, actual, err := canonicalManifest(row.Manifest)
	if err != nil || row.ManifestSha256 != s.manifestHash || actual != s.manifestHash {
		return submission.ErrMaterial
	}
	return nil
}

// Accept atomically persists source and task, or returns the original receipt.
// A commit error remains ambiguous: callers must query/retry with the same key.
func (s *Store) Accept(ctx context.Context, subject, scope, key string, req submission.Request) (submission.Receipt, bool, error) {
	if err := submission.Validate(subject, scope, key, req); err != nil {
		return submission.Receipt{}, false, err
	}
	if req.MaterialVersion != s.version {
		return submission.Receipt{}, false, submission.ErrMaterial
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return submission.Receipt{}, false, err
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	q := submissiondb.New(tx)
	receiptKey := submissiondb.GetReceiptParams{SubjectID: subject, ContextID: scope, IdempotencyKey: key}
	fingerprint := submission.Digest(req)
	// Return historical acceptance even if the material store later becomes unavailable.
	existing, err := q.GetReceipt(ctx, receiptKey)
	if err == nil {
		if existing.RequestSha256 != fingerprint {
			return submission.Receipt{}, false, submission.ErrConflict
		}
		return receipt(existing), false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return submission.Receipt{}, false, err
	}
	if err := s.checkMaterial(ctx, q); err != nil {
		return submission.Receipt{}, false, err
	}
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return submission.Receipt{}, false, err
	}
	id, err := q.CreateSubmission(ctx, submissiondb.CreateSubmissionParams{
		ID: hex.EncodeToString(idBytes), SubjectID: subject, ContextID: scope, IdempotencyKey: key,
		RequestSha256: fingerprint, MaterialVersion: req.MaterialVersion,
		ManifestSha256: s.manifestHash, Language: req.Language, SourceCode: req.Source,
	})
	created := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return submission.Receipt{}, false, err
	}
	if created {
		if err := q.CreateTask(ctx, id); err != nil {
			return submission.Receipt{}, false, err
		}
	}
	// A separate statement under READ COMMITTED sees the winning transaction
	// after ON CONFLICT waits. A single CTE snapshot would miss that row.
	row, err := q.GetReceipt(ctx, receiptKey)
	if err != nil {
		return submission.Receipt{}, false, err
	}
	if row.RequestSha256 != fingerprint {
		return submission.Receipt{}, false, submission.ErrConflict
	}
	if err := tx.Commit(ctx); err != nil {
		return submission.Receipt{}, false, err
	}
	return receipt(row), created, nil
}

// Receipt resolves a lost acceptance response within the original identity/context.
func (s *Store) Receipt(ctx context.Context, subject, scope, key string) (submission.Receipt, error) {
	if !submission.ValidKey(key) {
		return submission.Receipt{}, submission.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	row, err := submissiondb.New(s.pool).GetReceipt(ctx, submissiondb.GetReceiptParams{
		SubjectID: subject, ContextID: scope, IdempotencyKey: key,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return submission.Receipt{}, submission.ErrNotFound
	}
	if err != nil {
		return submission.Receipt{}, err
	}
	return receipt(row), nil
}

// Get enforces ownership even when a submission identifier is known.
func (s *Store) Get(ctx context.Context, subject, scope, id string) (submission.Receipt, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	row, err := submissiondb.New(s.pool).GetSubmission(ctx, submissiondb.GetSubmissionParams{
		ID: id, SubjectID: subject, ContextID: scope,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return submission.Receipt{}, submission.ErrNotFound
	}
	if err != nil {
		return submission.Receipt{}, err
	}
	return receipt(submissiondb.GetReceiptRow(row)), nil
}

func receipt(row submissiondb.GetReceiptRow) submission.Receipt {
	result := submission.Receipt{ID: row.ID, MaterialVersion: row.MaterialVersion,
		Language: row.Language, CreatedAt: row.CreatedAt.Time.UTC(), State: row.State}
	if row.Verdict.Valid {
		result.Verdict = &row.Verdict.String
	}
	return result
}
