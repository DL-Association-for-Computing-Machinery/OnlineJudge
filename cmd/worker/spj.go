package main

import (
	"context"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/worker"
)

type spjAttemptExecutor struct {
	worker.Executor
	reset func() error
}

func (e spjAttemptExecutor) JudgeReport(ctx context.Context, lease submission.Lease, d materials.Descriptor, blobs map[string][]byte) (submission.Result, error) {
	if err := e.reset(); err != nil {
		return submission.Result{}, err
	}
	return e.Executor.JudgeReport(ctx, lease, d, blobs)
}
