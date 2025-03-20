package di

import "prometheus-postgres-adapter/pkg/infrastructure/querybuilder"

func (c *Container) getSQLQueryBuilder() *querybuilder.SQL {
	if c.sqlQueryBuilder == nil {
		c.sqlQueryBuilder = querybuilder.NewSQL()
	}

	return c.sqlQueryBuilder
}
