package di

import (
	"prometheus-postgres-adapter/pkg/infrastructure/metricwriter"
)

func (c *Container) GetPostgreSQLMetricWriter() *metricwriter.PostgreSQL {
	if c.postgreSQLMetricWriter == nil {
		postgreSQLMetricWriter, err := metricwriter.NewPostgreSQL(
			c.baseCtx,
			c.getPostgreSQLClient(),
			c.getChanMessageQueue(),
			c.cfg.MetricParserCount,
			c.cfg.MetricWriterCount,
		)
		if err != nil {
			c.GetLogger().Fatal().Err(err).Msg("failed to create postgresql metric writer")
		}

		c.postgreSQLMetricWriter = postgreSQLMetricWriter
	}

	return c.postgreSQLMetricWriter
}
