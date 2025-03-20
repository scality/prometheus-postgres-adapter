package database

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

type PostgreSQL struct {
	db *sqlx.DB
}

func NewPostgreSQL(db *sqlx.DB) *PostgreSQL {
	return &PostgreSQL{
		db: db,
	}
}

func (p *PostgreSQL) Close() error {
	err := p.db.Close()
	if err != nil {
		return errors.Wrap(err, "failed to close database connection")
	}

	return nil
}

func (p *PostgreSQL) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute query")
	}

	return rows, nil
}

func (p *PostgreSQL) WriteRows(ctx context.Context, query string, rows [][]any) error {
	transaction, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(
			err,
			"failed to begin transaction with postgresql database",
		)
	}
	defer func() {
		if err != nil {
			_ = transaction.Rollback()
		}
	}()

	statement, err := transaction.PrepareContext(ctx, query)
	if err != nil {
		return errors.Wrap(err, "failed to prepare statement to postgresql database")
	}

	for _, row := range rows {
		_, err = statement.ExecContext(ctx, row...)
		if err != nil {
			return errors.Wrap(err, "failed to execute statement to postgresql database")
		}
	}

	err = statement.Close()
	if err != nil {
		return errors.Wrap(err, "failed to close statement to postgresql database")
	}

	err = transaction.Commit()
	if err != nil {
		return errors.Wrap(err, "failed to commit transaction to postgresql database")
	}

	return nil
}

func (p *PostgreSQL) CheckHealth(ctx context.Context) error {
	err := p.db.PingContext(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to ping database")
	}

	return nil
}

func (p *PostgreSQL) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	result, err := p.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute query")
	}

	return result, nil
}
