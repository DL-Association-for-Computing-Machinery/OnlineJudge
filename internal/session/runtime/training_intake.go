package runtime

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	submissionpg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

func localTrainingSubmit(ctx context.Context, pool *pgxpool.Pool, reviews *referencepg.ReviewStore, admission problemvalidation.APlusBLocalAdmission) (httptransport.TrainingSubmit, error) {
	_, version, _ := admission.Binding()
	queue, err := submissionpg.New(pool, version, admission.Manifest())
	if err != nil {
		return nil, err
	}
	if err := queue.CheckRegisteredMaterial(ctx); err != nil {
		return nil, err
	}
	return func(ctx context.Context, actor identity.TrainingActor, key, id, version, source string) (submission.Receipt, error) {
		receipt, _, err := queue.AcceptPublishedTraining(ctx, actor, key, id, version, source, func(ctx context.Context, tx pgx.Tx) error {
			return reviews.AllowLocalTraining(ctx, tx, id, version)
		})
		return receipt, err
	}, nil
}
