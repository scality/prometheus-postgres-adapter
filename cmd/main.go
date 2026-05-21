package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"prometheus-postgres-adapter/cmd/config"
	"prometheus-postgres-adapter/pkg/infrastructure/di"
	"runtime"
)

func main() {
	// Initialize the base context of the application.
	// 	Every dependency will be able to use this context.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load configuration from environment variables.
	cfg, err := config.NewEnvironment(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize the container
	container := di.NewContainer(ctx, cfg)

	// Initialize the logger
	logger := container.GetLogger()

	logger.InfoContext(ctx, "Starting prometheus-postgres-adapter",
		slog.String("go_version", runtime.Version()),
		slog.String("os", runtime.GOOS),
		slog.String("arch", runtime.GOARCH),
	)

	// Initialize the parsing / writing components
	metricWriter := container.GetPostgreSQLMetricWriter()

	// Run the parsing / writing goroutines
	metricWriter.Run(ctx)

	// Run the error logging goroutine
	// This goroutine will log any error that occurs in any of the
	// parsing / writing goroutines
	go func() {
		for concurrentError := range metricWriter.ErrorChan {
			if concurrentError.Err != nil {
				logger.ErrorContext(ctx, "Error in metric writer",
					slog.Any("error_message", concurrentError.Err),
					slog.String("component", concurrentError.Component),
				)
			}
		}
	}()

	// Initialize and run the HTTP server
	err = container.GetHTTPServer().ListenAndServe()
	if err != nil {
		logger.ErrorContext(ctx, "Failed to start HTTP server", slog.Any("error_message", err))
		os.Exit(1)
	}

	logger.InfoContext(ctx, "Stopping prometheus-postgres-adapter")
}
