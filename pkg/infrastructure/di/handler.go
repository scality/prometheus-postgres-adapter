package di

import (
	"net/http"

	"prometheus-postgres-adapter/pkg/presentation/http/handler"
)

func (c *Container) getWriteHTTPHandler() http.Handler {
	if c.writeHandler == nil {
		c.writeHandler = handler.NewPrometheusToPostgreSQLMetricsPusher(
			c.getPushPrometheusSamplesUseCase(),
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

func (c *Container) getReadHTTPHandler() http.Handler {
	if c.readHandler == nil {
		c.readHandler = handler.NewReadPrometheusMetrics(
			c.getReadPrometheusSamplesUseCase(),
			c.GetLogger(),
		).Handle()
	}

	return c.readHandler
}
