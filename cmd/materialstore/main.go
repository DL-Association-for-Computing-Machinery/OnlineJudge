// Command materialstore performs trusted, local object maintenance. It does not
// authorize users, import business drafts or publish database references.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		if errors.Is(err, materialstore.ErrCommitUnknown) {
			fmt.Fprintln(os.Stderr, "material persistence outcome unknown; retry the same object before publishing any reference")
		} else {
			fmt.Fprintln(os.Stderr, "material operation failed; check explicit configuration, private storage and object integrity")
		}
		os.Exit(1)
	}
}

func run(parent context.Context, args []string, output io.Writer) error {
	if len(args) < 1 || (args[0] != "put" && args[0] != "verify") {
		return materialstore.ErrInvalid
	}
	options := flag.NewFlagSet("materialstore", flag.ContinueOnError)
	options.SetOutput(io.Discard)
	root := options.String("root", "", "pre-existing private persistent directory")
	sha := options.String("sha256", "", "expected lowercase SHA-256")
	size := options.Int64("size", -1, "expected byte length")
	maximum := options.Int64("max-object-bytes", 0, "explicit per-object budget")
	timeout := options.Duration("timeout", 0, "explicit cooperative operation timeout")
	source := options.String("source", "", "ordinary source file, for put only")
	if err := options.Parse(args[1:]); err != nil {
		return materialstore.ErrInvalid
	}
	if options.NArg() != 0 || *timeout <= 0 || *timeout > time.Hour || (args[0] == "put") != (*source != "") {
		return materialstore.ErrInvalid
	}
	store, err := materialstore.OpenLocal(materialstore.Config{Root: *root, MaxObjectBytes: *maximum, MaxInFlight: 1})
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	ctx, cancel := context.WithTimeout(parent, *timeout)
	defer cancel()
	object := materialstore.Object{SHA256: *sha, Size: *size}
	if args[0] == "put" {
		file, err := os.OpenFile(*source, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return materialstore.ErrUnavailable
		}
		defer func() { _ = file.Close() }()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() != object.Size {
			return materialstore.ErrIntegrity
		}
		if _, err := store.Put(ctx, object, file); err != nil {
			return err
		}
	} else {
		file, err := store.Open(ctx, object)
		if err != nil {
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
	}
	if err := store.Close(); err != nil {
		if args[0] == "put" {
			return errors.Join(materialstore.ErrCommitUnknown, err)
		}
		return err
	}
	if err := json.NewEncoder(output).Encode(object); err != nil {
		if args[0] == "put" {
			return errors.Join(materialstore.ErrCommitUnknown, err)
		}
		return err
	}
	return nil
}
