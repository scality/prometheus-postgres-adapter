package metricwriter

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"prometheus-postgres-adapter/pkg/presentation/database"
	"prometheus-postgres-adapter/pkg/presentation/messagequeue"

	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
)

const (
	postgreSQLTickerPeriod = 10 * time.Millisecond

	postgreSQLCreateMetricsLabelsTableQuery = `
		CREATE TABLE 
		IF NOT EXISTS metric_labels
		(
			metric_id BIGINT PRIMARY KEY,
			metric_name TEXT NOT NULL,
			metric_name_label TEXT NOT NULL,
			metric_labels jsonb,
			UNIQUE(metric_name, metric_labels)
		)`

	postgreSQLCreateMetricsLabelsIndexQuery = `
		CREATE INDEX
		IF NOT EXISTS metric_labels_labels_idx
		ON metric_labels 
		USING GIN (metric_labels)
	`

	postgreSQLCreateMetricsValuesTableQuery = `
		CREATE TABLE
		IF NOT EXISTS metric_values
		(
			metric_id BIGINT,
			metric_time TIMESTAMPTZ,
			metric_value FLOAT8
		) PARTITION BY RANGE (metric_time)
	`

	postgreSQLCreateMetricsValuesIndexQuery = `
		CREATE INDEX 
		IF NOT EXISTS metric_values_id_time_idx
		ON metric_values
		USING btree 
		(
			metric_id,
			metric_time DESC
		)
	`

	postgreSQLCreateMetricsValuesTimeIndexQuery = `
		CREATE INDEX 
		IF NOT EXISTS metric_values_time_idx
		ON metric_values 
		USING btree 
		(
			metric_time DESC
		)
	`

	postgreSQLSelectMetricsLabelsQuery = `
		SELECT metric_name, metric_labels
		FROM metric_labels
	`

	postgreSQLInsertLabelsStatement = `
		INSERT INTO metric_labels
		    (metric_id, metric_name, metric_name_label, metric_labels)
		VALUES ($1, $2, $3, $4)
	`

	postgreSQLInsertValuesStatement = `
		INSERT INTO metric_values
		    (metric_id, metric_time, metric_value) 
		VALUES ($1, $2, $3)
	`
)

type PostgreSQL struct {
	labelRows  [][]any
	valuesRows [][]any

	messageQueue *messagequeue.Chan
	syncMap      *sync.Map

	parserCount int
	writerCount int

	postgreSQLClient *database.PostgreSQL

	ErrorChan chan error
}

func NewPostgreSQL(
	ctx context.Context,
	client *database.PostgreSQL,
	messageQueue *messagequeue.Chan,
	syncMap *sync.Map, // SyncMap is initialized in the caller, to account for existing metrics
	parserCount int,
	writerCount int,
) (*PostgreSQL, error) {
	postgreSQL := &PostgreSQL{
		parserCount:      parserCount,
		writerCount:      writerCount,
		postgreSQLClient: client,
		messageQueue:     messageQueue,
		syncMap:          syncMap,
		ErrorChan:        make(chan error),
	}

	// Initialize mandatory tables
	err := postgreSQL.setupPostgreSQLTables(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to setup postgresql tables")
	}

	// Recover and register existing metrics
	err = postgreSQL.registerExistingMetrics(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to register existing metrics")
	}

	return postgreSQL, nil
}

func (p *PostgreSQL) Run(ctx context.Context) {
	for _ = range p.parserCount {
		go p.parser(ctx)
	}

	for _ = range p.writerCount {
		go p.saver(ctx)
	}
}

func (p *PostgreSQL) registerExistingMetrics(ctx context.Context) error {
	rows, err := p.postgreSQLClient.Query(ctx, postgreSQLSelectMetricsLabelsQuery)
	if err != nil {
		return errors.Wrap(err, "failed to query existing metrics")
	}
	defer rows.Close()

	for rows.Next() {
		var metricName string
		var metricLabels string
		if err := rows.Scan(&metricName, &metricLabels); err != nil {
			return errors.Wrap(err, "failed to scan existing metrics")
		}

		p.syncMap.Store(metricName, metricLabels)
	}

	return nil
}

func (p *PostgreSQL) setupPostgreSQLTables(ctx context.Context) error {
	_, err := p.postgreSQLClient.Exec(ctx, postgreSQLCreateMetricsLabelsTableQuery)
	if err != nil {
		return errors.Wrap(err, "failed to create metric_labels table")
	}

	_, err = p.postgreSQLClient.Exec(ctx, postgreSQLCreateMetricsLabelsIndexQuery)
	if err != nil {
		return errors.Wrap(err, "failed to create metric_labels index")
	}

	_, err = p.postgreSQLClient.Exec(ctx, postgreSQLCreateMetricsValuesTableQuery)
	if err != nil {
		return errors.Wrap(err, "failed to create metric_values table")
	}

	_, err = p.postgreSQLClient.Exec(ctx, postgreSQLCreateMetricsValuesIndexQuery)
	if err != nil {
		return errors.Wrap(err, "failed to create metric_values index")
	}

	_, err = p.postgreSQLClient.Exec(ctx, postgreSQLCreateMetricsValuesTimeIndexQuery)
	if err != nil {
		return errors.Wrap(err, "failed to create metric_values time index")
	}

	return nil
}

func (p *PostgreSQL) saver(ctx context.Context) {
	ticker := time.NewTicker(postgreSQLTickerPeriod)

	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			close(p.ErrorChan)

			return
		case <-ticker.C:
			if len(p.valuesRows) > 0 { // There are rows to commit
				err := p.save(ctx)
				if err != nil {
					p.ErrorChan <- errors.Wrap(err, "failed to save rows")
				}
			}
		}
	}
}

func (p *PostgreSQL) save(ctx context.Context) error {
	err := p.postgreSQLClient.WriteRows(ctx, postgreSQLInsertLabelsStatement, p.labelRows)
	if err != nil {
		return errors.Wrap(err, "failed to save labels")
	}

	err = p.postgreSQLClient.WriteRows(ctx, postgreSQLInsertValuesStatement, p.valuesRows)
	if err != nil {
		return errors.Wrap(err, "failed to save values")
	}

	return nil
}

func (p *PostgreSQL) parser(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Millisecond)

	for {
		select {
		case <-ctx.Done():
			ticker.Stop()

			return
		case <-ticker.C:
			samples := p.messageQueue.Pop()
			if samples == nil {
				continue
			}

			parsedSamples := make([][]any, 0, len(samples.S))

			for _, sample := range samples.S {
				metricString := metricString(sample.Metric)
				// TODO Seems like it requires timestamp handling here, TBC

				// Get the metric ID from the sync map
				id, ok := p.syncMap.Load(metricString)
				if !ok {
					// This operation will be expensive if there are a lot of metrics
					// In our use-case, we expect a small number of metrics
					// If we ever need to scale this, we should consider using a different approach,
					// for example, a map with a mutex
					nextID := 1
					p.syncMap.Range(func(_, _ any) bool {
						nextID++

						return true
					})

					// Store the metric ID in the sync map
					p.syncMap.Store(metricString, nextID)

					// FIXME I don't like this piece of code
					index := strings.Index(metricString, "{")
					jsonbMap := make(map[string]any)
					err := json.Unmarshal([]byte(metricString[index:]), &jsonbMap)
					if err != nil {
						p.ErrorChan <- errors.Wrap(err, "failed to unmarshal json")

						continue
					}

					// Add this metric to the labels to be written
					p.labelRows = append(p.labelRows, []any{
						nextID,
						metricString[:index],
						metricString,
						jsonbMap,
					})

					id = nextID
				}

				parsedSamples = append(parsedSamples, []any{
					id,
					// FIXME Timestamp might need to be handled differently as mentionned above
					sample.Timestamp,
					sample.Value,
				})
			}
			p.valuesRows = append(p.valuesRows, parsedSamples...)
		}
	}
}

// TODO Might need to rethink this
func metricString(m model.Metric) string {
	metricName, hasName := m[model.MetricNameLabel]
	numLabels := len(m) - 1
	if !hasName {
		numLabels = len(m)
	}
	labelStrings := make([]string, 0, numLabels)
	for label, value := range m {
		if label != model.MetricNameLabel {
			labelStrings = append(labelStrings, fmt.Sprintf("\"%s\": %q", label, value))
		}
	}

	switch numLabels {
	case 0:
		if hasName {
			return string(metricName)
		}
		return "{}"
	default:
		sort.Strings(labelStrings)
		return fmt.Sprintf("%s{%s}", metricName, strings.Join(labelStrings, ", "))
	}
}
