package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/worker"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerclient"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

type sandboxHost interface {
	http.RoundTripper
	Verify(context.Context, experiment.ExecutionProfile) error
	Stop(context.Context) error
}

func run(ctx context.Context, logger *slog.Logger) error {
	c, err := loadConfig(os.Getenv)
	if err != nil {
		return err
	}
	lock, err := lockM0()
	if err != nil {
		return err
	}
	defer lock.Close()
	return runSlot(ctx, c, logger, lock, func(ctx context.Context) (sandboxHost, error) { return workerhost.OpenM0(ctx) })
}

func runSlot(ctx context.Context, c config, logger *slog.Logger, lock *slotLock, open func(context.Context) (sandboxHost, error)) (result error) {
	acquirer, err := workerclient.NewAcquirer(c.endpoint, c.roots, c.cert)
	if err != nil {
		return err
	}
	defer acquirer.Close()
	renewer, err := workerclient.NewRenewer(c.endpoint, c.roots, c.cert)
	if err != nil {
		return err
	}
	defer renewer.Close()
	reporter, err := workerclient.NewReporter(c.endpoint, c.roots, c.cert)
	if err != nil {
		return err
	}
	defer reporter.Close()
	startup, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	host, err := open(startup)
	if err != nil {
		return errors.New("M0 sandbox preflight failed; no task claimed")
	}
	// Both LeasedRunner and final shutdown use the same stop result. A failed
	// first stop must never become successful merely by retrying a fenced host.
	var once sync.Once
	var stopErr error
	stop := func(ctx context.Context) error {
		once.Do(func() { stopErr = host.Stop(ctx) })
		return stopErr
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if err := stop(cleanup); err != nil {
			result = errors.New("sandbox stop unconfirmed; slot remains quarantined")
			return
		}
		if err := lock.clean(); err != nil {
			result = err
		}
	}()
	if err := lock.dirty(); err != nil {
		return err
	}
	sandbox := execution.NewGoJudge(host, host.Verify)
	if err := sandbox.Verify(startup, experiment.PinnedEnvironment()); err != nil {
		return errors.New("M0 sandbox protocol preflight failed; no task claimed")
	}
	cancel()
	w, err := worker.New(acquirer, acquirer, execution.NewLeased(sandbox, renewer.Renew, stop), reporter)
	if err != nil {
		return err
	}
	logger.Info("Worker 已就绪", "mode", "m0-single-slot")
	err = w.Run(ctx)
	if errors.Is(err, execution.ErrStopUnconfirmed) {
		return errors.New("sandbox stop unconfirmed")
	}
	if ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return errors.New("worker operation failed; inspect controller state before recovery")
	}
	return nil
}
