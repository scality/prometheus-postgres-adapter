package di

import "prometheus-postgres-adapter/pkg/presentation/messagequeue"

func (c *Container) getChanMessageQueue() *messagequeue.Chan {
	if c.chanMessageQueue == nil {
		c.chanMessageQueue = messagequeue.NewChan()
	}

	return c.chanMessageQueue
}
