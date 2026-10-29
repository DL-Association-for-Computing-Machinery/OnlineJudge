// Command worker-api serves the opt-in M0 controller endpoint, never contestant code.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerapi"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, logger); err != nil {
		logger.Error("Worker controller stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	addr := os.Getenv("OJ_WORKER_API_ADDR")
	host, _, err := net.SplitHostPort(addr)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return errors.New("M0 worker API requires an explicit literal loopback address")
	}
	tlsConfig, err := workerapi.LoadTLS(os.Getenv("OJ_WORKER_CA_FILE"), os.Getenv("OJ_WORKER_CERT_FILE"), os.Getenv("OJ_WORKER_KEY_FILE"))
	if err != nil {
		return err
	}
	dsn := os.Getenv("OJ_DATABASE_URL")
	if dsn == "" {
		return errors.New("OJ_DATABASE_URL is required for the worker controller")
	}
	startup, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pool, err := postgres.Open(startup, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.CheckSchema(startup, pool); err != nil {
		return err
	}
	version, manifest, err := experiment.SelectFixture(os.Getenv("OJ_EXPERIMENT_MATERIAL_VERSION"))
	if err != nil {
		return err
	}
	store, err := postgres.New(pool, version, manifest)
	if err != nil {
		return err
	}
	var materialService *materials.Service
	if path := os.Getenv("OJ_MATERIAL_ROOT"); path != "" {
		reader, err := materials.OpenLocal(path)
		if err != nil {
			return err
		}
		// Cleanup does not change the completed operation or its primary error.
		defer func() { _ = reader.Close() }()
		materialService = materials.New(store, reader)
	}
	server := &http.Server{
		Addr: addr, Handler: workerapi.NewHandler(store, tlsConfig.ClientCAs, materialService), TLSConfig: tlsConfig,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192,
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	// Cleanup does not change the completed operation or its primary error.
	defer func() { _ = listener.Close() }()
	stopRecovery := startRecovery(ctx, logger, store.Recover)
	defer stopRecovery()
	result := make(chan error, 1)
	go func() { result <- server.ServeTLS(listener, "", "") }()
	logger.Info("Worker controller starting", "address", addr, "scope", "m0-lease-material-and-task-recovery")
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
			return err
		}
		if err := <-result; !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}
