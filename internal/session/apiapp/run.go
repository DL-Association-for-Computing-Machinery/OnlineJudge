package apiapp

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/httpapi"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/apiconfig"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
)

// Run prepares all dependencies before opening a TLS listener. Caller registers
// signals before calling and owns logger/lookup. It never provisions stores or
// enables experiment routes. Cancellation, cleanup and shutdown are cooperative;
// incomplete drain returns an error, never a graceful-success log.
func Run(ctx context.Context, logger *slog.Logger, address, path string, lookup Lookup) error {
	if ctx == nil || logger == nil || !validAddress(address) {
		return ErrConfiguration
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	started := time.Now()
	c, err := loadConfig(ctx, path, lookup)
	if err != nil {
		return err
	}
	// Include bounded synchronous configuration preparation in the same elapsed
	// startup budget; the configured deadline is known only after parsing.
	startup, cancel := context.WithDeadline(ctx, started.Add(c.Runtime.StartupTimeout))
	defer cancel()
	if err := startup.Err(); err != nil {
		return err
	}
	serverTLS, err := loadTLS(&c)
	if err != nil {
		return err
	}
	if err := startup.Err(); err != nil {
		return err
	}
	runtime, err := sessionruntime.Open(startup, c.Runtime)
	if err != nil {
		return err
	}
	if err := startup.Err(); err != nil {
		return failedPrepared(runtime, c.Server.ShutdownTimeout, err)
	}
	listener, err := (&net.ListenConfig{}).Listen(startup, "tcp", address)
	if err != nil {
		return failedPrepared(runtime, c.Server.ShutdownTimeout, ErrListen)
	}
	defer func() { _ = listener.Close() }()
	if err := startup.Err(); err != nil {
		return failedPrepared(runtime, c.Server.ShutdownTimeout, err)
	}
	server := serverFor(c, logger, runtime)
	server.TLSConfig = serverTLS
	served := make(chan error, 1)
	go func() { served <- server.ServeTLS(listener, "", "") }()
	logger.Info("API starting", "address", listener.Addr().String(), "scope", "ordinary-session-http")
	select {
	case serveErr := <-served:
		_ = server.Close()
		if errors.Is(serveErr, http.ErrServerClosed) {
			return failedPrepared(runtime, c.Server.ShutdownTimeout, ErrShutdown)
		}
		return failedPrepared(runtime, c.Server.ShutdownTimeout, ErrListen)
	case <-ctx.Done():
		logger.Info("API stopping", "scope", "ordinary-session-http")
		return stop(server, runtime, served, c.Server.ShutdownTimeout, logger)
	}
}

func failedPrepared(runtime *sessionruntime.Runtime, budget time.Duration, cause error) error {
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	// A failed command exits nonzero either way. Runtime keeps its own cleanup
	// alive past this bounded wait; never force-close its private store clients.
	if runtime.Close(ctx) != nil {
		return ErrShutdown
	}
	return cause
}

func validAddress(address string) bool {
	_, port, err := net.SplitHostPort(address)
	if err != nil || port == "" {
		return false
	}
	_, err = strconv.ParseUint(port, 10, 16)
	return err == nil
}

func serverFor(c apiconfig.Config, logger *slog.Logger, runtime *sessionruntime.Runtime) *http.Server {
	return &http.Server{Handler: httpapi.NewSessionHandler(runtime),
		ReadHeaderTimeout: c.Server.ReadHeaderTimeout, ReadTimeout: c.Server.ReadTimeout, WriteTimeout: c.Server.WriteTimeout,
		IdleTimeout: c.Server.IdleTimeout, MaxHeaderBytes: c.Server.MaxHeaderBytes,
		ErrorLog: log.New(safeHTTPLog{logger}, "", 0)}
}

type sessionCloser interface{ Close(context.Context) error }
type stoppingServer interface {
	Shutdown(context.Context) error
	Close() error
}

func stop(server stoppingServer, runtime sessionCloser, served <-chan error, budget time.Duration, logger *slog.Logger) error {
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	closed := make(chan error, 1)
	go func() { closed <- runtime.Close(ctx) }()
	shutdownErr := server.Shutdown(ctx)
	closeErr := <-closed
	if shutdownErr != nil || closeErr != nil {
		_ = server.Close()
		return ErrShutdown
	}
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		return ErrListen
	}
	logger.Info("API stopped gracefully", "scope", "ordinary-session-http")
	return nil
}

type safeHTTPLog struct{ logger *slog.Logger }

func (w safeHTTPLog) Write(body []byte) (int, error) {
	w.logger.Warn("session HTTP transport failure")
	return len(body), nil
}
