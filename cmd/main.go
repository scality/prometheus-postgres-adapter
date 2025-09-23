package main

import (
	"context"
	"log"
	"prometheus-postgres-adapter/cmd/config"
	"prometheus-postgres-adapter/pkg/infrastructure/di"
	"runtime"
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

	// Initialize the base context of the application.
	// 	Every dependency will be able to use this context.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load configuration from environment variables.
	cfg, err := config.NewEnvironment(ctx)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Initialize the container
	container := di.NewContainer(ctx, cfg)

	// Initialize the logger
	logger := container.GetLogger()

	logger.Info().Msg("Starting prometheus-postgres-adapter")

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
				logger.Error().
					Err(concurrentError.Err).
					Str("component", concurrentError.Component).
					Msg("Error in metric writer")
			}
		}
	}()

	// Initialize and run the HTTP server
	err = container.GetHTTPServer().ListenAndServe()
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to start HTTP server")
	}

	logger.Info().Msg("Stopping prometheus-postgres-adapter")
}
