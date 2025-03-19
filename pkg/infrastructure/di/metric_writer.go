package di

import (
	"sync"

	"prom-adapter/pkg/infrastructure/metricwriter"
)

func (c *Container) getPostgreSQLMetricWriter() *metricwriter.PostgreSQL {
	if c.postgreSQLMetricWriter == nil {
		postgreSQLMetricWriter, err := metricwriter.NewPostgreSQL(
			c.baseCtx,
			c.getPostgreSQLClient(),
			c.getChanMessageQueue(),
			&sync.Map{},
			c.cfg.MetricParserCount,
		)
		if err != nil {
			c.GetLogger().Fatal().Err(err).Msg("failed to create postgresql metric writer")
		}

		c.postgreSQLMetricWriter = postgreSQLMetricWriter
	}

	return c.postgreSQLMetricWriter
}
