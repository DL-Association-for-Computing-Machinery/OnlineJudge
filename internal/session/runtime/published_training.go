package runtime

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	submissionpg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

// PublishedTrainingConfig serves only approved, already published local
// problems whose execution manifests are independently pinned here.
type PublishedTrainingConfig struct {
	MaterialRoot string                 `json:"material_root"`
	MaxInFlight  int                    `json:"max_in_flight"`
	ManifestPins []PublishedManifestPin `json:"manifest_pins"`
}

// PublishedManifestPin is one independently trusted version and digest pair.
type PublishedManifestPin struct {
	Version string `json:"version"`
	Digest  string `json:"sha256"`
}

// Validate checks configuration syntax without accessing files or PostgreSQL.
func (c PublishedTrainingConfig) Validate() error {
	if !filepath.IsAbs(c.MaterialRoot) || filepath.Clean(c.MaterialRoot) != c.MaterialRoot ||
		len(c.MaterialRoot) > 4096 || strings.ContainsRune(c.MaterialRoot, 0) ||
		c.MaxInFlight < 1 || c.MaxInFlight > 10 || len(c.ManifestPins) == 0 || len(c.ManifestPins) > 10 {
		return ErrConfig
	}
	seen := make(map[string]bool, len(c.ManifestPins))
	for _, pin := range c.ManifestPins {
		id, ok := strings.CutPrefix(pin.Version, "aplusb-")
		if !ok {
			id, ok = strings.CutPrefix(pin.Version, "local-text-")
		}
		if !ok || !problem.ValidImportRevision(id) || !problem.ValidImportRevision(pin.Digest) || seen[pin.Version] {
			return ErrConfig
		}
		seen[pin.Version] = true
	}
	return nil
}

type publicationScope func(referencepg.TrainingPublication) bool

func legacyPublicationScope(a problemvalidation.LocalExecutionAdmission) publicationScope {
	id, plan := a.ReviewBinding()
	_, version, digest := a.Binding()
	return func(p referencepg.TrainingPublication) bool {
		return p.Source == id.SourceID && p.ImportRevision == id.ImportRevision && p.Plan == plan &&
			p.MaterialVersion == version && p.MaterialDigest == digest
	}
}

func catalogReader(pool *pgxpool.Pool, localOnly bool, objects problem.ReferenceObjectReader, acquire func() (func(), error)) httptransport.CatalogReader {
	catalog := referencepg.NewCatalog(pool, localOnly)
	return httptransport.CatalogReader{
		List: func(ctx context.Context, after, query string) ([]httptransport.CatalogEntry, string, error) {
			entries, next, err := catalog.ListPublished(ctx, after, query)
			if err != nil {
				return nil, "", err
			}
			out := make([]httptransport.CatalogEntry, 0, len(entries))
			for _, entry := range entries {
				out = append(out, httptransport.CatalogEntry{ID: entry.ID, Version: entry.Version, Title: entry.Title})
			}
			return out, next, nil
		},
		Detail: func(ctx context.Context, id string) (problem.PublicationStatement, error) {
			release, err := acquire()
			if err != nil {
				return problem.PublicationStatement{}, err
			}
			defer release()
			return catalog.ReadPublished(ctx, objects, id)
		},
	}
}

func withPublishedTraining(h http.Handler, pool *pgxpool.Pool, catalog httptransport.CatalogReader, scope publicationScope, java21, python3 bool) (http.Handler, error) {
	if scope != nil {
		catalog.CanSubmit = func(ctx context.Context, id, version string) (bool, error) {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			p, err := referencepg.ResolveTrainingPublication(ctx, pool, id, version)
			if errors.Is(err, problemreference.ErrNotFound) {
				return false, nil
			}
			return err == nil && scope(p), err
		}
	}
	next, err := httptransport.WithCatalog(h, catalog)
	if err != nil || scope == nil {
		return next, err
	}
	resolve := func(ctx context.Context, tx pgx.Tx, id, version string) (submissionpg.PublishedMaterial, error) {
		p, err := referencepg.ResolveTrainingPublication(ctx, tx, id, version)
		if errors.Is(err, problemreference.ErrNotFound) || (err == nil && !scope(p)) {
			return submissionpg.PublishedMaterial{}, submission.ErrNotFound
		}
		return submissionpg.PublishedMaterial{Version: p.MaterialVersion, Digest: p.MaterialDigest}, err
	}
	return httptransport.WithTrainingIntake(next, func(ctx context.Context, actor identity.TrainingActor, key, id, version, language, source string) (submission.Receipt, error) {
		if (language == submission.LanguageJava21 && !java21) || (language == submission.LanguagePython3 && !python3) {
			return submission.Receipt{}, submission.ErrNotFound
		}
		receipt, _, err := submissionpg.AcceptPublishedTraining(ctx, pool, actor, key, id, version, language, source, resolve)
		return receipt, err
	}, func(ctx context.Context, actor identity.TrainingActor, key string) (submission.Receipt, error) {
		return submissionpg.ReadTrainingReceipt(ctx, pool, actor, key)
	})
}

func openPublishedTraining(h http.Handler, pool *pgxpool.Pool, c PublishedTrainingConfig, java21, python3 bool) (http.Handler, func() error, error) {
	objects, err := materialstore.OpenLocal(materialstore.Config{Root: c.MaterialRoot, MaxObjectBytes: 128 << 20, MaxInFlight: c.MaxInFlight})
	if err != nil {
		return nil, nil, ErrStartup
	}
	slots := make(chan struct{}, c.MaxInFlight)
	acquire := func() (func(), error) {
		select {
		case slots <- struct{}{}:
			return func() { <-slots }, nil
		default:
			return nil, problem.ErrBusy
		}
	}
	pins := make(map[string]string, len(c.ManifestPins))
	for _, pin := range c.ManifestPins {
		pins[pin.Version] = pin.Digest
	}
	next, err := withPublishedTraining(h, pool, catalogReader(pool, true, objects, acquire),
		func(p referencepg.TrainingPublication) bool {
			return pins[p.MaterialVersion] != "" && pins[p.MaterialVersion] == p.MaterialDigest
		}, java21, python3)
	if err != nil {
		_ = objects.Close()
		return nil, nil, ErrConfig
	}
	return next, objects.Close, nil
}
