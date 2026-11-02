package localdev

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/checkedfile"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
	"github.com/jackc/pgx/v5/pgxpool"
)

func identityStore(ctx context.Context, c sessionruntime.Config) (*identitypg.Store, *pgxpool.Pool, *password.BoundedHasher, error) {
	h, err := password.New(c.PasswordPolicy, c.PasswordLimits)
	if err != nil {
		return nil, nil, nil, errState
	}
	work, err := password.NewBounded(h, c.PasswordAdmission)
	if err != nil {
		return nil, nil, nil, errState
	}
	pc, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, nil, nil, errState
	}
	pc.MaxConns = 4
	pc.MinConns = 0
	pc.MaxConnLifetime = time.Hour
	pc.MaxConnIdleTime = time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, nil, nil, errState
	}
	s, err := identitypg.New(pool, h)
	if err != nil {
		pool.Close()
		return nil, nil, nil, errState
	}
	return s, pool, work, nil
}

// Initialize provisions only a completely fresh, isolated development install.
// Existing installs are checked, never repaired, rekeyed or password-reset.
func Initialize(ctx context.Context, dir string) error {
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return errState
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errState
	}
	if err = os.Chmod(dir, 0700); err != nil {
		return errState
	}
	if _, err = os.Lstat(filepath.Join(dir, "ready.json")); err == nil {
		return validate(ctx, dir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return errState
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 {
		return errState
	}
	c, err := configuration(dir)
	if err != nil {
		return errState
	}
	s, pool, work, err := identityStore(ctx, c)
	if err != nil {
		return err
	}
	defer pool.Close()
	client, err := redisstore.NewClient(c.Redis)
	if err != nil {
		return errState
	}
	defer func() { _ = client.Close() }()
	var tables int
	if pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'`).Scan(&tables) != nil || tables != 0 {
		return errState
	}
	count, err := client.DBSize(ctx).Result()
	if err != nil || count != 0 {
		return errState
	}
	// Leaving any partial materials makes interruption fail closed on restart.
	if err = generate(dir); err != nil {
		return err
	}
	conn, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return errState
	}
	defer func() { _ = conn.Close() }()
	conn.SetMaxOpenConns(2)
	provider, err := db.Provider(conn)
	if err != nil {
		return errState
	}
	if _, err = provider.Up(ctx); err != nil {
		return errState
	}
	a, err := readAccount(dir)
	if err != nil {
		return err
	}
	email, err := identity.ParseEmail(a.Email)
	if err != nil {
		return errState
	}
	record, err := work.Create(ctx, []byte(a.Password))
	if err != nil {
		return errState
	}
	if _, err = s.InitializePersonal(ctx, a.ID, email, true, record); err != nil {
		return errState
	}
	generation, err := s.BeginRecovery(ctx)
	if err != nil {
		return errState
	}
	if err = s.FinishRecovery(ctx, generation); err != nil {
		return errState
	}
	store, err := redisstore.New(ctx, client, s, work, c.Namespace, redisstore.OrdinaryPolicy())
	if err != nil {
		return errState
	}
	secrets, err := redisstore.LoadLoginSecrets(c.SecretsFile)
	if err != nil {
		return errState
	}
	if redisstore.InitializeLoginLimits(ctx, store, secrets, c.LoginLimits) != nil || redisstore.InitializePasswordChangeLimits(ctx, store, secrets, c.PasswordChange.Limits, c.PasswordChange.Acceptance) != nil {
		return errState
	}
	r, err := sessionruntime.Open(ctx, c)
	if err != nil {
		return errState
	}
	if err = r.Close(ctx); err != nil {
		return errState
	}
	hashes, err := materialHashes(dir)
	if err != nil {
		return err
	}
	if _, err = certificate(dir); err != nil {
		return err
	}
	if err = writeJSON(dir, "ready.pending", manifest{Version: 1, Generation: generation, Files: hashes}); err != nil {
		return err
	}
	if err = os.Rename(filepath.Join(dir, "ready.pending"), filepath.Join(dir, "ready.json")); err != nil {
		return errState
	}
	directory, err := os.Open(dir)
	if err != nil {
		return errState
	}
	defer func() { _ = directory.Close() }()
	if directory.Sync() != nil {
		return errState
	}
	return nil
}

func validate(ctx context.Context, dir string) error {
	body, err := checkedfile.ReadSecret(filepath.Join(dir, "ready.json"), 16384)
	if err != nil {
		return errState
	}
	var ready manifest
	if json.Unmarshal(body, &ready) != nil || ready.Version != 1 || !identity.ValidRecoveryID(ready.Generation) {
		return errState
	}
	hashes, err := materialHashes(dir)
	if err != nil || !reflect.DeepEqual(hashes, ready.Files) {
		return errState
	}
	if _, err = certificate(dir); err != nil {
		return err
	}
	c, err := configuration(dir)
	if err != nil {
		return errState
	}
	s, pool, _, err := identityStore(ctx, c)
	if err != nil {
		return err
	}
	defer pool.Close()
	state, err := s.RecoveryState(ctx)
	if err != nil || !state.Enabled || state.Generation != ready.Generation {
		return errState
	}
	a, err := readAccount(dir)
	if err != nil {
		return err
	}
	p, err := s.PersonalByID(ctx, a.ID)
	if err != nil || !p.Enabled || p.Email.Original() != a.Email {
		return errState
	}
	// Credentials may have been changed deliberately. Never reset them here.
	r, err := sessionruntime.Open(ctx, c)
	if err != nil {
		return errState
	}
	if r.Close(ctx) != nil {
		return errState
	}
	return nil
}
