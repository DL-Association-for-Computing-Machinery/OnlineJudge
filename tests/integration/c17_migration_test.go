//go:build integration

package integration_test

import (
	"errors"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestC17MigrationKeepsHistoryOnBlockedDown(t *testing.T) {
	f := newBacklogFixture(t)
	insert := func(language string) error {
		_, err := f.a.Exec(f.ctx, `INSERT INTO submissions
			(id, subject_id, context_id, idempotency_key, request_sha256, material_version, manifest_sha256, language, source_code)
			SELECT 'c17-migration-test','synthetic','personal-training','c17-migration-key',
			       repeat('a',64), version_id, manifest_sha256, $1,'int main(void){return 0;}'
			FROM submission_materials WHERE version_id=$2`, language, f.request.MaterialVersion)
		return err
	}
	_, err := f.provider.Down(f.ctx)
	must(t, err)
	var check *pgconn.PgError
	if err := insert(submission.LanguageC17); !errors.As(err, &check) || check.Code != "23514" {
		t.Fatalf("schema 29 did not reject C17: %v", err)
	}
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	must(t, insert(submission.LanguageC17))
	if _, err := f.provider.Down(f.ctx); err == nil {
		t.Fatal("rollback removed C17 constraint support while history exists")
	}
	must(t, db.CheckSchema(f.ctx, f.a))
	var language string
	must(t, f.a.QueryRow(f.ctx, "SELECT language FROM submissions WHERE id=$1", "c17-migration-test").Scan(&language))
	if language != submission.LanguageC17 {
		t.Fatal("blocked rollback did not preserve language")
	}
}
