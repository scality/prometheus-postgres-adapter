package di

import (
	"context"
	"net/http"

	"prom-adapter/cmd/config"
	"prom-adapter/pkg/infrastructure/metricwriter"
	"prom-adapter/pkg/presentation/database"
	"prom-adapter/pkg/presentation/messagequeue"
	"prom-adapter/pkg/usecase"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
)

type Container struct {
	baseCtx context.Context
	logger  *zerolog.Logger
	cfg     *config.Environment

	// Low level components
	httpServer         *http.Server
	postgreSQLDatabase *sqlx.DB

	// Handlers
	writeHandler  http.Handler
	healthHandler http.Handler

	// Implementations
	postgreSQLMetricWriter *metricwriter.PostgreSQL
	chanMessageQueue       *messagequeue.Chan
	postgreSQLClient       *database.PostgreSQL

	// Use-cases
	pushSamplesUseCase  *usecase.PushSamples
	checkDatabaseHealth *usecase.CheckDatabaseHealth
}

func NewContainer(ctx context.Context, cfg *config.Environment) *Container {
	return &Container{
		baseCtx: ctx,
		cfg:     cfg,
	}
}
