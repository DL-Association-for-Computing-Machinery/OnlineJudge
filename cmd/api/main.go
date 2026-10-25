// Command api starts the OnlineJudge HTTP service.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/httpapi"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("API stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	addr := os.Getenv("OJ_API_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	gin.SetMode(gin.ReleaseMode)
	handler := httpapi.NewHandler()
	scope := "bootstrap-only"
	if tokenFile := os.Getenv("OJ_EXPERIMENT_TOKEN_FILE"); tokenFile != "" {
		token, err := experiment.LoadToken(addr, tokenFile)
		if err != nil {
			return err
		}
		startup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		dsn := os.Getenv("OJ_DATABASE_URL")
		if dsn == "" {
			return errors.New("OJ_DATABASE_URL is required for the experiment")
		}
		pool, err := postgres.Open(startup, dsn)
		if err != nil {
			return err
		}
		defer pool.Close()
		if err := db.CheckSchema(startup, pool); err != nil {
			return err
		}
		store, err := postgres.New(pool, experiment.Version, experiment.Manifest())
		if err != nil {
			return err
		}
		handler = httpapi.NewHandler(httpapi.Experiment{Store: store, Token: token})
		scope = "m0-acceptance-only"
	}
	server := &http.Server{
		Addr: addr, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second, WriteTimeout: 15 * time.Second,
		IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result := make(chan error, 1)
	go func() { result <- server.ListenAndServe() }()
	logger.Info("API starting", "address", addr, "scope", scope)
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return err
		}
		if err := <-result; !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		logger.Info("API stopped gracefully")
		return nil
	}
}
