package di

import "prom-adapter/pkg/usecase"

func (c *Container) getPushSamplesUseCase() *usecase.PushSamples {
	if c.pushSamplesUseCase == nil {
		c.pushSamplesUseCase = usecase.NewPushSamples(
			c.GetLogger(),
			c.getChanMessageQueue(),
		)
	}

	return c.pushSamplesUseCase
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
