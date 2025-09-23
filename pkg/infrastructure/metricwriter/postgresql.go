package metricwriter

import (
	"context"
	"encoding/json"
	"fmt"
	"prometheus-postgres-adapter/pkg/presentation/database"
	"prometheus-postgres-adapter/pkg/presentation/messagequeue"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
)

const (
	postgreSQLTickerPeriod             = 10 * time.Millisecond
	postgreSQLSelectMetricsLabelsQuery = `
		SELECT metric_id, metric_name_label, metric_labels
		FROM metric_labels
	`
	postgreSQLGetNextMetricIDQuery = `
		SELECT COALESCE(MAX(metric_id), 0) + 1 as next_id
		FROM metric_labels
	`
	postgreSQLInsertMetricLabelQuery = `
		INSERT INTO metric_labels (metric_id, metric_name, metric_name_label, metric_labels)
		SELECT $1, $2, $3, $4
		WHERE NOT EXISTS (
			SELECT 1 FROM metric_labels WHERE metric_name_label = $3
		)
		RETURNING metric_id
	`

	postgreSQLLabelRowLength = 4
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

		// Mutex to protect metric ID generation from race conditions
		metricIDMutex sync.Mutex

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

	// Recover and register existing metrics
	err := postgreSQL.registerExistingMetrics(ctx)
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
		metricID, metricIDOk := result["metric_id"].(int64)
		if !metricIDOk {
			// Handle different possible integer types from database
			metricIDInt, ok := result["metric_id"].(int)
			if !ok {
				return errors.New("failed to cast metric_id to int64")
			}

			metricID = int64(metricIDInt)
		}

		metricNameLabel, ok := result["metric_name_label"].(string)
		if !ok {
			return errors.New("failed to cast metric_name_label to string")
		}

		// Store the metric ID using the full metric string as key
		// This ensures proper mapping during metric ingestion
		p.syncMap.Store(metricNameLabel, metricID)
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

//nolint:gocognit,funlen,cyclop // This function is not that complicated
func (p *PostgreSQL) save(ctx context.Context) error {
	// Create copies of the data to avoid issues with deferred cleanup
	labelRowsCopy := make([][]any, len(p.labelRows))
	copy(labelRowsCopy, p.labelRows)

	valuesRowsCopy := make([][]any, len(p.valuesRows))
	copy(valuesRowsCopy, p.valuesRows)

	// Always reset the rows, regardless of success or failure
	defer func() {
		p.labelRows = nil
		p.valuesRows = nil
	}()

	// Save labels using atomic insert to prevent duplicates
	if len(labelRowsCopy) > 0 {
		for _, labelRow := range labelRowsCopy {
			if len(labelRow) != postgreSQLLabelRowLength {
				continue // Skip invalid rows
			}

			// Use atomic insert query that prevents duplicates
			_, err := p.postgreSQLClient.QueryToMap(
				ctx,
				postgreSQLInsertMetricLabelQuery,
				labelRow[0], // metric_id
				labelRow[1], // metric_name
				labelRow[2], // metric_name_label
				labelRow[3], // metric_labels
			)
			if err != nil {
				// If it's not a duplicate key error, it's a real problem
				if !strings.Contains(err.Error(), "duplicate key") &&
					!strings.Contains(err.Error(), "violates unique constraint") {
					return errors.Wrap(err, "failed to save labels")
				}
				// Otherwise, the label already exists, which is fine
			}
		}
	}

	// Only save values if we have any to save
	if len(valuesRowsCopy) > 0 {
		err := p.postgreSQLClient.CopyRows(
			ctx,
			"metric_values",
			[]string{
				"metric_id",
				"metric_time",
				"metric_value",
			},
			valuesRowsCopy,
		)
		if err != nil {
			return errors.Wrap(err, "failed to save values")
		}
	}

	return nil
}

//nolint:gocognit,funlen,cyclop // Splitting this function would make it even more complex
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
				if !ok { //nolint:nestif // This needs refactoring
					// Thread-safe metric ID generation using mutex protection
					p.metricIDMutex.Lock()

					// Double-check pattern: another goroutine might have added it while we waited
					if existingID, exists := p.syncMap.Load(metricString); exists {
						p.metricIDMutex.Unlock()

						id = existingID
					} else {
						// Generate new metric ID safely
						nextIDResults, err := p.postgreSQLClient.QueryToMap(ctx, postgreSQLGetNextMetricIDQuery)
						if err != nil {
							p.metricIDMutex.Unlock()
							p.ErrorChan <- ConcurrentError{
								Err:       errors.Wrap(err, "failed to get next metric ID"),
								Component: "Parser",
							}

							continue
						}

						if len(nextIDResults) == 0 {
							p.metricIDMutex.Unlock()
							p.ErrorChan <- ConcurrentError{
								Err:       errors.New("no result from next metric ID query"),
								Component: "Parser",
							}

							continue
						}

						nextID, ok := nextIDResults[0]["next_id"].(int64)
						if !ok {
							// Handle different possible integer types from database
							nextIDInt, ok := nextIDResults[0]["next_id"].(int)
							if !ok {
								p.metricIDMutex.Unlock()
								p.ErrorChan <- ConcurrentError{
									Err:       errors.New("failed to cast next metric ID to int64"),
									Component: "Parser",
								}

								continue
							}

							nextID = int64(nextIDInt)
						}

						// Store the metric ID in the sync map first to prevent duplicate processing
						p.syncMap.Store(metricString, nextID)
						p.metricIDMutex.Unlock()

						index := strings.Index(metricString, "{")
						jsonbMap := make(map[string]any)

						err = json.Unmarshal([]byte(metricString[index:]), &jsonbMap)
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
//
//	It takes a model.Metric as input and returns a string in the format:
//	net_storage_mb{
//	 	"instance": "prometheus-operator-prometheus.metalk8s-monitoring.svc:9090",
//	 	"job": "federate-prometheus",
//	 	"long_term": "true",
//	 	"prometheus": "metalk8s-monitoring/prometheus-operator-prometheus",
//	 	"prometheus_replica": "prometheus-prometheus-operator-prometheus-0"
//	}
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

	// There should always be at least one label
	// 	As the prometheus instance is configured to fetch metrics with the ltm label to true
	if numLabels > 0 {
		sort.Strings(labelStrings)

		return fmt.Sprintf("%s{%s}", metricName, strings.Join(labelStrings, ", "))
	}

	if hasName {
		return string(metricName)
	}

	return "{}"
}
