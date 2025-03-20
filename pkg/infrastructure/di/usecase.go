package di

import "prometheus-postgres-adapter/pkg/usecase"

func (c *Container) getPushPrometheusSamplesUseCase() *usecase.PushPrometheusSamples {
	if c.pushPrometheusSamplesUseCase == nil {
		c.pushPrometheusSamplesUseCase = usecase.NewPushPrometheusSamples(
			c.GetLogger(),
			c.getChanMessageQueue(),
		)
	}

	return c.pushPrometheusSamplesUseCase
}

func (c *Container) getCheckDatabaseHealthUseCase() *usecase.CheckDatabaseHealth {
	if c.checkDatabaseHealth == nil {
		c.checkDatabaseHealth = usecase.NewCheckDatabaseHealth(
			c.getPostgreSQLClient(),
			c.GetLogger(),
		)
	}

	return c.checkDatabaseHealth
}

func (c *Container) getReadPrometheusSamplesUseCase() *usecase.ReadPrometheusSamples {
	if c.readPrometheusSamplesUseCase == nil {
		c.readPrometheusSamplesUseCase = usecase.NewReadPrometheusSamples(
			c.GetLogger(),
			c.getSQLQueryBuilder(),
			c.getPostgreSQLClient(),
		)
	}

	return c.readPrometheusSamplesUseCase
}
