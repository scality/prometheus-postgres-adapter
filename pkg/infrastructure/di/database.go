package di

import (
	"prom-adapter/pkg/presentation/database"

	"github.com/jmoiron/sqlx"
)

func (c *Container) getPostgreSQLClient() *database.PostgreSQL {
	if c.postgreSQLClient == nil {
		c.postgreSQLClient = database.NewPostgreSQL(
			c.getPostgreSQLDatabase(),
		)
	}

	return c.postgreSQLClient
}

func (c *Container) getPostgreSQLDatabase() *sqlx.DB {
	if c.postgreSQLDatabase == nil {
		db, err := sqlx.Connect("postgres", c.cfg.Database.URL)
		if err != nil {
			c.GetLogger().Fatal().Err(err).Msg("failed to connect to postgres database")
		}

		c.postgreSQLDatabase = db
	}

	return c.postgreSQLDatabase
}
