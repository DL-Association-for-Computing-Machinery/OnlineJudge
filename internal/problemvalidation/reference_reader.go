package problemvalidation

import (
	"context"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

type referenceReader interface {
	ReadSource(context.Context) ([]byte, error)
	ReadAnswer(context.Context, int) ([]byte, error)
	ReadCase(context.Context, int) ([]byte, []byte, error)
	Close() error
}

func freezeReferences(ctx context.Context, prepared *problem.Prepared, profile Profile, disk bool) (problem.ReferencePlan, referenceReader, error) {
	if disk {
		s, err := prepared.PrepareDiskReference(ctx)
		if err != nil {
			return problem.ReferencePlan{}, nil, err
		}
		return s.Plan(), s, nil
	}
	p, err := prepared.PrepareReference(ctx, profile.Materials)
	return p, memoryReference{p}, err
}

type memoryReference struct{ plan problem.ReferencePlan }

func (m memoryReference) ReadSource(ctx context.Context) ([]byte, error) {
	return m.plan.SourceData, ctx.Err()
}

func (m memoryReference) ReadAnswer(ctx context.Context, i int) ([]byte, error) {
	return m.plan.Cases[i].AnswerData, ctx.Err()
}

func (m memoryReference) ReadCase(ctx context.Context, i int) ([]byte, []byte, error) {
	c := m.plan.Cases[i]
	return c.InputData, c.AnswerData, ctx.Err()
}

func (m memoryReference) Close() error { return nil }
