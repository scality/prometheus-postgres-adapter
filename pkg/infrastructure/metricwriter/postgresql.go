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
		)
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
)

type (
	PostgreSQL struct {
		labelRows  [][]any
		valuesRows [][]any

		messageQueue *messagequeue.Chan
		syncMap      *sync.Map

		parserCount int
		writerCount int

		postgreSQLClient *database.PostgreSQL

		ErrorChan chan ConcurrentError
	}

	ConcurrentError struct {
		Err       error
		Component string
	}
)

//nolint:revive // No choice but to use that many parameters
func NewPostgreSQL(
	ctx context.Context,
	client *database.PostgreSQL,
	messageQueue *messagequeue.Chan,
	parserCount int,
	writerCount int,
) (*PostgreSQL, error) {
	postgreSQL := &PostgreSQL{
		parserCount:      parserCount,
		writerCount:      writerCount,
		postgreSQLClient: client,
		syncMap:          &sync.Map{},
		messageQueue:     messageQueue,
		ErrorChan:        make(chan ConcurrentError),
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
	for range p.parserCount {
		go p.parser(ctx)
	}

	for range p.writerCount {
		go p.saver(ctx)
	}
}

func (p *PostgreSQL) registerExistingMetrics(ctx context.Context) error {
	results, err := p.postgreSQLClient.QueryToMap(ctx, postgreSQLSelectMetricsLabelsQuery)
	if err != nil {
		return errors.Wrap(err, "failed to query existing metrics")
	}

	for _, result := range results {
		metricName, ok := result["metric_name"].(string)
		if !ok {
			return errors.New("failed to cast metric_name to string")
		}

		p.syncMap.Store(metricName, result["metric_labels"])
	}

	return nil
}

func (p *PostgreSQL) setupPostgreSQLTables(ctx context.Context) error {
	err := p.postgreSQLClient.Exec(ctx, postgreSQLCreateMetricsLabelsTableQuery)
	if err != nil {
		return errors.Wrap(err, "failed to create metric_labels table")
	}

	err = p.postgreSQLClient.Exec(ctx, postgreSQLCreateMetricsLabelsIndexQuery)
	if err != nil {
		return errors.Wrap(err, "failed to create metric_labels index")
	}

	err = p.postgreSQLClient.Exec(ctx, postgreSQLCreateMetricsValuesTableQuery)
	if err != nil {
		return errors.Wrap(err, "failed to create metric_values table")
	}

	err = p.postgreSQLClient.Exec(ctx, postgreSQLCreateMetricsValuesIndexQuery)
	if err != nil {
		return errors.Wrap(err, "failed to create metric_values index")
	}

	err = p.postgreSQLClient.Exec(ctx, postgreSQLCreateMetricsValuesTimeIndexQuery)
	if err != nil {
		return errors.Wrap(err, "failed to create metric_values time index")
	}

	return nil
}

func (p *PostgreSQL) saver(ctx context.Context) {
	ticker := time.NewTicker(postgreSQLTickerPeriod)

	for {
		select {
		case <-ctx.Done(): // Exit routine
			ticker.Stop()
			close(p.ErrorChan) // Close the channel on top of the hierarchy (Writer owns Parser)

			return
		case <-ticker.C:
			if len(p.valuesRows) > 0 { // There are rows to commit
				err := p.save(ctx)
				if err != nil {
					p.ErrorChan <- ConcurrentError{
						Err:       errors.Wrap(err, "failed to save rows"),
						Component: "Writer",
					}
				}
			}
		}
	}
}

func (p *PostgreSQL) save(ctx context.Context) error {
	err := p.postgreSQLClient.CopyRows(
		ctx,
		"metric_labels",
		[]string{
			"metric_id",
			"metric_name",
			"metric_name_label",
			"metric_labels",
		},
		p.labelRows,
	)

	defer func() {
		// Reset the label rows
		p.labelRows = nil
	}()

	if err != nil {
		return errors.Wrap(err, "failed to save labels")
	}

	err = p.postgreSQLClient.CopyRows(
		ctx,
		"metric_values",
		[]string{
			"metric_id",
			"metric_time",
			"metric_value",
		},
		p.valuesRows,
	)

	defer func() {
		// Reset the values rows
		p.valuesRows = nil
	}()

	if err != nil {
		return errors.Wrap(err, "failed to save values")
	}

	return nil
}

//nolint:gocognit,funlen // Splitting this function would make it even more complex
func (p *PostgreSQL) parser(ctx context.Context) {
	ticker := time.NewTicker(postgreSQLTickerPeriod)

	for {
		select {
		case <-ctx.Done(): // Exit routine
			ticker.Stop()

			return
		case <-ticker.C:
			samples := p.messageQueue.Pop()
			if samples == nil {
				continue
			}

			parsedSamples := make([][]any, 0, len(samples.S))

			for _, sample := range samples.S {
				// Convert the metric to a string representation
				metricString := transformToMetricString(sample.Metric)

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

					index := strings.Index(metricString, "{")
					jsonbMap := make(map[string]any)

					err := json.Unmarshal([]byte(metricString[index:]), &jsonbMap)
					if err != nil {
						p.ErrorChan <- ConcurrentError{
							Err:       errors.Wrap(err, "failed to unmarshal json"),
							Component: "Parser",
						}

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

				// Add this sample to the values to be written
				parsedSamples = append(parsedSamples, []any{
					id,
					toTimestamp(sample.Timestamp.UnixNano() / 1000000), //nolint:mnd // Pouet
					sample.Value,
				})
			}

			// Add the parsed samples to the values rows
			// This is done outside the loop to push values by batch
			p.valuesRows = append(p.valuesRows, parsedSamples...)
		}
	}
}

//nolint:mnd // Constants would reduce readability
func toTimestamp(milliseconds int64) time.Time {
	sec := milliseconds / 1000
	nanoSec := (milliseconds - (sec * 1000)) * 1000000

	return time.Unix(sec, nanoSec).UTC()
}

// transformToMetricString converts a Prometheus metric to a string representation.
// 	It takes a model.Metric as input and returns a string in the format:
// 	net_storage_mb{
//  	"instance": "prometheus-operator-prometheus.metalk8s-monitoring.svc:9090",
//  	"job": "federate-prometheus",
//  	"long_term": "true",
//  	"prometheus": "metalk8s-monitoring/prometheus-operator-prometheus",
//  	"prometheus_replica": "prometheus-prometheus-operator-prometheus-0"
// 	}
func transformToMetricString(m model.Metric) string {
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
