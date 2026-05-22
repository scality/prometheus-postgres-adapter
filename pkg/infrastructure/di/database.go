package di

import (
	"fmt"
	"log/slog"
	"os"
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

		if c.cfg.Database.SSLMode != "" {
			connectionString += fmt.Sprintf("?sslmode=%s", c.cfg.Database.SSLMode)
		}

		pool, err := pgxpool.New(c.ctx, connectionString)
		if err != nil {
			c.GetLogger().ErrorContext(
				c.ctx,
				"failed to connect to postgres database",
				slog.Any("error", err),
			)
			os.Exit(1) //nolint:revive // Fatal-equivalent for DI initialization failure
		}

		c.postgreSQLDatabaseConnexion = pool
	}

	return c.postgreSQLDatabaseConnexion
}
