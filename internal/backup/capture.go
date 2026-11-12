// Package backup captures a logical database archive and its referenced files.
package backup

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/restorecheck"
)

const (
	backupFormat    = "oj.logical-backup.v1"
	maxPackageFiles = 1_000_000
	archiveName     = "postgres.dump"
	inventoryName   = "materials.jsonl"
	manifestName    = "manifest.json"
	judgeDir        = "judge"
	privateDir      = "private"
)

// CaptureConfig selects a single trusted Unix socket endpoint and a new private
// output directory. The operator protects the source files from reclamation.
type CaptureConfig struct {
	Socket, Database, User, Passfile string
	Port                             uint16
	JudgeRoot, PrivateRoot, Output   string
}

// FileIdentity binds an exact output file. File names are fixed by the format.
type FileIdentity struct {
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

// Manifest is written last after both the archive and the inventory are synced.
type Manifest struct {
	Format       string                       `json:"format"`
	Database     string                       `json:"database"`
	CapturedAt   time.Time                    `json:"captured_at"`
	Server       string                       `json:"postgres_version"`
	Archive      FileIdentity                 `json:"archive"`
	Inventory    FileIdentity                 `json:"inventory"`
	JudgeFiles   int64                        `json:"judge_files"`
	PrivateFiles int64                        `json:"private_files"`
	References   restorecheck.MaterialSummary `json:"references"`
}

// Capture exports one read-only snapshot for both pg_dump and the material
// inventory. On pre-seal error it leaves only an incomplete private directory.
// It never changes the source or reuses an output directory.
func Capture(ctx context.Context, cfg CaptureConfig) (Manifest, error) {
	var zero Manifest
	if err := cfg.validate(); err != nil {
		return zero, err
	}
	passfile, err := openPassfile(cfg.Passfile)
	if err != nil {
		return zero, err
	}
	if passfile != nil {
		defer func() { _ = passfile.Close() }()
	}
	conn, err := connect(ctx, cfg, passfile)
	if err != nil {
		return zero, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = conn.Close(cleanup)
	}()
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return zero, errors.New("backup snapshot could not start")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	m := Manifest{Format: backupFormat, Database: cfg.Database}
	var snapshot, actual string
	if err := tx.QueryRow(ctx, `SELECT current_database(), pg_export_snapshot(), statement_timestamp(), current_setting('server_version')`).
		Scan(&actual, &snapshot, &m.CapturedAt, &m.Server); err != nil || actual != cfg.Database {
		return zero, errors.New("backup snapshot or database confirmation failed")
	}
	m.CapturedAt = m.CapturedAt.UTC()
	if err := db.CheckSchema(ctx, tx); err != nil {
		return zero, errors.New("backup requires a compatible application schema in public")
	}
	judge, err := materials.OpenLocal(cfg.JudgeRoot)
	if err != nil {
		return zero, errors.New("backup judge material root unavailable")
	}
	defer func() { _ = judge.Close() }()
	private, err := materialstore.OpenLocal(materialstore.Config{Root: cfg.PrivateRoot, MaxObjectBytes: 128 << 20, MaxInFlight: 1})
	if err != nil {
		return zero, errors.New("backup private material root unavailable")
	}
	defer func() { _ = private.Close() }()
	if err := createOutput(cfg.Output); err != nil {
		return zero, err
	}
	m.Archive, err = captureArchive(ctx, cfg, snapshot, passfile)
	if err != nil {
		return zero, err
	}
	m.Inventory, err = writeIdentity(cfg.Output, inventoryName, func(w io.Writer) error {
		files := materialFiles{output: cfg.Output, index: json.NewEncoder(w)}
		m.References, err = restorecheck.CheckMaterialReferences(ctx, tx,
			func(ctx context.Context, digest string) error {
				return files.copy(judgeDir, digest, -1, func(w io.Writer) (int64, error) {
					return judge.CopyVerified(ctx, digest, w)
				})
			},
			func(ctx context.Context, object materialstore.Object) error {
				return files.copy(privateDir, object.SHA256, object.Size, func(w io.Writer) (int64, error) {
					return object.Size, private.CopyVerified(ctx, object, w)
				})
			})
		m.JudgeFiles, m.PrivateFiles = files.judge, files.private
		return err
	})
	if err != nil {
		return zero, err
	}
	if err := tx.Commit(ctx); err != nil {
		return zero, errors.New("backup snapshot did not complete")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if err := seal(cfg.Output, m); err != nil {
		return zero, err
	}
	return m, nil
}
