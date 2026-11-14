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
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
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
	ReferenceRequests                             bool
	ReviewIntake                                  bool
	ReviewAccess                                  bool
	ReviewReturns                                 bool
	ReviewApprovals                               bool
	PublicCatalog                                 bool
	StatementConfirmations                        bool
	LocalExecution                                *problemvalidation.APlusBLocalAdmissionInput
	LocalTextExecution                            *problemvalidation.LocalTextAdmissionInput
}

func (c PrivateDraftsConfig) hasLocalExecution() bool {
	return c.LocalExecution != nil || c.LocalTextExecution != nil
}

// Validate checks bounded lexical configuration without accessing files.
func (c PrivateDraftsConfig) Validate() error {
	if c.ReviewIntake && !c.ReferenceRequests {
		return ErrConfig
	}
	if (c.ReviewReturns || c.ReviewApprovals || c.StatementConfirmations) && !c.ReviewAccess {
		return ErrConfig
	}
	if c.LocalExecution != nil && (!c.ReviewApprovals || !c.PublicCatalog || len(c.SourceIDs) != 1 || c.SourceIDs[0] != "library-checker:aplusb") {
		return ErrConfig
	}
	if c.LocalTextExecution != nil && (c.LocalExecution != nil || !c.ReviewApprovals || !c.PublicCatalog ||
		!c.LocalTextExecution.Valid() || len(c.SourceIDs) != 1 || c.SourceIDs[0] != c.LocalTextExecution.SourceID) {
		return ErrConfig
	}
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

type draftCapabilities struct {
	localIntake httptransport.TrainingSubmit
	importer    *problem.Importer
	catalog     *httptransport.CatalogReader
	references  *problemreference.Service
	preparation *problemreference.Preparation
	review      *problemreference.ReviewIntake
	solutions   *problemreference.AuthorSolutions
	evidence    *problemreference.ReviewAccess
	discovery   *problemreference.ReviewDiscovery
	materials   *problemreference.ReviewMaterialReader
	returns     *problemreference.ReviewerReturns
	approvals   *problemreference.ReviewerApprovals
	statements  *problemreference.StatementConfirmations
}

func openDrafts(ctx context.Context, c PrivateDraftsConfig, sessions *redisstore.Store, pool *pgxpool.Pool, secrets *redisstore.LoginSecrets, enableLocalIntake bool) (*draftCapabilities, func() error, error) {
	var prepared []*problem.Prepared
	var materials *materialstore.Local
	var preparation *problemreference.Preparation
	cleanup := func() error {
		if preparation != nil {
			preparation.Close()
		}
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
	result := &draftCapabilities{importer: importer}
	reviewer, err := referencepg.NewReviewStore(pool, access)
	if err != nil {
		return nil, nil, ErrStartup
	}
	var admission problemvalidation.LocalExecutionAdmission
	if c.hasLocalExecution() {
		if c.LocalTextExecution != nil {
			admission, err = problemvalidation.ReadLocalTextAdmission(ctx, *c.LocalTextExecution, sources[c.LocalTextExecution.SourceID])
		} else {
			admission, err = problemvalidation.ReadAPlusBLocalAdmission(ctx, *c.LocalExecution)
		}
		if err != nil {
			return nil, nil, ErrStartup
		}
		reviewer, err = reviewer.WithLocalExecution(admission)
		if err != nil {
			return nil, nil, ErrStartup
		}
		if enableLocalIntake {
			result.localIntake, err = localTrainingSubmit(ctx, pool, reviewer, admission)
			if err != nil {
				return nil, nil, ErrStartup
			}
		}
	}
	if c.PublicCatalog {
		result.catalog = &httptransport.CatalogReader{
			List: func(ctx context.Context, after, query string) ([]httptransport.CatalogEntry, string, error) {
				entries, next, err := reviewer.ListPublished(ctx, after, query)
				if err != nil {
					return nil, "", err
				}
				out := make([]httptransport.CatalogEntry, 0, len(entries))
				for _, e := range entries {
					out = append(out, httptransport.CatalogEntry{ID: e.ID, Version: e.Version, Title: e.Title})
				}
				return out, next, nil
			},
			Detail: func(ctx context.Context, id string) (problem.PublicationStatement, error) {
				release, err := importer.AcquireMaterialSlot()
				if err != nil {
					return problem.PublicationStatement{}, err
				}
				defer release()
				return reviewer.ReadPublished(ctx, materials, id)
			},
		}
	}
	if c.ReviewAccess {
		result.evidence, err = problemreference.NewReviewAccess(sessions, reviewer)
		if err != nil {
			return nil, nil, ErrStartup
		}
		cursor, err := redisstore.NewReviewCursorCipher(secrets)
		if err != nil {
			return nil, nil, ErrStartup
		}
		result.discovery, err = problemreference.NewReviewDiscovery(sessions, reviewer, cursor)
		if err != nil {
			return nil, nil, ErrStartup
		}
		if c.ReviewReturns {
			result.returns, err = problemreference.NewReviewerReturns(sessions, reviewer)
			if err != nil {
				return nil, nil, ErrStartup
			}
		}
		if c.ReviewApprovals {
			result.approvals, err = problemreference.NewReviewerApprovals(sessions, reviewer)
			if err != nil {
				return nil, nil, ErrStartup
			}
		}
		reviewObjects, err := referencepg.NewReviewMaterialStore(reviewer, problempg.ReadBoundReviewDraft)
		if err != nil {
			return nil, nil, ErrStartup
		}
		result.materials, err = problemreference.NewReviewMaterialReader(sessions, reviewObjects, materials, importer.AcquireMaterialSlot)
		if err != nil {
			return nil, nil, ErrStartup
		}
		if c.StatementConfirmations {
			result.statements, err = problemreference.NewStatementConfirmations(sessions, reviewObjects, materials, importer.AcquireMaterialSlot)
			if err != nil {
				return nil, nil, ErrStartup
			}
		}
	}
	if c.ReferenceRequests {
		catalog, err := problemreference.NewCatalogWithLocalPackage(sources, admission)
		if err != nil {
			return nil, nil, ErrStartup
		}
		access, err := problempg.NewReferenceAccess(access)
		if err != nil {
			return nil, nil, ErrStartup
		}
		repo, err := referencepg.New(pool, access)
		if err != nil {
			return nil, nil, ErrStartup
		}
		result.references, err = problemreference.New(sessions, repo, catalog)
		if err != nil {
			return nil, nil, ErrStartup
		}
		if c.ReviewIntake {
			result.review, err = problemreference.NewReviewIntake(sessions, repo)
			if err != nil {
				return nil, nil, ErrStartup
			}
			solutions, err := referencepg.NewAuthorSolutionStore(repo, problempg.ReadBoundReviewDraft)
			if err != nil {
				return nil, nil, ErrStartup
			}
			result.solutions, err = problemreference.NewAuthorSolutions(sessions, solutions, materials, importer.AcquireMaterialSlot)
			if err != nil {
				return nil, nil, ErrStartup
			}
		}
		plans, err := problemreference.NewPlanService(sessions, repo, repository, materials, lock)
		if err != nil {
			return nil, nil, ErrStartup
		}
		preparation, err = problemreference.NewPreparation(plans)
		if err != nil {
			return nil, nil, ErrStartup
		}
		result.preparation = preparation
	}
	ok = true
	return result, cleanup, nil
}
