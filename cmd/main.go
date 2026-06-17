package main

import (
	"context"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"prometheus-postgres-adapter/cmd/config"
	"prometheus-postgres-adapter/pkg/infrastructure/di"
	"runtime"
	"syscall"
	"time"

	"github.com/scality/go-errors"
)

const (
	shutdownTimeout = 10 * time.Second
	serverCount     = 2 // HTTP + gRPC
)

var (
	// ErrListenGRPC is returned when the gRPC listener cannot be created.
	ErrListenGRPC = errors.New("failed to listen for gRPC")
	// ErrServerStopped is returned when a server stops before a shutdown signal.
	ErrServerStopped = errors.New("server stopped unexpectedly")
	// ErrShutdownHTTP is returned when the HTTP server fails to shut down gracefully.
	ErrShutdownHTTP = errors.New("failed to shut down HTTP server")
)

func main() {
	log.Printf(
		"Starting %s@%s on %s (%s/%s)\n",
		config.ApplicationName,
		config.ApplicationVersion,
		runtime.Version(),
		runtime.GOOS,
		runtime.GOARCH,
	)

	// The context is cancelled on SIGINT/SIGTERM, which triggers graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Load configuration from environment variables.
	cfg, err := config.NewEnvironment(ctx)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	container := di.NewContainer(ctx, cfg)
	logger := container.GetLogger()

	logger.InfoContext(ctx, "Starting prometheus-postgres-adapter")

	// Run the parsing / writing goroutines.
	metricWriter := container.GetPostgreSQLMetricWriter()
	metricWriter.Run(ctx)

	// Run the error logging goroutine.
	// This goroutine logs any error that occurs in any of the parsing / writing
	// goroutines.
	go func() {
		for concurrentError := range metricWriter.ErrorChan {
			if concurrentError.Err != nil {
				logger.ErrorContext(ctx, "Error in metric writer",
					slog.Any("error", concurrentError.Err),
					slog.String("component", concurrentError.Component),
				)
			}
		}
	}()

	if err := serve(ctx, logger, container, cfg); err != nil {
		logger.ErrorContext(ctx, "Server error", slog.Any("error", err))
		os.Exit(1) //nolint:revive // Fatal-equivalent for server failure
	}

	logger.InfoContext(ctx, "Stopping prometheus-postgres-adapter")
}

// serve starts the HTTP and gRPC servers, waits for a shutdown signal or a fatal
// server error, then gracefully stops both servers.
func serve(
	ctx context.Context,
	logger *slog.Logger,
	container *di.Container,
	cfg *config.Environment,
) error {
	grpcListener, err := net.Listen("tcp", cfg.GRPC.Addr)
	if err != nil {
		return errors.Wrap(ErrListenGRPC, errors.CausedBy(err))
	}

	httpServer := container.GetHTTPServer()
	grpcServer := container.GetGRPCServer()

	serverErrors := make(chan error, serverCount)

	go func() {
		logger.InfoContext(ctx, "Starting HTTP server", slog.String("address", cfg.HTTP.Addr))
		serverErrors <- httpServer.ListenAndServe()
	}()

	go func() {
		logger.InfoContext(ctx, "Starting gRPC StoreAPI server", slog.String("address", cfg.GRPC.Addr))
		serverErrors <- grpcServer.Serve(grpcListener)
	}()

	select {
	case <-ctx.Done():
		logger.InfoContext(ctx, "Shutdown signal received, stopping servers")
	case err := <-serverErrors:
		return errors.Wrap(ErrServerStopped, errors.CausedBy(err))
	}

	grpcServer.GracefulStop()

	// Detach from the (already cancelled) signal context so the timeout applies.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return errors.Wrap(ErrShutdownHTTP, errors.CausedBy(err))
	}

	return nil
}
