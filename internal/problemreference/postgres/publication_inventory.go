package postgres

import (
	"context"
	"errors"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/jackc/pgx/v5"
)

// VisitPublishedStatements enumerates the immutable display objects owned by
// all publication revisions in the caller's snapshot. Catalog visibility does
// not affect backup retention.
func VisitPublishedStatements(ctx context.Context, tx pgx.Tx, visit func(context.Context, materialstore.Object) error) error {
	rows, err := tx.Query(ctx, `SELECT statement_sha256, statement_size FROM problem_publications`)
	if err != nil {
		return errors.New("published statement inventory unavailable")
	}
	defer rows.Close()
	for rows.Next() {
		var object materialstore.Object
		if err := rows.Scan(&object.SHA256, &object.Size); err != nil ||
			!problem.ValidImportRevision(object.SHA256) || object.Size <= 0 || object.Size > problem.MaxPublicationStatementBytes {
			return errors.New("published statement inventory invalid")
		}
		if err := visit(ctx, object); err != nil {
			return err
		}
	}
	if rows.Err() != nil {
		return errors.New("published statement inventory unavailable")
	}
	return nil
}
