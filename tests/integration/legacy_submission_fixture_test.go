//go:build integration

package integration_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

// acceptLegacySubmission creates synthetic pre-upgrade data, not an admission or idempotency implementation.
func acceptLegacySubmission(ctx context.Context, pool *pgxpool.Pool, subject, scope, key string, req submission.Request) (submission.Receipt, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := submission.Validate(subject, scope, key, req); err != nil {
		return submission.Receipt{}, false, err
	}
	fixtures := map[string]struct{ key, source string }{
		"synthetic-pre-identity":      {"identity-upgrade-existing-01", "// synthetic v2 retained source"},
		"synthetic-pre-recovery":      {"recovery-upgrade-existing-01", "// synthetic retained version-three source"},
		"synthetic-recovery-rollback": {"recovery-rollback-sub-only-01", "// synthetic submission-only rollback fixture"},
		"synthetic-pre-email":         {"personal-upgrade-existing-01", "// synthetic retained email migration source"},
	}
	fixture, ok := fixtures[subject]
	if !ok || key != fixture.key || req.Source != fixture.source || scope != "training" || req.MaterialVersion != experiment.Version {
		return submission.Receipt{}, false, errors.New("legacy fixture requires synthetic data")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return submission.Receipt{}, false, err
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	var version int64
	var policyAbsent bool
	if err = tx.QueryRow(ctx, `SELECT (SELECT version_id FROM goose_db_version ORDER BY id DESC LIMIT 1), to_regclass('submission_backlog_policy') IS NULL`).Scan(&version, &policyAbsent); err != nil {
		return submission.Receipt{}, false, err
	}
	if version >= 6 || !policyAbsent {
		return submission.Receipt{}, false, errors.New("legacy fixture requires schema before six without backlog policy")
	}
	idBytes := make([]byte, 16)
	if _, err = rand.Read(idBytes); err != nil {
		return submission.Receipt{}, false, err
	}
	receipt := submission.Receipt{ID: hex.EncodeToString(idBytes), MaterialVersion: req.MaterialVersion, Language: req.Language, State: "queued"}
	err = tx.QueryRow(ctx, `INSERT INTO submissions (id, subject_id, context_id, idempotency_key, request_sha256, material_version, manifest_sha256, language, source_code)
		SELECT $1, $2, $3, $4, $5, version_id, manifest_sha256, $6, $7 FROM submission_materials WHERE version_id=$8 RETURNING created_at`,
		receipt.ID, subject, scope, key, submission.Digest(req), req.Language, req.Source, req.MaterialVersion).Scan(&receipt.CreatedAt)
	if err != nil {
		return submission.Receipt{}, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO judge_tasks (submission_id) VALUES ($1)`, receipt.ID); err != nil {
		return submission.Receipt{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return submission.Receipt{}, false, err
	}
	return receipt, true, nil
}
