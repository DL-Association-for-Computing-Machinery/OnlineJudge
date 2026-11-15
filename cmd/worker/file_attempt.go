package main

import (
	"context"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/worker"
)

// The Worker executes attempts serially. A previous execution or cleanup
// failure ends the loop, so a new budget never resumes a failed sandbox.
type fileAttemptExecutor struct {
	worker.Executor
	reset func() error
}

func (e fileAttemptExecutor) JudgeReport(ctx context.Context, lease submission.Lease, d materials.Descriptor, blobs map[string][]byte) (submission.Result, error) {
	if err := e.reset(); err != nil {
		return submission.Result{}, err
	}
	return e.Executor.JudgeReport(ctx, lease, d, blobs)
}
