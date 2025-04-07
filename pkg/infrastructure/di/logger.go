package di

import (
	"os"

	"github.com/rs/zerolog"

	"prometheus-postgres-adapter/cmd/config"
)

func (c *Container) GetLogger() *zerolog.Logger {
	if c.logger == nil {
		logLevel, err := zerolog.ParseLevel(c.cfg.LoggerLogLevel)
		if err != nil {
			logLevel = zerolog.InfoLevel
		}

		logger := zerolog.New(os.Stderr).
			Level(logLevel).
			With().
			Timestamp().
			Str("application_name", config.ApplicationName).
			Str("application_version", config.ApplicationVersion).
			Logger()

		c.logger = &logger
	}

	return c.logger
}
