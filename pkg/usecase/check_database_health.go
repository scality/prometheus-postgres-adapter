package usecase

import (
	"context"
	"log/slog"

	"github.com/pkg/errors"
)

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
		c.logger.ErrorContext(ctx, "database health check failed", slog.Any("error_message", err))

		return errors.Wrap(err, "database health check failed")
	}

	return nil
}
