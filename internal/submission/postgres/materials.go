package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres/submissiondb"
	"github.com/jackc/pgx/v5"
)

// Materials locks authorization records, then checks expiry using database time.
// No filesystem or network I/O happens while the transaction holds these locks.
func (s *Store) Materials(ctx context.Context, node submission.NodeIdentity, token string) (submission.TaskMaterials, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return submission.TaskMaterials{}, err
	}
	defer rollback(tx)
	q := submissiondb.New(tx)
	if _, err := authorizeNode(ctx, q, node); err != nil {
		return submission.TaskMaterials{}, err
	}
	attempt, err := q.LockAttempt(ctx, submissiondb.LockAttemptParams{Token: token, NodeID: node.ID, CredentialVersion: node.CredentialVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return submission.TaskMaterials{}, submission.ErrLease
	}
	if err != nil {
		return submission.TaskMaterials{}, err
	}
	if attempt.State != "leased" || attempt.TaskState != "running" || attempt.AttemptNumber != attempt.CurrentAttempt {
		return submission.TaskMaterials{}, submission.ErrLease
	}
	row, err := q.GetLeaseMaterials(ctx, token)
	if errors.Is(err, pgx.ErrNoRows) {
		return submission.TaskMaterials{}, submission.ErrLease
	}
	if err != nil {
		return submission.TaskMaterials{}, err
	}
	manifest, hash, err := canonicalManifest(row.Manifest)
	if err != nil || row.MaterialVersion != s.version || hash != row.ManifestSha256 || hash != s.manifestHash {
		return submission.TaskMaterials{}, submission.ErrMaterial
	}
	if err := tx.Commit(ctx); err != nil {
		return submission.TaskMaterials{}, err
	}
	return submission.TaskMaterials{SubmissionID: row.ID, Version: row.MaterialVersion, ManifestSHA256: hash,
		Manifest: manifest, Language: row.Language, Source: row.SourceCode}, nil
}
