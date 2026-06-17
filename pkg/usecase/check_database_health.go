package usecase

import (
	"context"
	"log/slog"

	"github.com/scality/go-errors"
)

// ErrDatabaseHealthCheck is returned when the database health check fails.
var ErrDatabaseHealthCheck = errors.New("database health check failed")

type (
	CheckDatabaseHealth struct {
		HealthChecker HealthChecker
		logger        *slog.Logger
	}

	HealthChecker interface {
		CheckHealth(context.Context) error
	}
)

func NewCheckDatabaseHealth(
	healthChecker HealthChecker,
	logger *slog.Logger,
) *CheckDatabaseHealth {
	return &CheckDatabaseHealth{
		HealthChecker: healthChecker,
		logger:        logger.With(slog.String("usecase", "check_database_health")),
	}
}

func (c *CheckDatabaseHealth) Execute(ctx context.Context) error {
	c.logger.DebugContext(ctx, "Executing use case")

	err := c.HealthChecker.CheckHealth(ctx)
	if err != nil {
		c.logger.ErrorContext(ctx, "database health check failed", slog.Any("error", err))

		return errors.Wrap(ErrDatabaseHealthCheck, errors.CausedBy(err))
	}

	return nil
}
