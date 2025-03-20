package di

import (
	"os"

	"prometheus-postgres-adapter/cmd/config"

	"github.com/rs/zerolog"
)

func (c *Container) GetLogger() *zerolog.Logger {
	if c.logger == nil {
		logger := zerolog.New(os.Stdout).
			With().
			Timestamp().
			Str("application_name", config.ApplicationName).
			Str("application_version", config.ApplicationVersion).
			Logger()

		c.logger = &logger
	}

	return c.logger
}
