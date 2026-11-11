package problemvalidation

import (
	"bytes"
	"context"
	"io"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

type referenceReader interface {
	ReadSource(context.Context) ([]byte, error)
	ReadDependencies(context.Context) ([]problem.ReferenceSourceFile, error)
	AnswerAt(context.Context, int) (io.ReaderAt, int64, error)
	ReadInput(context.Context, int) ([]byte, error)
	Close() error
}

func freezeReferences(ctx context.Context, prepared *problem.Prepared, limits problem.ReferenceLimits, disk bool) (problem.ReferencePlan, referenceReader, error) {
	if disk {
		s, err := prepared.PrepareDiskReference(ctx)
		if err != nil {
			return problem.ReferencePlan{}, nil, err
		}
		return s.Plan(), s, nil
	}
	p, err := prepared.PrepareReference(ctx, limits)
	return p, memoryReference{p}, err
}

type memoryReference struct{ plan problem.ReferencePlan }

func (m memoryReference) ReadDependencies(ctx context.Context) ([]problem.ReferenceSourceFile, error) {
	return nil, ctx.Err()
}

func (m memoryReference) ReadSource(ctx context.Context) ([]byte, error) {
	return m.plan.SourceData, ctx.Err()
}

func (m memoryReference) AnswerAt(ctx context.Context, i int) (io.ReaderAt, int64, error) {
	data := m.plan.Cases[i].AnswerData
	return bytes.NewReader(data), int64(len(data)), ctx.Err()
}

func (m memoryReference) ReadInput(ctx context.Context, i int) ([]byte, error) {
	return m.plan.Cases[i].InputData, ctx.Err()
}

func (m memoryReference) Close() error { return nil }
