package di

import (
	"context"
	"net/http"

	"prometheus-postgres-adapter/cmd/config"
	"prometheus-postgres-adapter/pkg/infrastructure/metricwriter"
	"prometheus-postgres-adapter/pkg/infrastructure/querybuilder"
	"prometheus-postgres-adapter/pkg/presentation/database"
	"prometheus-postgres-adapter/pkg/presentation/messagequeue"
	"prometheus-postgres-adapter/pkg/usecase"

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
	readHandler   http.Handler

	// Implementations
	postgreSQLMetricWriter *metricwriter.PostgreSQL
	chanMessageQueue       *messagequeue.Chan
	postgreSQLClient       *database.PostgreSQL
	sqlQueryBuilder        *querybuilder.SQL

	// Use-cases
	pushSamplesUseCase  *usecase.PushSamples
	checkDatabaseHealth *usecase.CheckDatabaseHealth
	readSamplesUseCase  *usecase.ReadSamples
}

func NewContainer(ctx context.Context, cfg *config.Environment) *Container {
	return &Container{
		baseCtx: ctx,
		cfg:     cfg,
	}
}
