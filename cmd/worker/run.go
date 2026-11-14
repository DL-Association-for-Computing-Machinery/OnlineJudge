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
	if c.fixture.UsesCachedText() {
		slot, err := workerhost.ReferenceSlotForTier(c.fixture.APlusBTier())
		if c.ubuntuTextSlot != "" {
			slot, err = workerhost.UbuntuTextSlot(c.ubuntuTextSlot)
		}
		if err != nil {
			return err
		}
		if c.fixture.Java21Enabled() {
			slot = slot.WithJava21Image()
		}
		if slot.IsUbuntuAPlusB() {
			probe, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := workerhost.CheckUbuntuAPlusBHost(probe)
			cancel()
			if err != nil {
				return err
			}
		}
		var lock *slotLock
		if c.ubuntuTextSlot != "" {
			lock, err = lockUbuntuTextDual(slot, false)
		} else {
			lock, err = lockReferenceRuntime(slot, false)
		}
		if err != nil {
			return err
		}
		defer lock.Close()
		return runSlot(ctx, c, logger, lock, func(ctx context.Context) (sandboxHost, error) {
			if c.startAPlusB {
				if err := lock.dirty(); err != nil {
					return nil, err
				}
				host, err := workerhost.StartReferenceSlot(ctx, slot)
				if host == nil {
					if slot.IsUbuntuAPlusB() {
						// Match the guest's standalone entry: an unconfirmed startup stays fenced.
						return nil, err
					}
					// Desktop preserves the managed startup contract.
					return nil, errors.Join(err, lock.clean())
				}
				return host, err
			}
			return workerhost.OpenReferenceSlot(ctx, slot)
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
	host, openErr := open(startup)
	if host == nil && c.startAPlusB {
		return errors.Join(errors.New("A+B slot startup refused; no task claimed"), openErr)
	}
	if host == nil || (openErr != nil && !c.startAPlusB) {
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
	if c.startAPlusB {
		if openErr != nil {
			return errors.New("A+B slot startup failed; no task claimed")
		}
	} else {
		if err := lock.dirty(); err != nil {
			return err
		}
	}
	sandbox := execution.NewGoJudge(host, host.Verify)
	var runner *execution.LeasedRunner
	if c.fixture.UsesCachedText() {
		files, ok := host.(execution.FileTransport)
		if !ok {
			return errors.New("A+B cached transport required")
		}
		factory := execution.NewAPlusBCacheFactory(host, host.Verify, files, c.fixture)
		if factory == nil {
			return errors.New("A+B cached runtime unavailable")
		}
		if err := factory().Verify(startup, c.fixture.Profile()); err != nil {
			return errors.New("A+B sandbox protocol preflight failed; no task claimed")
		}
		if err := factory().Verify(startup, experiment.C17Profile(c.fixture.Profile())); err != nil {
			return errors.Join(errors.New("C17 sandbox protocol preflight failed; no task claimed"), err)
		}
		if err := factory().Verify(startup, experiment.GNUCPP20Profile(c.fixture.Profile())); err != nil {
			return errors.Join(errors.New("GNU++20 sandbox protocol preflight failed; no task claimed"), err)
		}
		if c.fixture.Java21Enabled() {
			if err := factory().Verify(startup, experiment.Java21Profile(c.fixture.Profile())); err != nil {
				return errors.Join(errors.New("sandbox protocol preflight failed for Java 21; no task claimed"), err)
			}
		}
		runner, err = execution.NewLeasedCachedText(factory, renewer.Renew, stop, c.fixture)
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
	if c.fixture.IsLocalText() {
		mode = "isolated-local-text-single-slot"
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
