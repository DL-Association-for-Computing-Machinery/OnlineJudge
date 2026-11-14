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
	_, err := f.provider.DownTo(f.ctx, 29)
	must(t, err)
	var check *pgconn.PgError
	if err := insert(submission.LanguageC17); !errors.As(err, &check) || check.Code != "23514" {
		t.Fatalf("schema 29 did not reject C17: %v", err)
	}
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	must(t, insert(submission.LanguageC17))
	if _, err := f.provider.DownTo(f.ctx, 30); err != nil {
		t.Fatalf("C17 history must allow reverting only GNU++20: %v", err)
	}
	if _, err := f.provider.Down(f.ctx); err == nil {
		t.Fatal("rollback removed C17 constraint support while history exists")
	}
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	must(t, db.CheckSchema(f.ctx, f.a))
	var language string
	must(t, f.a.QueryRow(f.ctx, "SELECT language FROM submissions WHERE id=$1", "c17-migration-test").Scan(&language))
	if language != submission.LanguageC17 {
		t.Fatal("blocked rollback did not preserve language")
	}
}

func TestGNUCPP20MigrationKeepsHistoryOnBlockedDown(t *testing.T) {
	f := newBacklogFixture(t)
	_, err := f.provider.DownTo(f.ctx, 30)
	must(t, err)
	insert := func() error {
		_, err := f.a.Exec(f.ctx, `INSERT INTO submissions
			(id, subject_id, context_id, idempotency_key, request_sha256, material_version, manifest_sha256, language, source_code)
			SELECT 'gnu20-migration-test', 'synthetic', 'personal-training', 'gnu20-migration-key',
			       repeat('a',64), version_id, manifest_sha256, $1, 'int main(){}'
			FROM submission_materials WHERE version_id=$2`, submission.LanguageGNUCPP20, f.request.MaterialVersion)
		return err
	}
	var check *pgconn.PgError
	if err := insert(); !errors.As(err, &check) || check.Code != "23514" {
		t.Fatalf("schema 30 did not reject GNU++20: %v", err)
	}
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	must(t, insert())
	if _, err := f.provider.DownTo(f.ctx, 30); err == nil {
		t.Fatal("rollback removed GNU++20 support while history exists")
	}
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	must(t, db.CheckSchema(f.ctx, f.a))
	var language string
	must(t, f.a.QueryRow(f.ctx, "SELECT language FROM submissions WHERE id=$1", "gnu20-migration-test").Scan(&language))
	if language != submission.LanguageGNUCPP20 {
		t.Fatal("blocked rollback did not preserve language")
	}
}

func TestJava21MigrationKeepsHistoryOnBlockedDown(t *testing.T) {
	testPublishedLanguageDown(t, submission.LanguageJava21, 31)
}

func TestPython3MigrationKeepsHistoryOnBlockedDown(t *testing.T) {
	testPublishedLanguageDown(t, submission.LanguagePython3, 32)
}

func testPublishedLanguageDown(t *testing.T, language string, oldVersion int64) {
	t.Helper()
	f := newBacklogFixture(t)
	_, err := f.provider.DownTo(f.ctx, oldVersion)
	must(t, err)
	insert := func() error {
		_, err := f.a.Exec(f.ctx, `INSERT INTO submissions
			(id, subject_id, context_id, idempotency_key, request_sha256, material_version, manifest_sha256, language, source_code)
			SELECT 'language-migration-test', 'synthetic', 'personal-training', 'language-migration-key',
			       repeat('a',64), version_id, manifest_sha256, $1, 'synthetic source'
			FROM submission_materials WHERE version_id=$2`, language, f.request.MaterialVersion)
		return err
	}
	var check *pgconn.PgError
	if err := insert(); !errors.As(err, &check) || check.Code != "23514" {
		t.Fatalf("schema %d did not reject %s: %v", oldVersion, language, err)
	}
	_, err = f.provider.UpTo(f.ctx, oldVersion+1)
	must(t, err)
	must(t, insert())
	if _, err := f.provider.Down(f.ctx); err == nil {
		t.Fatalf("rollback removed %s while history exists", language)
	}
	version, err := f.provider.GetDBVersion(f.ctx)
	must(t, err)
	if version != oldVersion+1 {
		t.Fatalf("blocked rollback changed schema from %d to %d", oldVersion+1, version)
	}
	var stored string
	must(t, f.a.QueryRow(f.ctx, "SELECT language FROM submissions WHERE id=$1", "language-migration-test").Scan(&stored))
	if stored != language {
		t.Fatal("blocked rollback did not preserve language")
	}
}
