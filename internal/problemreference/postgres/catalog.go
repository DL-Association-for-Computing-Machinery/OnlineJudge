package postgres

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres/referencedb"
	"github.com/jackc/pgx/v5"
)

// CatalogEntry contains only public identifiers and a display title.
type CatalogEntry struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Title   string `json:"title"`
}

// ListPublished pages current catalog entries in ID order. Search applies only
// to the current title and treats the supplied string literally.
func (s *ReviewStore) ListPublished(ctx context.Context, after, query string) ([]CatalogEntry, string, error) {
	if s == nil || s.pool == nil || ctx == nil || (after != "" && !identity.ValidID(after)) ||
		len(query) > 128 || !utf8.ValidString(query) || strings.ContainsFunc(query, unicode.IsControl) {
		return nil, "", problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := referencedb.New(s.pool).CatalogProblems(ctx, referencedb.CatalogProblemsParams{
		AfterID: after, TitleQuery: strings.TrimSpace(query), LocalOnly: s.policy() == problemreference.LocalReviewPolicyVersion})
	if err != nil {
		return nil, "", storageError{err}
	}
	page := make([]CatalogEntry, 0, min(len(rows), 20))
	next := ""
	for _, row := range rows {
		if len(page) == 20 {
			next = page[19].ID
			break
		}
		page = append(page, CatalogEntry{ID: row.ProblemID, Version: row.Version, Title: row.Title})
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	return page, next, nil
}

// ReadPublished loads the current public projection, including its verified
// inline images. Callers must hold a shared material read slot. No
// caller-selected material hash can reach object storage.
func (s *ReviewStore) ReadPublished(ctx context.Context, objects problem.ReferenceObjectReader, id string) (problem.PublicationStatement, error) {
	var zero problem.PublicationStatement
	if s == nil || s.pool == nil || ctx == nil || objects == nil || !identity.ValidID(id) {
		return zero, problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	q := referencedb.New(s.pool)
	lookup := referencedb.CatalogProblemParams{ProblemID: id, LocalOnly: s.policy() == problemreference.LocalReviewPolicyVersion}
	row, err := q.CatalogProblem(ctx, lookup)
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, problemreference.ErrNotFound
	}
	if err != nil {
		return zero, storageError{err}
	}
	binding := problem.PublicationMaterials{ProblemID: row.ProblemID, Version: row.Version, FormalPlan: row.FormalPlanSha256,
		Import:    problem.ImportIdentity{SourceID: row.SourceID, Version: row.ContentVersion, ImportRevision: row.ImportRevision, SourceLockSHA256: row.SourceLockSha256},
		Statement: materialstore.Object{SHA256: row.StatementSha256, Size: row.StatementSize}}
	display, err := problem.ReadSealedPublicationStatement(ctx, objects, binding)
	if err != nil || display.Title != row.Title {
		return zero, storageError{errors.Join(problemreference.ErrInvalid, err)}
	}
	current, err := q.CatalogProblem(ctx, lookup)
	if err != nil || current != row {
		return zero, storageError{errors.Join(problemreference.ErrReviewChanged, err)}
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return display, nil
}
