package backup

import (
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
)

var namePattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]{0,62}$`)

func (cfg CaptureConfig) validate() error {
	if cfg.Port == 0 || !namePattern.MatchString(cfg.Database) || !namePattern.MatchString(cfg.User) ||
		!absoluteClean(cfg.Socket) || strings.Contains(cfg.Socket, ",") || (cfg.Passfile != "" && !absoluteClean(cfg.Passfile)) ||
		!absoluteClean(cfg.JudgeRoot) || !absoluteClean(cfg.PrivateRoot) || !absoluteClean(cfg.Output) {
		return errors.New("invalid backup configuration; use an explicit Unix socket and absolute paths")
	}
	return checkOutput(cfg.Output, cfg.JudgeRoot, cfg.PrivateRoot)
}

func absoluteClean(path string) bool { return filepath.IsAbs(path) && filepath.Clean(path) == path }

func (cfg CaptureConfig) port() string { return strconv.FormatUint(uint64(cfg.Port), 10) }

func openPassfile(path string) (*os.File, error) {
	if path == "" {
		return nil, nil
	}
	const invalid = "backup passfile must be an owned 0600 regular file up to 32 KiB with /proc available"
	// ParseConfig opens the file without a context. Pin an ordinary bounded
	// inode before handing it to either client, rejecting symlinks and FIFOs.
	before, err := os.Lstat(path)
	if err != nil || !privatePassfile(before) {
		return nil, errors.New(invalid)
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, errors.New("backup passfile unavailable")
	}
	info, err := file.Stat()
	if err == nil && privatePassfile(info) && os.SameFile(before, info) {
		pinned, err := os.Stat(passfileFD(file))
		if err == nil && os.SameFile(info, pinned) {
			return file, nil
		}
	}
	_ = file.Close()
	return nil, errors.New(invalid)
}

func privatePassfile(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && uint64(stat.Uid) == uint64(os.Geteuid()) && info.Mode() == 0600 && info.Size() <= 32<<10
}

func passfileFD(file *os.File) string {
	if file == nil {
		return os.DevNull
	}
	return "/proc/self/fd/" + strconv.FormatUint(uint64(file.Fd()), 10)
}

func connect(ctx context.Context, cfg CaptureConfig, passfile *os.File) (*pgx.Conn, error) {
	// pgx can import a named service from the environment before parsing the
	// explicit settings. Do not read an unrelated service file for a backup.
	if _, present := os.LookupEnv("PGSERVICE"); present {
		return nil, errors.New("backup requires explicit connection settings; unset PGSERVICE")
	}
	values := url.Values{
		"host": {cfg.Socket}, "port": {cfg.port()}, "dbname": {cfg.Database}, "user": {cfg.User},
		"sslmode": {"disable"}, "password": {""}, "passfile": {passfileFD(passfile)},
		"connect_timeout": {"5"}, "target_session_attrs": {"any"},
	}
	dsn := url.URL{Scheme: "postgres", Host: "localhost", RawQuery: values.Encode()}
	parsed, err := pgx.ParseConfig(dsn.String())
	if err != nil || parsed.Host != cfg.Socket || parsed.Port != cfg.Port || parsed.Database != cfg.Database || parsed.User != cfg.User || len(parsed.Fallbacks) != 0 {
		return nil, errors.New("backup database connection settings invalid")
	}
	parsed.ConnectTimeout = 5 * time.Second
	parsed.RuntimeParams = map[string]string{"search_path": "public", "statement_timeout": "30000", "lock_timeout": "5000"}
	conn, err := pgx.ConnectConfig(ctx, parsed)
	if err != nil {
		return nil, errors.New("backup database unavailable")
	}
	return conn, nil
}

func captureArchive(ctx context.Context, cfg CaptureConfig, snapshot string, passfile *os.File) (FileIdentity, error) {
	// Both clients use the same explicit socket, port, role and database; no
	// DSN or password is passed on argv. The parent holds the exporter open.
	archive, err := writeIdentity(cfg.Output, archiveName, func(w io.Writer) error {
		cmd := exec.CommandContext(ctx, "pg_dump", "--format=custom", "--no-password", "--host", cfg.Socket,
			"--port", cfg.port(), "--username", cfg.User, "--dbname", cfg.Database, "--snapshot", snapshot, "--lock-wait-timeout=5s")
		passwordPath := os.DevNull
		if passfile != nil {
			cmd.ExtraFiles = []*os.File{passfile}
			passwordPath = "/proc/self/fd/3"
		}
		cmd.Env = []string{"LC_ALL=C", "PGCONNECT_TIMEOUT=5", "PGPASSFILE=" + passwordPath}
		cmd.Stdout = w
		if err := cmd.Run(); err != nil {
			return errors.New("pg_dump failed on the exported snapshot")
		}
		return nil
	})
	if err == nil && archive.Bytes == 0 {
		err = errors.New("pg_dump produced an empty archive")
	}
	return archive, err
}
