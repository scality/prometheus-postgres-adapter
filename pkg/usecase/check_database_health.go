package usecase

import (
	"context"

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
	return &CheckDatabaseHealth{
		HealthChecker: healthChecker,
		logger:        logger,
	}
}

func (c *CheckDatabaseHealth) Execute(ctx context.Context) error {
	err := c.HealthChecker.CheckHealth(ctx)
	if err != nil {
		c.logger.Error().Err(err).Msg("database health check failed")
	}

	return nil
}
