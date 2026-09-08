package database

import (
	"context"
	"fmt"
	"log/slog"
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
	// ErrQuerySeriesExist is returned when the series existence cannot be queried.
	ErrQuerySeriesExist = errors.New("failed to query series existence")
	// ErrCollectSeriesExist is returned when the series existence cannot be collected.
	ErrCollectSeriesExist = errors.New("failed to collect series existence")
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

	// The label metadata queries take a predicate built by the SQL query
	// builder, which references the metric_labels table under the alias l.
	// It is parenthesized where it joins the other conditions, so that a
	// predicate holding a top-level OR cannot widen them.
	//
	// The label names are sorted by the server, which merges the metric name
	// and the external labels into them, so sorting them here would be work
	// thrown away.
	labelNamesQueryFormat = `
		SELECT DISTINCT e.key
		FROM metric_labels l
		LEFT JOIN LATERAL jsonb_each_text(
			CASE
				WHEN jsonb_typeof(l.metric_labels) = 'object' THEN l.metric_labels
				ELSE '{}'::jsonb
			END
		) AS e ON COALESCE(e.value, '') <> ''
		WHERE (%s)`

	metricNameValuesQueryFormat = `
		SELECT DISTINCT l.metric_name
		FROM metric_labels l
		WHERE l.metric_name IS NOT NULL AND l.metric_name <> ''
		  AND (%s)
		ORDER BY metric_name`

	// The containment test is what the GIN index serves, and the only reason
	// this is not a sequential scan: the COALESCE that follows says the same
	// thing about a key that is missing, but no index can answer it (50k
	// series, a label nothing carries: 0.02ms with it, 7.5ms without).
	labelValuesQueryFormat = `
		SELECT DISTINCT l.metric_labels->>$1 AS value
		FROM metric_labels l
		WHERE l.metric_labels ? $1
		  AND COALESCE(l.metric_labels->>$1, '') <> ''
		  AND (%s)
		ORDER BY value`

	seriesExistQueryFormat = `
		SELECT EXISTS (
			SELECT 1
			FROM metric_labels l
			WHERE (%s)
		)`
)

type (
	PostgreSQL struct {
		logger *slog.Logger
		db     database
	}

	database interface {
		Close()
		Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
		Begin(ctx context.Context) (pgx.Tx, error)
		Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
		Ping(ctx context.Context) error
	}
)

func NewPostgreSQL(logger *slog.Logger, db database) *PostgreSQL {
	return &PostgreSQL{
		logger: logger.With(slog.String("component", "database")),
		db:     db,
	}
}

func (p *PostgreSQL) Close() error {
	p.db.Close()

	return nil
}

// logQuery logs an executed SQL statement and its arguments at debug level.
func (p *PostgreSQL) logQuery(ctx context.Context, query string, args ...any) {
	p.logger.DebugContext(ctx, "executing query",
		slog.String("query", query),
		slog.Any("args", args),
	)
}

func (p *PostgreSQL) QueryToMap(
	ctx context.Context,
	query string,
	args ...any,
) ([]map[string]any, error) {
	p.logQuery(ctx, query, args...)

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
	p.logQuery(ctx, query, args...)

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
	p.logger.DebugContext(ctx, "copying rows",
		slog.String("table", tableName),
		slog.Int("rows", len(rows)),
	)

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
	p.logQuery(ctx, minSampleTimestampQuery)

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

// LabelNames returns the distinct label names carried by the series matching
// the given SQL predicate, and whether any series matched at all. The outer
// join is what answers both at once: a matching series carrying no label of its
// own still comes back, as a row with no key.
//
// The metric name is not one of the names: it lives in its own column, and the
// StoreAPI server reports it. The time range of a query is not applied either:
// the result may hold names whose series have no sample in the requested
// window. The names come back unsorted, since the server sorts what it merges
// them into.
func (p *PostgreSQL) LabelNames(ctx context.Context, predicate string) ([]string, bool, error) {
	query := fmt.Sprintf(labelNamesQueryFormat, predicate)

	p.logQuery(ctx, query)

	rows, err := p.db.Query(ctx, query)
	if err != nil {
		return nil, false, errors.Wrap(ErrQueryLabelNames, errors.CausedBy(err))
	}

	keys, err := pgx.CollectRows(rows, pgx.RowTo[*string])
	if err != nil {
		return nil, false, errors.Wrap(ErrCollectLabelNames, errors.CausedBy(err))
	}

	names := make([]string, 0, len(keys))

	for _, key := range keys {
		if key != nil {
			names = append(names, *key)
		}
	}

	return names, len(keys) > 0, nil
}

// SeriesExist reports whether at least one stored series matches the given SQL
// predicate.
func (p *PostgreSQL) SeriesExist(ctx context.Context, predicate string) (bool, error) {
	query := fmt.Sprintf(seriesExistQueryFormat, predicate)

	p.logQuery(ctx, query)

	rows, err := p.db.Query(ctx, query)
	if err != nil {
		return false, errors.Wrap(ErrQuerySeriesExist, errors.CausedBy(err))
	}

	exist, err := pgx.CollectExactlyOneRow(rows, pgx.RowTo[bool])
	if err != nil {
		return false, errors.Wrap(ErrCollectSeriesExist, errors.CausedBy(err))
	}

	return exist, nil
}

// LabelValues returns the distinct values the given label takes on the series
// matching the given SQL predicate. The time range of a query is not applied:
// the result may hold values whose series have no sample in the requested
// window.
func (p *PostgreSQL) LabelValues(ctx context.Context, label, predicate string) ([]string, error) {
	query := fmt.Sprintf(labelValuesQueryFormat, predicate)

	args := []any{label}

	if label == model.MetricNameLabel {
		query = fmt.Sprintf(metricNameValuesQueryFormat, predicate)
		args = nil
	}

	p.logQuery(ctx, query, args...)

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
	p.logQuery(ctx, query, args...)

	_, err := p.db.Exec(ctx, query, args...)
	if err != nil {
		return errors.Wrap(ErrExecuteQuery, errors.CausedBy(err))
	}

	return nil
}
