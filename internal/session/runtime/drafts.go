package runtime

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/checkedfile"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	problempg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// PrivateDraftsConfig enables trusted snapshot imports, owner import receipts
// and statement/sample previews. The source lock is provisioned independently
// outside the import snapshot. Paths are local configuration, never HTTP input;
// no directory is created.
type PrivateDraftsConfig struct {
	MaterialRoot, BundleDirectory, SourceLockFile string
	SourceIDs                                     []string
	Timeout                                       time.Duration
	MaxInFlight                                   int
}

// Validate checks bounded lexical configuration without accessing files.
func (c PrivateDraftsConfig) Validate() error {
	for _, path := range []string{c.MaterialRoot, c.BundleDirectory, c.SourceLockFile} {
		if len(path) > 4096 || !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsRune(path, 0) {
			return ErrConfig
		}
	}
	within := func(path, root string) bool {
		rel, err := filepath.Rel(root, path)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	if within(c.MaterialRoot, c.BundleDirectory) || within(c.BundleDirectory, c.MaterialRoot) || within(c.SourceLockFile, c.BundleDirectory) || within(c.SourceLockFile, c.MaterialRoot) ||
		c.Timeout <= 0 || c.Timeout > time.Minute || c.MaxInFlight < 1 || c.MaxInFlight > 10 || len(c.SourceIDs) == 0 || len(c.SourceIDs) > 10 {
		return ErrConfig
	}
	seen := make(map[string]bool, len(c.SourceIDs))
	for _, id := range c.SourceIDs {
		if id == "" || len(id) > 128 || seen[id] {
			return ErrConfig
		}
		seen[id] = true
	}
	return nil
}

func openDrafts(ctx context.Context, c PrivateDraftsConfig, sessions *redisstore.Store, pool *pgxpool.Pool) (*problem.Importer, func() error, error) {
	var prepared []*problem.Prepared
	var materials *materialstore.Local
	cleanup := func() error {
		var errs []error
		for _, p := range prepared {
			errs = append(errs, p.Close())
		}
		if materials != nil {
			errs = append(errs, materials.Close())
		}
		if errors.Join(errs...) != nil {
			return ErrClose
		}
		return nil
	}
	ok := false
	defer func() {
		if !ok {
			_ = cleanup()
		}
	}()
	lock, err := checkedfile.ReadPublic(c.SourceLockFile, 1<<20)
	if err != nil {
		return nil, nil, ErrStartup
	}
	sources := make(map[string]*problem.Prepared, len(c.SourceIDs))
	for _, id := range c.SourceIDs {
		if ctx.Err() != nil {
			return nil, nil, startupFailure(ctx)
		}
		p, err := problem.LoadBundle(c.BundleDirectory, id, lock)
		if err != nil {
			return nil, nil, ErrStartup
		}
		prepared = append(prepared, p)
		sources[id] = p
	}
	if ctx.Err() != nil {
		return nil, nil, startupFailure(ctx)
	}
	materials, err = materialstore.OpenLocal(materialstore.Config{Root: c.MaterialRoot, MaxObjectBytes: 128 << 20, MaxInFlight: c.MaxInFlight})
	if err != nil {
		return nil, nil, ErrStartup
	}
	access, err := identitypg.NewProblemAccess(pool)
	if err != nil {
		return nil, nil, ErrStartup
	}
	repository, err := problempg.New(pool, access)
	if err != nil {
		return nil, nil, ErrStartup
	}
	service, err := problem.New(sessions, repository, materials)
	if err != nil {
		return nil, nil, ErrStartup
	}
	importer, err := problem.NewImporter(service, sources, c.MaxInFlight)
	if err != nil {
		return nil, nil, ErrStartup
	}
	ok = true
	return importer, cleanup, nil
}
