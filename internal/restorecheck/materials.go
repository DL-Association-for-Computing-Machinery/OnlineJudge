// Package restorecheck verifies materials against an isolated restored database.
package restorecheck

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	submissionpg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

const recentLimit = 4096

// MaterialSummary counts checked inventories and references, including duplicate
// references. No source, paths, private document or object bytes are returned.
type MaterialSummary struct {
	JudgeManifests    int64 `json:"judge_manifests"`
	JudgeReferences   int64 `json:"judge_references"`
	Drafts            int64 `json:"drafts"`
	ReferencePlans    int64 `json:"reference_plans"`
	DifferentialPlans int64 `json:"differential_plans"`
	PrivateReferences int64 `json:"private_references"`
}

// VerifyMaterials checks a repeatable, read-only snapshot of the explicitly
// named database and streams the matching files. Both storage roots and the
// database must be isolated from writers throughout the check.
func VerifyMaterials(ctx context.Context, pool *pgxpool.Pool, database, judgeRoot, privateRoot string) (MaterialSummary, error) {
	var zero MaterialSummary
	if ctx == nil || pool == nil || database == "" || !absoluteClean(judgeRoot) || !absoluteClean(privateRoot) {
		return zero, errors.New("invalid restored material configuration")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return zero, errors.New("restored material database unavailable")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	var matches bool
	if err := tx.QueryRow(ctx, `SELECT current_database()::text=$1`, database).Scan(&matches); err != nil || !matches {
		return zero, errors.New("restored material database name did not match confirmation")
	}
	if err := db.CheckSchema(ctx, tx); err != nil {
		return zero, errors.New("restored material database schema check failed")
	}
	judge, err := materials.OpenLocal(judgeRoot)
	if err != nil {
		return zero, errors.New("restored judge material root unavailable")
	}
	defer func() { _ = judge.Close() }()
	private, err := materialstore.OpenLocal(materialstore.Config{Root: privateRoot, MaxObjectBytes: 128 << 20, MaxInFlight: 1})
	if err != nil {
		return zero, errors.New("restored private material root unavailable")
	}
	defer func() { _ = private.Close() }()
	var summary MaterialSummary
	if err := verifyJudgeMaterials(ctx, tx, judge, &summary); err != nil {
		return zero, err
	}
	objects := privateVerifier{store: private, recent: make(map[materialstore.Object]struct{})}
	if err := verifyDrafts(ctx, tx, &objects, &summary); err != nil {
		return zero, err
	}
	if err := verifyPlans(ctx, tx, &objects, &summary); err != nil {
		return zero, err
	}
	if err := tx.Commit(ctx); err != nil {
		return zero, errors.New("restored material database read could not complete")
	}
	return summary, nil
}

func absoluteClean(path string) bool { return filepath.IsAbs(path) && path == filepath.Clean(path) }

func verifyJudgeMaterials(ctx context.Context, tx pgx.Tx, local *materials.Local, summary *MaterialSummary) error {
	rows, err := tx.Query(ctx, `SELECT version_id, manifest_sha256,
		CASE WHEN octet_length(manifest::text)<=32768 THEN manifest END FROM submission_materials`)
	if err != nil {
		return errors.New("restored judge material inventory unavailable")
	}
	defer rows.Close()
	recent := make(map[string]struct{})
	for rows.Next() {
		var version, digest string
		var raw []byte
		if err := rows.Scan(&version, &digest, &raw); err != nil {
			return errors.New("restored judge material inventory invalid")
		}
		canonical, _, err := submissionpg.CanonicalManifest(raw)
		if err != nil {
			return errors.New("restored judge manifest invalid")
		}
		files, err := materials.ManifestDigests(version, digest, canonical)
		if err != nil {
			return errors.New("restored judge manifest invalid")
		}
		for _, file := range files {
			if _, ok := recent[file]; !ok {
				if err := local.Verify(ctx, file); err != nil {
					return fmt.Errorf("restored judge file %s failed verification", file)
				}
				remember(recent, file)
			}
			summary.JudgeReferences++
		}
		summary.JudgeManifests++
	}
	if rows.Err() != nil {
		return errors.New("restored judge material inventory unavailable")
	}
	return nil
}

type privateVerifier struct {
	store  *materialstore.Local
	recent map[materialstore.Object]struct{}
}

func (v *privateVerifier) check(ctx context.Context, object materialstore.Object, summary *MaterialSummary) error {
	if !problem.ValidImportRevision(object.SHA256) {
		return errors.New("restored private object identity invalid")
	}
	if _, ok := v.recent[object]; !ok {
		if err := v.store.Verify(ctx, object); err != nil {
			return fmt.Errorf("restored private object %s failed verification", object.SHA256)
		}
		remember(v.recent, object)
	}
	summary.PrivateReferences++
	return nil
}

// A bounded recent cache avoids repeated I/O without keeping the full inventory.
func remember[K comparable](recent map[K]struct{}, key K) {
	if len(recent) == recentLimit {
		clear(recent)
	}
	recent[key] = struct{}{}
}
