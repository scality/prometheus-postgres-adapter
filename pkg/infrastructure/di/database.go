package di

import (
	"fmt"
	"prometheus-postgres-adapter/pkg/presentation/database"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq" // PostgreSQL driver
)

const postgresConnexionString = "postgresql://%s:%s@%s:%d/%s"

func (c *Container) getPostgreSQLClient() *database.PostgreSQL {
	if c.postgreSQLClient == nil {
		c.postgreSQLClient = database.NewPostgreSQL(
			c.getPostgreSQLDatabase(),
		)
	}

	return c.postgreSQLClient
}

func (c *Container) getPostgreSQLDatabase() *pgxpool.Pool {
	if c.postgreSQLDatabaseConnexion == nil {
		connectionString := fmt.Sprintf(
			postgresConnexionString,
			c.cfg.Database.User,
			c.cfg.Database.Password,
			c.cfg.Database.Host,
			c.cfg.Database.Port,
			c.cfg.Database.Name,
		)

		pool, err := pgxpool.New(c.baseCtx, connectionString)
		if err != nil {
			c.GetLogger().Fatal().Err(err).Msg("failed to connect to postgres database")
		}

		c.postgreSQLDatabaseConnexion = pool
	}

	return c.postgreSQLDatabaseConnexion
}
