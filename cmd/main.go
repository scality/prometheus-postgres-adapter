package main

import (
	"context"
	"log"
	"runtime"

	"prom-adapter/cmd/config"
	"prom-adapter/pkg/infrastructure/di"
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

	metricWriter := container.GetPostgreSQLMetricWriter()

	metricWriter.Run(ctx)

	go func() {
		for err := range metricWriter.ErrorChan {
			if err != nil {
				logger.Error().Err(err).Msg("Error in metric writer")
			}
		}
	}()

	err = container.GetHTTPServer().ListenAndServe()
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to start HTTP server")
	}

	logger.Info().Msg("Stopping prometheus-postgres-adapter")
}
