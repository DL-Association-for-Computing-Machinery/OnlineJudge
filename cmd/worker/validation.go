package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

type validationHost interface {
	sandboxHost
	Identity() (string, string)
}

func validateReference(ctx context.Context, args []string, logger *slog.Logger) error {
	if os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" {
		return errors.New("explicit OJ_SANDBOX_EXPERIMENT=1 required")
	}
	options := flag.NewFlagSet("validate-reference", flag.ContinueOnError)
	options.SetOutput(io.Discard)
	bundle := options.String("bundle", "", "private complete bundle")
	lockPath := options.String("source-lock", "", "reviewed fixed LC lock")
	output := options.String("output", "", "new private report directory")
	start := options.Bool("start-stopped-slot", false, "explicitly start the pre-existing owned slot")
	if options.Parse(args) != nil || options.NArg() != 0 || !canonicalAbsolute(*bundle) || !canonicalAbsolute(*lockPath) || !canonicalAbsolute(*output) {
		return errors.New("invalid reference validation arguments")
	}
	if !referenceOutputOutside(*bundle, *output) {
		return errors.New("report output must be outside the private bundle")
	}
	slot, err := workerhost.ParseM0Slot(os.Getenv("OJ_WORKER_M0_SLOT"))
	if err != nil {
		return err
	}
	lockBytes, err := readValidationLock(*lockPath)
	if err != nil {
		return err
	}
	prepared, err := problem.LoadBundle(*bundle, "library-checker:aplusb", lockBytes)
	if err != nil {
		return err
	}
	defer func() { _ = prepared.Close() }()
	if err := os.Mkdir(*output, 0700); err != nil {
		return errors.New("report directory must be new")
	}
	if err := os.WriteFile(filepath.Join(*output, "INCOMPLETE"), []byte("Reference validation incomplete; publication blocked.\n"), 0600); err != nil {
		return err
	}
	lock, err := lockM0(slot)
	if err != nil {
		return err
	}
	defer lock.Close()
	report, runErr := runReferenceLocked(ctx, prepared, lock, func(ctx context.Context) (validationHost, error) {
		var host *workerhost.Host
		var err error
		if *start {
			host, err = workerhost.StartM0Slot(ctx, slot)
		} else {
			host, err = workerhost.OpenM0Slot(ctx, slot)
		}
		// Preserve a real nil on refused preflight; an interface containing a
		// typed nil Host must never reach Identity or Stop.
		if host == nil {
			return nil, err
		}
		return host, err
	})
	if report.Schema == "" {
		return runErr
	}
	report.Slot = slot.ContainerName()
	if err := persistReferenceReport(*output, report); err != nil {
		return errors.New("reference report persistence failed; keep incomplete output")
	}
	if runErr != nil {
		return errors.New("reference validation failed; inspect private report")
	}
	if report.Status != "reference-validation-passed" {
		return errors.New("reference program did not pass; inspect private report")
	}
	logger.Info("完整参考验证通过；发布仍阻断", "source_id", report.Plan.SourceID, "points", len(report.Points), "plan_sha256", report.PlanSHA256, "profile_sha256", report.ProfileSHA256)
	return nil
}

func canonicalAbsolute(path string) bool { return filepath.IsAbs(path) && filepath.Clean(path) == path }

func referenceOutputOutside(bundle, output string) bool {
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		return false
	}
	source, err := os.Stat(bundle)
	if err != nil || !source.IsDir() {
		return false
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(output))
	if err != nil {
		return false
	}
	// File identity catches case-insensitive aliases of the bundle root; a
	// lexical relative path alone can classify the input itself as "outside".
	for {
		info, err := os.Stat(parent)
		if err != nil || !info.IsDir() || os.SameFile(source, info) {
			return false
		}
		next := filepath.Dir(parent)
		if next == parent {
			return true
		}
		parent = next
	}
}

func readValidationLock(path string) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, errors.New("unsupported source lock")
	}
	raw, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	h := sha256.Sum256(raw)
	if err != nil || len(raw) > 1<<20 || hex.EncodeToString(h[:]) != problemvalidation.SourceLockSHA256 {
		return nil, errors.New("source lock admission required")
	}
	return raw, nil
}

func runReferenceLocked(ctx context.Context, prepared *problem.Prepared, lock *slotLock, open func(context.Context) (validationHost, error)) (problemvalidation.Report, error) {
	var report problemvalidation.Report
	if err := lock.dirty(); err != nil {
		return report, err
	}
	startup, done := context.WithTimeout(ctx, 20*time.Second)
	host, openErr := open(startup)
	done()
	if host == nil {
		return report, errors.New("reference sandbox preflight failed; slot remains quarantined")
	}
	if openErr == nil {
		report, openErr = problemvalidation.New(execution.NewGoJudge(host, host.Verify)).Validate(ctx, prepared, problemvalidation.PinnedProfile())
	} else {
		p := problemvalidation.PinnedProfile()
		report = problemvalidation.Report{Schema: "oj.reference-validation.v1", Status: "reference-validation-failed", Publication: "blocked", Profile: p, ProfileSHA256: p.Digest(), Verdict: "SE", Points: []problemvalidation.Point{}}
	}
	report.ContainerID, report.StartedAt = host.Identity()
	stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	stopErr := host.Stop(stopCtx)
	cancel()
	if d := workerhost.StopDetails(stopErr); d != nil {
		report.StopDiagnostic = &problemvalidation.StopDiagnostic{Stage: d.Stage, Code: d.Code, Observed: d.Observed, Confirmed: d.Confirmed, Probe: d.Probe}
	}
	var cleanErr error
	if stopErr == nil {
		cleanErr = lock.clean()
	}
	finalErr := report.Finalize(errors.Join(openErr, ctx.Err()), stopErr, cleanErr)
	return report, finalErr
}

func persistReferenceReport(directory string, report problemvalidation.Report) error {
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(directory, "report.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(append(raw, '\n'))
	syncErr := f.Sync()
	closeErr := f.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	dir, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	if err := dir.Sync(); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(directory, "INCOMPLETE")); err != nil {
		return err
	}
	return dir.Sync()
}
