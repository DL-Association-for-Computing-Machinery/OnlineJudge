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
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
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
	referenceNodeID := os.Getenv("OJ_WORKER_REFERENCE_NODE_ID")
	referenceRoot := os.Getenv("OJ_WORKER_REFERENCE_MATERIAL_ROOT")
	referenceExecution, err := referenceExecutionEnabled(os.Getenv("OJ_WORKER_REFERENCE_EXECUTION"), referenceNodeID, referenceRoot)
	if err != nil {
		return err
	}
	dispatchSetting := os.Getenv("OJ_WORKER_REFERENCE_DISPATCH")
	if dispatchSetting != "" && (dispatchSetting != "1" || !referenceExecution) {
		return errors.New("reference dispatch requires explicit opt-in and enabled reference execution")
	}
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
	aplusb, aplusbFixture, err := experiment.LoadCachedFixture(os.Getenv)
	if err != nil {
		return err
	}
	version, manifest, err := experiment.SelectFixture(os.Getenv("OJ_EXPERIMENT_MATERIAL_VERSION"))
	if err != nil {
		return err
	}
	enabled := aplusbFixture.UsesCachedText()
	if enabled {
		if os.Getenv("OJ_MATERIAL_ROOT") == "" || referenceNodeID != "" || referenceRoot != "" {
			return errors.New("A+B controller requires a material root and no reference request service")
		}
		version, manifest = aplusb.Version, aplusbFixture.Manifest()
	}
	store, err := postgres.New(pool, version, manifest)
	if err != nil {
		return err
	}
	if enabled {
		if err := store.CheckRegisteredMaterial(startup); err != nil {
			return err
		}
	}
	var materialService *materials.Service
	if path := os.Getenv("OJ_MATERIAL_ROOT"); path != "" {
		open := materials.OpenLocal
		if aplusbFixture.IsLocalText() {
			open = materials.OpenLocalText
		}
		reader, err := open(path)
		if err != nil {
			return err
		}
		// Cleanup does not change the completed operation or its primary error.
		defer func() { _ = reader.Close() }()
		materialService = materials.New(store, reader)
	}
	var referenceObjects problem.ReferenceObjectReader
	if referenceRoot != "" {
		if referenceNodeID == "" {
			return errors.New("reference material root requires a reference node")
		}
		objects, err := materialstore.OpenLocal(materialstore.Config{Root: referenceRoot, MaxObjectBytes: 128 << 20, MaxInFlight: 1})
		if err != nil {
			return err
		}
		defer func() { _ = objects.Close() }()
		referenceObjects = objects
	}
	base := workerapi.NewHandler(store, tlsConfig.ClientCAs, materialService)
	if version == experiment.UbuntuVersion {
		base = workerapi.NewUbuntuFixtureHandler(store, tlsConfig.ClientCAs, materialService)
	}
	if enabled {
		if aplusbFixture.APlusBTier() == experiment.ReferenceAPlusBUbuntu {
			pin := os.Getenv("OJ_APLUSB_MANIFEST_SHA256")
			if aplusbFixture.IsLocalText() {
				pin = os.Getenv("OJ_LOCAL_TEXT_MANIFEST_SHA256")
			}
			base = workerapi.NewUbuntuAPlusBHandler(store, tlsConfig.ClientCAs, materialService, aplusb.Version, pin)
		} else {
			base = workerapi.NewAPlusBHandler(store, tlsConfig.ClientCAs, materialService, aplusb)
		}
	}
	handler, err := configureReferenceClaims(base,
		pool, tlsConfig.ClientCAs, referenceNodeID, referenceObjects, referenceExecution, dispatchSetting == "1")
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr: addr, Handler: handler, TLSConfig: tlsConfig,
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
