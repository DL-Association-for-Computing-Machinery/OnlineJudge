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

func run(ctx context.Context, logger *slog.Logger, drain <-chan struct{}) error {
	c, err := loadConfig(os.Getenv)
	if err != nil {
		return err
	}
	if c.fixture.IsAPlusB() {
		slot := workerhost.ReferenceSlot{}
		lock, err := lockSlotMode(slot.LockName(), false)
		if err != nil {
			return err
		}
		defer lock.Close()
		return runSlot(ctx, c, logger, lock, func(ctx context.Context) (sandboxHost, error) {
			return workerhost.OpenReference(ctx)
		}, drain)
	}
	ubuntu := c.fixture.Version() == experiment.UbuntuVersion
	lock, err := lockM0Runtime(c.slot, false, os.Getenv(dualModeEnv), ubuntu)
	if err != nil {
		return err
	}
	defer lock.Close()
	return runSlot(ctx, c, logger, lock, func(ctx context.Context) (sandboxHost, error) {
		if ubuntu {
			return workerhost.OpenUbuntuM0(ctx)
		}
		return workerhost.OpenM0Slot(ctx, c.slot)
	}, drain)
}

func runSlot(ctx context.Context, c config, logger *slog.Logger, lock *slotLock, open func(context.Context) (sandboxHost, error), drain <-chan struct{}) (result error) {
	acquirer, err := workerclient.NewAcquirerForFixture(c.endpoint, c.roots, c.cert, c.fixture)
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
	var runner *execution.LeasedRunner
	if c.fixture.IsAPlusB() {
		files, ok := host.(execution.FileTransport)
		if !ok {
			return errors.New("A+B cached transport required")
		}
		factory := func() *execution.CachedGoJudge { return execution.NewReferenceGoJudge(host, host.Verify, files) }
		if err := factory().Verify(startup, c.fixture.Profile()); err != nil {
			return errors.New("A+B sandbox protocol preflight failed; no task claimed")
		}
		runner, err = execution.NewLeasedAPlusB(factory, renewer.Renew, stop, c.fixture)
		if err != nil {
			return err
		}
	} else {
		if err := sandbox.Verify(startup, c.fixture.Profile()); err != nil {
			return errors.New("M0 sandbox protocol preflight failed; no task claimed")
		}
		runner = execution.NewLeasedForFixture(sandbox, renewer.Renew, stop, c.fixture)
	}
	cancel()
	w, err := worker.New(acquirer, acquirer, runner, reporter)
	if err != nil {
		return err
	}
	mode := "m0-single-slot"
	if c.fixture.IsAPlusB() {
		mode = "isolated-aplusb-single-slot"
	}
	logger.Info("Worker 已就绪", "mode", mode)
	err = w.RunWithDrain(ctx, drain)
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
