// checker-candidate performs an offline preflight of explicit checker settings.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(parent context.Context, args []string, out, diagnostics io.Writer) error {
	f := flag.NewFlagSet("checker-candidate", flag.ContinueOnError)
	f.SetOutput(diagnostics)
	bundle := f.String("bundle", "", "absolute path to a prepared private bundle")
	sourceID := f.String("source-id", "", "source ID of the selected problem")
	lock := f.String("source-lock", "", "trusted checked-in source lock file")
	plan := f.String("expected-plan", "", "independently trusted formal plan SHA-256")
	version := f.String("checker", "", "explicit checker semantics version")
	caps := judging.TextLimits{}
	f.IntVar(&caps.Bytes, "bytes", 0, "maximum bytes per reference answer or output")
	f.IntVar(&caps.TokenBytes, "token-bytes", 0, "maximum bytes per token")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return errors.New("invalid checker candidate flags")
	}
	if f.NArg() != 0 || *bundle == "" || *sourceID == "" || *lock == "" || !problem.ValidImportRevision(*plan) ||
		*version != judging.LibraryCheckerTextVersion || !caps.Valid() {
		f.Usage()
		return errors.New("explicit checker settings and trusted bundle binding required")
	}
	raw, err := readPinnedLock(*lock)
	if err != nil {
		return err
	}
	prepared, err := problem.LoadBundle(*bundle, *sourceID, raw)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	candidate, err := problemvalidation.PreflightCheckerCandidate(ctx, prepared, *plan, *version, caps)
	if err := errors.Join(err, prepared.Close(), ctx.Err()); err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(struct {
		Candidate problemvalidation.CheckerCandidate `json:"candidate"`
		SHA256    string                             `json:"sha256"`
	}{candidate, candidate.Digest()})
}

func readPinnedLock(path string) ([]byte, error) {
	const max = 1 << 20
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, errors.New("trusted source lock unavailable")
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > max {
		return nil, errors.New("unsupported source lock")
	}
	raw, err := io.ReadAll(io.LimitReader(f, max+1))
	h := sha256.Sum256(raw)
	if err != nil || len(raw) > max || hex.EncodeToString(h[:]) != problemvalidation.SourceLockSHA256 {
		return nil, errors.New("source lock admission required")
	}
	return raw, nil
}
