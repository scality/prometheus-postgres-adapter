package di

import (
	"fmt"

	"prometheus-postgres-adapter/pkg/presentation/database"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq"
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

//
// postgresql://[user[:password]@][netloc][:port][/dbname][?param1=value1&...]
// user=longtermmetrics_owner_user password=rqDzDAcEZIhGTFaeGwBr0unbc0gtDfcow8PnzLfZ77gr8Vkv5g0jAaL6rd1Va9Sp host=artesca-postgres.artesca-auth.svc port=5432 database=longtermmetrics

// postgresql://longtermmetrics_owner_user:rqDzDAcEZIhGTFaeGwBr0unbc0gtDfcow8PnzLfZ77gr8Vkv5g0jAaL6rd1Va9Sp@host=artesca-postgres.artesca-auth.svc:5432/longtermmetrics
