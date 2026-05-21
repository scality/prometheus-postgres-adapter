package di

import (
	"log/slog"
	"os"
	"prometheus-postgres-adapter/pkg/infrastructure/metricwriter"
)

func (c *Container) GetPostgreSQLMetricWriter() *metricwriter.PostgreSQL {
	if c.postgreSQLMetricWriter == nil {
		postgreSQLMetricWriter, err := metricwriter.NewPostgreSQL(
			c.ctx,
			c.getPostgreSQLClient(),
			c.getChanMessageQueue(),
			c.cfg.MetricParserCount,
			c.cfg.MetricWriterCount,
		)
		if err != nil {
			c.GetLogger().ErrorContext(
				c.ctx,
				"failed to create postgresql metric writer",
				slog.Any("error", err),
			)
			os.Exit(1) //nolint:revive // Fatal-equivalent for DI initialization failure
		}

		c.postgreSQLMetricWriter = postgreSQLMetricWriter
	}

	return c.postgreSQLMetricWriter
}
