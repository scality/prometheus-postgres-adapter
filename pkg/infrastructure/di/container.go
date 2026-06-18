package di

import (
	"context"
	"log/slog"
	"net/http"
	"prometheus-postgres-adapter/cmd/config"
	"prometheus-postgres-adapter/pkg/infrastructure/metricwriter"
	"prometheus-postgres-adapter/pkg/infrastructure/querybuilder"
	"prometheus-postgres-adapter/pkg/presentation/database"
	"prometheus-postgres-adapter/pkg/presentation/messagequeue"
	"prometheus-postgres-adapter/pkg/usecase"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
)

type Container struct {
	ctx    context.Context //nolint:containedctx // This context is important to for the DI
	logger *slog.Logger
	cfg    *config.Environment

	// Low level components
	httpServer                  *http.Server
	grpcServer                  *grpc.Server
	postgreSQLDatabaseConnexion *pgxpool.Pool

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
	pushPrometheusSamplesUseCase *usecase.PushPrometheusSamples
	checkDatabaseHealth          *usecase.CheckDatabaseHealth
	readPrometheusSamplesUseCase *usecase.ReadPrometheusSamples
}

func NewContainer(ctx context.Context, cfg *config.Environment) *Container {
	return &Container{
		ctx: ctx,
		cfg: cfg,
	}
}
