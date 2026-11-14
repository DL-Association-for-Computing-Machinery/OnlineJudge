//go:build integration && linux

package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/backup"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/restorecheck"
	submissionpg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

func TestCaptureLogicalBackupMatchesExportedSnapshot(t *testing.T) {
	f, evidence := publicationApprovalFixture(t)
	statement := seedBackupPublication(t, f, evidence)
	archive, schema := dumpSubmissionFixture(t, f.backlogFixture, t.TempDir())
	seeded := newSubmissionRestoreTarget(t, f.ctx, schema, archive)
	_, err := seeded.Exec(f.ctx, `ALTER SCHEMA public RENAME TO original_public`)
	must(t, err)
	_, err = seeded.Exec(f.ctx, `ALTER SCHEMA `+pgx.Identifier{schema}.Sanitize()+` RENAME TO public`)
	must(t, err)
	config := seeded.Config()
	config.ConnConfig.RuntimeParams["search_path"] = "public"
	passfile := requireBackupPassword(t, f.ctx, seeded, config)
	source, err := pgxpool.NewWithConfig(f.ctx, config)
	must(t, err)
	t.Cleanup(source.Close)
	judge, private := t.TempDir(), t.TempDir()
	must(t, os.Chmod(private, 0700))
	must(t, experiment.WriteMaterials(judge))
	for _, ref := range f.draft.References {
		snapshot, err := f.materials.Open(f.ctx, ref.Object)
		must(t, err)
		raw, err := io.ReadAll(snapshot)
		must(t, err)
		must(t, snapshot.Close())
		must(t, os.WriteFile(filepath.Join(private, ref.Object.SHA256), raw, 0600))
	}
	snapshot, err := f.materials.Open(f.ctx, statement)
	must(t, err)
	display, err := io.ReadAll(snapshot)
	must(t, snapshot.Close())
	must(t, err)
	must(t, os.WriteFile(filepath.Join(private, statement.SHA256), display, 0600))
	// A file without a database reference must not be copied into the set.
	orphan := materialHash([]byte("unreferenced judge bytes"))
	must(t, os.WriteFile(filepath.Join(judge, orphan), []byte("unreferenced judge bytes"), 0600))

	bin := t.TempDir()
	ready, proceed := filepath.Join(bin, "snapshot-ready"), filepath.Join(bin, "writer-done")
	dump, err := exec.LookPath("pg_dump")
	must(t, err)
	// Hold the real pg_dump until an independent connection writes after the
	// exporter is open. The restored archive and copied files must exclude it.
	shim := fmt.Sprintf("#!/bin/sh\nset -eu\n: > %q\nwhile [ ! -f %q ]; do sleep 0.01; done\nexec %q \"$@\"\n", ready, proceed, dump)
	must(t, os.WriteFile(filepath.Join(bin, "pg_dump"), []byte(shim), 0700))
	output := filepath.Join(t.TempDir(), "completed")
	ctx, cancel := context.WithTimeout(f.ctx, 12*time.Second)
	defer cancel()
	command := captureBackupCLI(t, ctx, source, judge, private, output, bin+":"+os.Getenv("PATH"), passfile)
	var receipt bytes.Buffer
	command.Stdout, command.Stderr = &receipt, &receipt
	must(t, command.Start())
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	waitBackupSnapshot(t, ctx, ready, done, &receipt)
	const lateVersion = "later-backup-fixture"
	lateBytes := []byte("new file after the exported snapshot\n")
	lateHash := materialHash(lateBytes)
	must(t, os.WriteFile(filepath.Join(judge, lateHash), lateBytes, 0600))
	manifest := strings.ReplaceAll(string(experiment.Manifest()), experiment.Version, lateVersion)
	manifest = strings.Replace(manifest, materialHash([]byte("1 2\n")), lateHash, 1)
	later, err := submissionpg.New(source, lateVersion, []byte(manifest))
	must(t, err)
	must(t, later.RegisterFixture(ctx))
	must(t, os.WriteFile(proceed, nil, 0600))
	if err := <-done; err != nil {
		t.Fatalf("backup command failed: %s (%v)", receipt.String(), err)
	}
	sealed := assertCapturedBackup(t, output, receipt.Bytes(), lateHash, orphan)
	captured, err := os.ReadFile(filepath.Join(output, "private", statement.SHA256))
	if err != nil || !bytes.Equal(captured, display) {
		t.Fatalf("successful backup did not contain the published statement: %v", err)
	}
	var live int
	must(t, source.QueryRow(f.ctx, `SELECT count(*) FROM submission_materials`).Scan(&live))
	if live != 2 {
		t.Fatal("late writer did not modify the live source")
	}
	for _, file := range []string{lateHash, orphan} {
		if _, err := os.Stat(filepath.Join(output, "judge", file)); !os.IsNotExist(err) {
			t.Fatal("backup copied a file outside the snapshot inventory")
		}
	}
	received := t.TempDir()
	must(t, os.CopyFS(received, os.DirFS(output)))
	verifyReceivedBackup(t, f.ctx, received, materialHash(receipt.Bytes()), sealed)
	target := newSubmissionRestoreTarget(t, f.ctx, "public", filepath.Join(received, "postgres.dump"))
	checked, err := restorecheck.VerifyMaterials(f.ctx, target, target.Config().ConnConfig.Database, filepath.Join(received, "judge"), filepath.Join(received, "private"))
	must(t, err)
	if checked != sealed.References || checked.JudgeManifests != 1 || checked.Drafts != 1 || checked.ReferencePlans != 1 {
		t.Fatal("restored archive did not match the sealed material inventory")
	}
	verifyBackupStatementFailures(t, source, target, judge, private, received, passfile, statement.SHA256, display)
	var restoredLate int
	must(t, target.QueryRow(f.ctx, `SELECT count(*) FROM submission_materials WHERE version_id=$1`, lateVersion).Scan(&restoredLate))
	if restoredLate != 0 {
		t.Fatal("pg_dump included a write committed after snapshot export")
	}

	if got, err := captureBackupCLI(t, f.ctx, source, judge, private, output, os.Getenv("PATH"), passfile).CombinedOutput(); err == nil ||
		!strings.Contains(string(got), "backup output must be a new directory") {
		t.Fatal("capture reused an already sealed destination")
	}
	assertCapturedBackup(t, output, receipt.Bytes(), lateHash, orphan)
	must(t, os.Remove(filepath.Join(judge, materialHash([]byte("1 2\n")))))
	failed := filepath.Join(t.TempDir(), "failed")
	if got, err := captureBackupCLI(t, f.ctx, source, judge, private, failed, os.Getenv("PATH"), passfile).CombinedOutput(); err == nil ||
		!strings.Contains(string(got), "failed verification") {
		t.Fatalf("missing file did not fail capture: %q (%v)", got, err)
	}
	if _, err := os.Stat(filepath.Join(failed, "manifest.json")); !os.IsNotExist(err) {
		t.Fatal("failed capture left a completion manifest")
	}
	fifo, oversized := filepath.Join(bin, "not-a-passfile"), filepath.Join(bin, "large-passfile")
	must(t, syscall.Mkfifo(fifo, 0600))
	must(t, os.WriteFile(oversized, make([]byte, (32<<10)+1), 0600))
	for _, invalid := range []string{fifo, oversized} {
		dir := filepath.Join(t.TempDir(), "invalid-passfile")
		got, err := captureBackupCLI(t, f.ctx, source, judge, private, dir, os.Getenv("PATH"), invalid).CombinedOutput()
		if err == nil || !strings.Contains(string(got), "passfile must be an owned 0600 regular file") {
			t.Fatal("invalid passfile was not refused before connecting")
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("invalid passfile created a backup directory")
		}
	}
}

// Use a historical approval fixture; this test exercises backup without
// relaxing the real publication or approval gates.
func seedBackupPublication(t *testing.T, f reviewFixture, e problemreference.ReviewEvidence) materialstore.Object {
	t.Helper()
	_, err := f.access.SetPublisherRole(f.ctx, publisherChange(f.actor.SubjectID, 0, true))
	must(t, err)
	seedPublicationApproval(t, f, e, currentPublicationSnapshot(t, f, e), problemreference.ReviewPolicyVersion)
	draft, err := f.repository.GetOwn(f.ctx, f.actor, e.DraftID)
	must(t, err)
	bound, err := draft.MaterialInventoryIdentity()
	must(t, err)
	var plan problem.ReferencePlan
	must(t, json.Unmarshal(f.want.Encoding, &plan))
	prepared, err := draft.PlanPublicationMaterials(identityID(5511), e.DraftID, f.actor.SubjectID, bound, plan, e.FormalPlanSHA256)
	must(t, err)
	sealed, err := prepared.Seal(f.ctx, f.materials)
	must(t, err)
	display, err := problem.ReadSealedPublicationStatement(f.ctx, f.materials, sealed)
	must(t, err)
	var executionSHA string
	must(t, f.a.QueryRow(f.ctx, `SELECT manifest_sha256 FROM submission_materials WHERE version_id=$1`, experiment.Version).Scan(&executionSHA))
	_, err = f.a.Exec(f.ctx, `INSERT INTO problem_execution_admissions
		(import_revision, formal_plan_sha256, execution_version, execution_sha256) VALUES ($1,$2,$3,$4)`,
		bound.ImportRevision, e.FormalPlanSHA256, experiment.Version, executionSHA)
	must(t, err)
	_, err = f.a.Exec(f.ctx, `INSERT INTO problem_publications
		(problem_id,revision,version,title,statement_sha256,statement_size,import_revision,formal_plan_sha256,
		execution_version,execution_sha256,review_id,assignment_revision,publisher_id,publisher_revision,request_key)
		VALUES ($1,1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,1,'backup-publication')
	`, sealed.ProblemID, sealed.Version, display.Title, sealed.Statement.SHA256, sealed.Statement.Size,
		bound.ImportRevision, e.FormalPlanSHA256, experiment.Version, executionSHA, e.ReviewID, e.AssignmentRevision, f.actor.SubjectID)
	must(t, err)
	return sealed.Statement
}

func verifyBackupStatementFailures(t *testing.T, source, target *pgxpool.Pool, judge, private, received, passfile, digest string, display []byte) {
	t.Helper()
	bad := bytes.Clone(display)
	bad[0] ^= 1 // Keep the length correct so only the digest catches corruption.
	src := filepath.Join(private, digest)
	dst := filepath.Join(received, "private", digest)
	for _, state := range []string{"missing", "corrupt"} {
		if state == "missing" {
			must(t, os.Remove(src))
			must(t, os.Remove(dst))
		} else {
			must(t, os.WriteFile(src, bad, 0600))
			must(t, os.WriteFile(dst, bad, 0600))
		}
		out := filepath.Join(t.TempDir(), state)
		got, err := captureBackupCLI(t, t.Context(), source, judge, private, out, os.Getenv("PATH"), passfile).CombinedOutput()
		if err == nil || !strings.Contains(string(got), "published statement:") {
			t.Fatalf("%s statement did not fail capture: %s (%v)", state, got, err)
		}
		if _, err := os.Stat(filepath.Join(out, "manifest.json")); !os.IsNotExist(err) {
			t.Fatalf("%s statement left a completion manifest", state)
		}
		_, err = restorecheck.VerifyMaterials(t.Context(), target, target.Config().ConnConfig.Database,
			filepath.Join(received, "judge"), filepath.Join(received, "private"))
		if err == nil || !strings.Contains(err.Error(), "published statement:") {
			t.Fatalf("%s statement did not fail restore verification: %v", state, err)
		}
	}
	must(t, os.WriteFile(src, display, 0600))
	must(t, os.WriteFile(dst, display, 0600))
}

func verifyReceivedBackup(t *testing.T, ctx context.Context, received, trustedDigest string, manifest backup.Manifest) {
	t.Helper()
	migrate, err := filepath.Abs("../../.local/bin/migrate")
	must(t, err)
	cmd := exec.CommandContext(ctx, migrate, "verify-backup", "-manifest-sha256", trustedDigest, "-timeout", "5s", received)
	cmd.Env = []string{"PATH=/no-helper-programs", "OJ_DATABASE_URL=invalid", "OJ_RESTORED_DATABASE_URL=invalid"}
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("received backup rejected before database restore: %s (%v)", raw, err)
	}
	var got backup.TransferReceipt
	must(t, json.Unmarshal(raw, &got))
	if got.ManifestSHA256 != trustedDigest || got.Archive != manifest.Archive ||
		got.JudgeFiles != manifest.JudgeFiles || got.PrivateFiles != manifest.PrivateFiles {
		t.Fatal("transfer receipt did not bind the same sealed backup")
	}
}

func requireBackupPassword(t *testing.T, ctx context.Context, seeded *pgxpool.Pool, config *pgxpool.Config) string {
	t.Helper()
	password := rand.Text()
	// The random ASCII value and role are confined to this disposable cluster.
	var command string
	must(t, seeded.QueryRow(ctx, `SELECT format('ALTER ROLE %I PASSWORD %L',$1::text,$2::text)`, config.ConnConfig.User, password).Scan(&command))
	_, err := seeded.Exec(ctx, command)
	must(t, err)
	var loaded time.Time
	must(t, seeded.QueryRow(ctx, `SELECT pg_conf_load_time()`).Scan(&loaded))
	must(t, os.WriteFile(filepath.Join(config.ConnConfig.Host, "data", "pg_hba.conf"), []byte("local all all scram-sha-256\n"), 0600))
	var reloaded bool
	must(t, seeded.QueryRow(ctx, `SELECT pg_reload_conf()`).Scan(&reloaded))
	if !reloaded {
		t.Fatal("could not enable password authentication for the disposable source")
	}
	for until := time.Now().Add(2 * time.Second); ; {
		var ready bool
		must(t, seeded.QueryRow(ctx, `SELECT pg_conf_load_time()>$1`, loaded).Scan(&ready))
		if ready {
			break
		}
		if time.Now().After(until) {
			t.Fatal("source did not reload the test authentication configuration")
		}
		time.Sleep(10 * time.Millisecond)
	}
	without, err := pgx.ConnectConfig(ctx, config.ConnConfig.Copy())
	if err == nil {
		must(t, without.Close(ctx))
		t.Fatal("disposable source did not require a password")
	}
	config.ConnConfig.Password = password
	path := filepath.Join(t.TempDir(), "backup-passfile")
	must(t, os.WriteFile(path, []byte(fmt.Sprintf("*:%d:%s:%s:%s\n", config.ConnConfig.Port, config.ConnConfig.Database, config.ConnConfig.User, password)), 0600))
	return path
}

func captureBackupCLI(t *testing.T, ctx context.Context, pool *pgxpool.Pool, judge, private, output, path, passfile string) *exec.Cmd {
	t.Helper()
	migrate, err := filepath.Abs("../../.local/bin/migrate")
	must(t, err)
	c := pool.Config().ConnConfig
	cmd := exec.CommandContext(ctx, migrate, "capture-backup", "-socket", c.Host, "-port", strconv.Itoa(int(c.Port)), "-user", c.User,
		"-timeout", "10s", "-passfile", passfile, c.Database, judge, private, output)
	cmd.Env = []string{"PATH=" + path, "PGHOST=/ignored-socket", "PGDATABASE=ignored-database", "PGUSER=ignored-user", "PGPASSWORD=ignored-password",
		"PGPASSFILE=/ignored-passfile", "PGOPTIONS=-c search_path=ignored_schema"}
	return cmd
}

func waitBackupSnapshot(t *testing.T, ctx context.Context, ready string, done <-chan error, receipt *bytes.Buffer) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(ready); err == nil {
			return
		}
		select {
		case err := <-done:
			t.Fatalf("backup exited before invoking pg_dump: %s (%v)", receipt.String(), err)
		case <-ctx.Done():
			t.Fatal("pg_dump did not observe the exported snapshot")
		case <-ticker.C:
		}
	}
}

func assertCapturedBackup(t *testing.T, root string, receipt []byte, excluded ...string) backup.Manifest {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	must(t, err)
	if !bytes.Equal(raw, receipt) {
		t.Fatal("published manifest and successful command receipt differ")
	}
	var m backup.Manifest
	must(t, json.Unmarshal(raw, &m))
	if m.Format != "oj.logical-backup.v1" || m.CapturedAt.IsZero() || m.Archive.Bytes == 0 || m.JudgeFiles != 4 || m.PrivateFiles == 0 {
		t.Fatalf("unexpected backup manifest: %+v", m)
	}
	for name, expected := range map[string]backup.FileIdentity{"postgres.dump": m.Archive, "materials.jsonl": m.Inventory} {
		body, err := os.ReadFile(filepath.Join(root, name))
		must(t, err)
		if int64(len(body)) != expected.Bytes || materialHash(body) != expected.SHA256 {
			t.Fatal("backup file disagrees with published identity")
		}
	}
	index, err := os.Open(filepath.Join(root, "materials.jsonl"))
	must(t, err)
	defer func() { must(t, index.Close()) }()
	decoder := json.NewDecoder(index)
	seen := make(map[string]bool)
	for {
		var row struct {
			Store, SHA256 string
			Bytes         int64
		}
		if err := decoder.Decode(&row); err == io.EOF {
			break
		} else {
			must(t, err)
		}
		for _, digest := range excluded {
			if row.SHA256 == digest {
				t.Fatal("inventory listed an unreferenced file")
			}
		}
		key := filepath.Join(row.Store, row.SHA256)
		body, err := os.ReadFile(filepath.Join(root, key))
		must(t, err)
		if seen[key] || int64(len(body)) != row.Bytes || materialHash(body) != row.SHA256 {
			t.Fatal("material inventory did not describe unique verified files")
		}
		seen[key] = true
	}
	if int64(len(seen)) != m.JudgeFiles+m.PrivateFiles {
		t.Fatal("manifest file counts disagree with inventory")
	}
	return m
}
