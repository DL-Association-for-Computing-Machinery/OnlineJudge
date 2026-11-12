//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRestoreSubmissionAndMaterials(t *testing.T) {
	f := newBacklogFixture(t)
	backlogPolicy(t, f, 2, 2)
	const subject, scope = "restore-fixture", "m0-training"
	const historyKey, queuedKey = "restore-history-key", "restore-queued-key"
	node := submission.NodeIdentity{ID: "restore-fixture-node", CredentialVersion: 1}
	_, err := f.a.Exec(f.ctx, `INSERT INTO worker_nodes(id, credential_version) VALUES($1, 1)`, node.ID)
	must(t, err)
	historyRequest := f.request
	historyRequest.Source = "// 合成历史源码\r\n// restored byte for byte 😀\n"
	history, created, err := f.first.Accept(f.ctx, subject, scope, historyKey, historyRequest)
	must(t, err)
	if !created {
		t.Fatal("historical fixture was not accepted")
	}
	lease, err := f.first.Claim(f.ctx, node)
	must(t, err)
	if lease.SubmissionID != history.ID {
		t.Fatal("claimed the wrong historical fixture")
	}
	must(t, f.first.Report(f.ctx, node, lease.Token, submission.Result{Verdict: "AC", EvidenceSHA256: materialHash([]byte("synthetic historical result"))}))
	history, err = f.first.Get(f.ctx, subject, scope, history.ID)
	must(t, err)
	if history.Verdict == nil || *history.Verdict != "AC" {
		t.Fatal("fixture has no historical AC result")
	}
	queued, created, err := f.first.Accept(f.ctx, subject, scope, queuedKey, f.request)
	must(t, err)
	if !created {
		t.Fatal("queued fixture was not accepted")
	}

	// The fixture is quiescent between the database dump and the blob copy.
	sourceRoot, backupRoot, restoreRoot := t.TempDir(), t.TempDir(), t.TempDir()
	must(t, experiment.WriteMaterials(sourceRoot))
	archive := filepath.Join(backupRoot, "schema.dump")
	var schema string
	must(t, f.a.QueryRow(f.ctx, `SELECT current_schema()`).Scan(&schema))
	source := f.a.Config().ConnConfig
	must(t, restoreCommand(f.ctx, "pg_dump", "--host", source.Host, "--port", strconv.Itoa(int(source.Port)), "--username", source.User,
		"--dbname", source.Database, "--no-password", "--format=custom", "--strict-names", "--schema", schema, "--file", archive))
	entries, err := os.ReadDir(sourceRoot)
	must(t, err)
	for _, entry := range entries {
		copyRestoreFixtureFile(t, sourceRoot, backupRoot, entry.Name())
	}

	target := newSubmissionRestoreTarget(t, f.ctx, schema, archive)
	must(t, db.CheckSchema(f.ctx, target))
	restored, err := postgres.New(target, experiment.Version, experiment.Manifest())
	must(t, err)
	// No migrations, fixture registration or submission seeding on the target.
	replayed, created, err := restored.Accept(f.ctx, subject, scope, historyKey, historyRequest)
	must(t, err)
	if created || !reflect.DeepEqual(replayed, history) {
		t.Fatal("historical receipt or idempotency changed after restore")
	}
	var historySource string
	must(t, target.QueryRow(f.ctx, `SELECT source_code FROM submissions WHERE id=$1`, history.ID).Scan(&historySource))
	if historySource != historyRequest.Source {
		t.Fatal("historical source bytes changed after restore")
	}
	lease, err = restored.Claim(f.ctx, node)
	must(t, err)
	if lease.SubmissionID != queued.ID || lease.AttemptNumber != 1 {
		t.Fatal("restored queue did not retain its unclaimed submission")
	}

	reader, err := materials.OpenLocal(restoreRoot)
	must(t, err)
	t.Cleanup(func() { must(t, reader.Close()) })
	service := materials.New(restored, reader)
	descriptor, err := service.Describe(f.ctx, node, lease.Token)
	must(t, err)
	if descriptor.SubmissionID != queued.ID || descriptor.Source != f.request.Source || descriptor.SourceSHA256 != materialHash([]byte(f.request.Source)) ||
		!bytes.Equal(descriptor.Manifest, experiment.Manifest()) || descriptor.ManifestSHA256 != materialHash(experiment.Manifest()) {
		t.Fatal("restored assignment changed source, manifest or their digests")
	}
	assertUnavailable := func() {
		t.Helper()
		file, err := service.Download(f.ctx, node, lease.Token, "01", "input")
		if file != nil {
			must(t, file.Close())
			t.Fatal("invalid restore exposed material bytes")
		}
		if !errors.Is(err, materials.ErrUnavailable) {
			t.Fatalf("expected material unavailable, got %v", err)
		}
	}
	assertUnavailable()
	must(t, os.WriteFile(filepath.Join(restoreRoot, materialHash([]byte("1 2\n"))), []byte("2 1\n"), 0600))
	assertUnavailable()
	for _, entry := range entries {
		copyRestoreFixtureFile(t, backupRoot, restoreRoot, entry.Name())
	}
	for _, want := range []struct{ testID, kind, body string }{
		{"01", "input", "1 2\n"}, {"01", "answer", "3\n"}, {"02", "input", "-7 4\n"}, {"02", "answer", "-3\n"},
	} {
		file, err := service.Download(f.ctx, node, lease.Token, want.testID, want.kind)
		must(t, err)
		body, readErr := io.ReadAll(file)
		must(t, file.Close())
		must(t, readErr)
		if string(body) != want.body || file.SHA256 != materialHash([]byte(want.body)) || file.Size != int64(len(want.body)) {
			t.Fatalf("restored %s for test %s differs from fixture", want.kind, want.testID)
		}
	}
	t.Log("restored historical AC/source, original receipt, queued submission, and all four manifest blobs; rejected missing and corrupt input")
}

func copyRestoreFixtureFile(t *testing.T, from, to, name string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(from, name))
	must(t, err)
	must(t, os.WriteFile(filepath.Join(to, name), data, 0600))
}

func newSubmissionRestoreTarget(t *testing.T, ctx context.Context, schema, archive string) *pgxpool.Pool {
	t.Helper()
	// A separate, socket-only cluster cannot serve the restored query from the source DB.
	root, err := os.MkdirTemp("/tmp", "oj-restore-")
	must(t, err)
	data, log := filepath.Join(root, "data"), filepath.Join(root, "postgres.log")
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := os.Stat(filepath.Join(data, "postmaster.pid")); err == nil {
			must(t, restoreCommand(cleanup, "pg_ctl", "-D", data, "-m", "fast", "-w", "stop"))
		} else if !os.IsNotExist(err) {
			must(t, err)
		}
		must(t, os.RemoveAll(root))
	})
	must(t, restoreCommand(ctx, "initdb", "-D", data, "--username=oj_test", "--auth-local=trust", "--auth-host=reject", "--no-locale", "--encoding=UTF8"))
	must(t, restoreCommand(ctx, "pg_ctl", "-D", data, "-l", log, "-o", "-k "+root+" -p 5432 -c listen_addresses=''", "-w", "start"))
	must(t, restoreCommand(ctx, "pg_restore", "--host", root, "--port", "5432", "--username", "oj_test", "--dbname", "postgres",
		"--no-password", "--single-transaction", "--no-owner", "--no-privileges", archive))
	cfg, err := pgxpool.ParseConfig("host=" + root + " port=5432 user=oj_test dbname=postgres sslmode=disable")
	must(t, err)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	must(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func restoreCommand(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	// Ignore ambient PG* options and service files. Endpoints are set explicitly.
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "LC_ALL=C"}
	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.New(name + ": " + err.Error() + ": " + string(output))
	}
	return nil
}
