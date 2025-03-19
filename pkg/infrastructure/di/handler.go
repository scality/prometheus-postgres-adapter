package di

import (
	"net/http"

	"prom-adapter/pkg/presentation/http/handler"
)

func (c *Container) getWriteHTTPHandler() http.Handler {
	if c.writeHandler == nil {
		c.writeHandler = handler.NewPrometheusToPostgreSQLMetricsPusher(
			c.getPushSamplesUseCase(),
			c.GetLogger(),
		).Handle()
	}

	return c.writeHandler
}

func (c *Container) getHealthHTTPHandler() http.Handler {
	if c.healthHandler == nil {
		c.healthHandler = handler.NewHealth(
			c.getCheckDatabaseHealthUseCase(),
			c.GetLogger(),
		).Handle()
	}

	return c.healthHandler
}
