package usecase

import (
	"context"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type (
	CheckDatabaseHealth struct {
		HealthChecker HealthChecker
		logger        *zerolog.Logger
	}

	HealthChecker interface {
		CheckHealth(context.Context) error
	}
)

func NewCheckDatabaseHealth(
	healthChecker HealthChecker,
	logger *zerolog.Logger,
) *CheckDatabaseHealth {
	l := logger.With().Str("usecase", "check_database_health").Logger()

	return &CheckDatabaseHealth{
		HealthChecker: healthChecker,
		logger:        &l,
	}
}

func (c *CheckDatabaseHealth) Execute(ctx context.Context) error {
	c.logger.Debug().Msg("Executing use case")

	err := c.HealthChecker.CheckHealth(ctx)
	if err != nil {
		c.logger.Error().Err(err).Msg("database health check failed")

		return errors.Wrap(err, "database health check failed")
	}

	return nil
}
