package database

import (
	"context"
	"prometheus-postgres-adapter/pkg/domain"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pkg/errors"
)

type (
	PostgreSQL struct {
		db database
	}

	database interface {
		Close()
		Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
		Begin(ctx context.Context) (pgx.Tx, error)
		Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
		Ping(ctx context.Context) error
	}
)

func NewPostgreSQL(db database) *PostgreSQL {
	return &PostgreSQL{
		db: db,
	}
}

func (p *PostgreSQL) Close() error {
	p.db.Close()

	return nil
}

func (p *PostgreSQL) QueryToMap(
	ctx context.Context,
	query string,
	args ...any,
) ([]map[string]any, error) {
	rows, err := p.db.Query(ctx, query, args...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute query")
	}

	results, err := pgx.CollectRows(rows, pgx.RowToMap)
	if err != nil {
		return nil, errors.Wrap(err, "failed to collect rows")
	}

	return results, nil
}

func (p *PostgreSQL) QueryDatabaseSamples(
	ctx context.Context,
	query string,
	args ...any,
) ([]*domain.SamplesReadFromDatabase, error) {
	rows, err := p.db.Query(ctx, query, args...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute query")
	}

	samples, err := pgx.CollectRows(
		rows,
		func(row pgx.CollectableRow) (*domain.SamplesReadFromDatabase, error) {
			var timestamp time.Time
			var name string
			var value float64
			var labels domain.SampleLabels

			err := row.Scan(&timestamp, &name, &value, &labels)
			if err != nil {
				return &domain.SamplesReadFromDatabase{}, errors.Wrap(err, "failed to scan row")
			}

			return &domain.SamplesReadFromDatabase{
				Timestamp: timestamp,
				Value:     value,
				Name:      name,
				Labels:    labels,
			}, nil
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to collect rows")
	}

	return samples, nil
}

func (p *PostgreSQL) CopyRows(
	ctx context.Context,
	tableName string,
	columnNames []string,
	rows [][]any,
) error {
	transaction, err := p.db.Begin(ctx)
	if err != nil {
		return errors.Wrap(
			err,
			"failed to begin transaction with postgresql database",
		)
	}
	defer transaction.Rollback(ctx) //nolint:errcheck // Rollback is deferred to ensure it is called

	copyCount, err := transaction.CopyFrom(
		ctx,
		pgx.Identifier{tableName},
		columnNames,
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return errors.Wrap(err, "failed to copy rows to postgresql database")
	}

	err = transaction.Commit(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to commit transaction to postgresql database")
	}

	if copyCount != int64(len(rows)) {
		return errors.New("not all rows were copied")
	}

	return nil
}

func (p *PostgreSQL) CheckHealth(ctx context.Context) error {
	err := p.db.Ping(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to ping database")
	}

	return nil
}

func (p *PostgreSQL) Exec(ctx context.Context, query string, args ...any) error {
	_, err := p.db.Exec(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to execute query")
	}

	return nil
}
