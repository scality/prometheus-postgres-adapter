package database

import (
	"context"
	"prometheus-postgres-adapter/pkg/domain"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/prometheus/common/model"
	"github.com/scality/go-errors"
)

var (
	// ErrExecuteQuery is returned when a database query cannot be executed.
	ErrExecuteQuery = errors.New("failed to execute query")
	// ErrCollectRows is returned when rows cannot be collected from a query result.
	ErrCollectRows = errors.New("failed to collect rows")
	// ErrScanRow is returned when a row cannot be scanned into a sample.
	ErrScanRow = errors.New("failed to scan row")
	// ErrBeginTransaction is returned when a transaction cannot be started.
	ErrBeginTransaction = errors.New("failed to begin transaction with postgresql database")
	// ErrCopyRows is returned when rows cannot be copied to the database.
	ErrCopyRows = errors.New("failed to copy rows to postgresql database")
	// ErrCommitTransaction is returned when a transaction cannot be committed.
	ErrCommitTransaction = errors.New("failed to commit transaction to postgresql database")
	// ErrNotAllRowsCopied is returned when fewer rows were copied than expected.
	ErrNotAllRowsCopied = errors.New("not all rows were copied")
	// ErrQueryMinTimestamp is returned when the minimum sample timestamp cannot be queried.
	ErrQueryMinTimestamp = errors.New("failed to query minimum sample timestamp")
	// ErrCollectMinTimestamp is returned when the minimum sample timestamp cannot be collected.
	ErrCollectMinTimestamp = errors.New("failed to collect minimum sample timestamp")
	// ErrQueryLabelNames is returned when label names cannot be queried.
	ErrQueryLabelNames = errors.New("failed to query label names")
	// ErrCollectLabelNames is returned when label names cannot be collected.
	ErrCollectLabelNames = errors.New("failed to collect label names")
	// ErrQueryLabelValues is returned when label values cannot be queried.
	ErrQueryLabelValues = errors.New("failed to query label values")
	// ErrCollectLabelValues is returned when label values cannot be collected.
	ErrCollectLabelValues = errors.New("failed to collect label values")
	// ErrPingDatabase is returned when the database ping fails.
	ErrPingDatabase = errors.New("failed to ping database")
)

const (
	minSampleTimestampQuery = `
		SELECT COALESCE(EXTRACT(EPOCH FROM MIN(metric_time)) * 1000, -1)::bigint
		FROM metric_values`

	labelNamesQuery = `
		SELECT DISTINCT key
		FROM metric_labels, jsonb_object_keys(metric_labels) AS key
		ORDER BY key`

	metricNameValuesQuery = `
		SELECT DISTINCT metric_name
		FROM metric_labels
		WHERE metric_name IS NOT NULL AND metric_name <> ''
		ORDER BY metric_name`

	labelValuesQuery = `
		SELECT DISTINCT metric_labels->>$1 AS value
		FROM metric_labels
		WHERE metric_labels ? $1
		ORDER BY value`
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
		return nil, errors.Wrap(ErrExecuteQuery, errors.CausedBy(err))
	}

	results, err := pgx.CollectRows(rows, pgx.RowToMap)
	if err != nil {
		return nil, errors.Wrap(ErrCollectRows, errors.CausedBy(err))
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
		return nil, errors.Wrap(ErrExecuteQuery, errors.CausedBy(err))
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
				return &domain.SamplesReadFromDatabase{}, errors.Wrap(ErrScanRow, errors.CausedBy(err))
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
		return nil, errors.Wrap(ErrCollectRows, errors.CausedBy(err))
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
		return errors.Wrap(ErrBeginTransaction, errors.CausedBy(err))
	}
	defer transaction.Rollback(ctx) //nolint:errcheck // Rollback is deferred to ensure it is called

	copyCount, err := transaction.CopyFrom(
		ctx,
		pgx.Identifier{tableName},
		columnNames,
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return errors.Wrap(ErrCopyRows, errors.CausedBy(err))
	}

	err = transaction.Commit(ctx)
	if err != nil {
		return errors.Wrap(ErrCommitTransaction, errors.CausedBy(err))
	}

	if copyCount != int64(len(rows)) {
		return ErrNotAllRowsCopied
	}

	return nil
}

// MinSampleTimestamp returns the oldest sample timestamp in milliseconds. The
// boolean is false when the database holds no samples.
func (p *PostgreSQL) MinSampleTimestamp(ctx context.Context) (int64, bool, error) {
	rows, err := p.db.Query(ctx, minSampleTimestampQuery)
	if err != nil {
		return 0, false, errors.Wrap(ErrQueryMinTimestamp, errors.CausedBy(err))
	}

	milliseconds, err := pgx.CollectExactlyOneRow(rows, pgx.RowTo[int64])
	if err != nil {
		return 0, false, errors.Wrap(ErrCollectMinTimestamp, errors.CausedBy(err))
	}

	if milliseconds < 0 {
		return 0, false, nil
	}

	return milliseconds, true, nil
}

// LabelNames returns all distinct label names stored in the database, excluding
// the metric name. It returns a superset (matchers and time range are not
// applied), which the Thanos StoreAPI permits.
func (p *PostgreSQL) LabelNames(ctx context.Context) ([]string, error) {
	rows, err := p.db.Query(ctx, labelNamesQuery)
	if err != nil {
		return nil, errors.Wrap(ErrQueryLabelNames, errors.CausedBy(err))
	}

	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, errors.Wrap(ErrCollectLabelNames, errors.CausedBy(err))
	}

	return names, nil
}

// LabelValues returns all distinct values for the given label name. It returns a
// superset (matchers and time range are not applied).
func (p *PostgreSQL) LabelValues(ctx context.Context, label string) ([]string, error) {
	query := labelValuesQuery

	args := []any{label}

	if label == model.MetricNameLabel {
		query = metricNameValuesQuery
		args = nil
	}

	rows, err := p.db.Query(ctx, query, args...)
	if err != nil {
		return nil, errors.Wrap(ErrQueryLabelValues, errors.CausedBy(err))
	}

	values, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, errors.Wrap(ErrCollectLabelValues, errors.CausedBy(err))
	}

	return values, nil
}

func (p *PostgreSQL) CheckHealth(ctx context.Context) error {
	err := p.db.Ping(ctx)
	if err != nil {
		return errors.Wrap(ErrPingDatabase, errors.CausedBy(err))
	}

	return nil
}

func (p *PostgreSQL) Exec(ctx context.Context, query string, args ...any) error {
	_, err := p.db.Exec(ctx, query, args...)
	if err != nil {
		return errors.Wrap(ErrExecuteQuery, errors.CausedBy(err))
	}

	return nil
}
