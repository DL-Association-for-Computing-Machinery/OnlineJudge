package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/apiapp"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
)

var errSessionRecoveryUsage = errors.New("usage: migrate session-recovery status|close <database> or open <database> <generation> --accounts-reviewed")

const recoveryRedisFailure = "replacement Redis unavailable, nonempty or only partly prepared; read status before retrying and do not reset this instance"

func runSessionRecovery(args []string) error {
	if len(args) < 2 || args[1] == "" {
		return errSessionRecoveryUsage
	}
	req := sessionruntime.RecoveryRequest{Action: sessionruntime.RecoveryAction(args[0]), Database: args[1]}
	switch req.Action {
	case sessionruntime.RecoveryStatus, sessionruntime.RecoveryClose:
		if len(args) != 2 {
			return errSessionRecoveryUsage
		}
	case sessionruntime.RecoveryOpen:
		if len(args) != 4 || args[3] != "--accounts-reviewed" {
			return errSessionRecoveryUsage
		}
		req.Generation, req.AccountsReviewed = args[2], true
	default:
		return errSessionRecoveryUsage
	}
	path := os.Getenv("OJ_RESTORED_SESSION_CONFIG_FILE")
	if path == "" {
		return errors.New("OJ_RESTORED_SESSION_CONFIG_FILE is required")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	state, err := apiapp.MaintainRecovery(ctx, path, os.LookupEnv, req)
	if err != nil {
		if errors.Is(err, sessionruntime.ErrRecoveryRedis) {
			return errors.New(recoveryRedisFailure)
		}
		for _, known := range []error{sessionruntime.ErrRecoveryEpoch, sessionruntime.ErrConfig, sessionruntime.ErrStartup,
			sessionruntime.ErrSecrets, apiapp.ErrConfiguration, apiapp.ErrTLS} {
			if errors.Is(err, known) {
				return known
			}
		}
		return errors.New("session recovery failed or outcome unknown; read status before retrying")
	}
	if _, err := fmt.Printf("generation=%s enabled=%t\n", state.Generation, state.Enabled); err != nil {
		return errors.New("session recovery receipt unavailable; read status before retrying")
	}
	return nil
}
