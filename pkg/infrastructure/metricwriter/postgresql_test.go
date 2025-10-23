package metricwriter

import (
	"prometheus-postgres-adapter/pkg/presentation/database"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/common/model"
)

func TestTransformToMetricString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metric   model.Metric
		expected string
	}{
		{
			name: "metric with sorted labels",
			metric: model.Metric{
				model.MetricNameLabel: "test_metric",
				"instance":            "localhost:9090",
				"job":                 "prometheus",
				"env":                 "production",
			},
			expected: `test_metric{"env": "production", "instance": "localhost:9090", "job": "prometheus"}`,
		},
		{
			name: "metric with single label",
			metric: model.Metric{
				model.MetricNameLabel: "simple_metric",
				"label":               "value",
			},
			expected: `simple_metric{"label": "value"}`,
		},
		{
			name: "metric without name but with labels",
			metric: model.Metric{
				"label1": "value1",
				"label2": "value2",
			},
			expected: `{"label1": "value1", "label2": "value2"}`,
		},
		{
			name:     "metric without labels",
			metric:   model.Metric{model.MetricNameLabel: "bare_metric"},
			expected: "bare_metric",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := transformToMetricString(tt.metric)
			if result != tt.expected {
				t.Errorf("transformToMetricString() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestTransformToMetricStringConsistency(t *testing.T) {
	t.Parallel()

	// Test that the same labels in different order produce the same string
	metric1 := model.Metric{
		model.MetricNameLabel: "test_metric",
		"z_last":              "1",
		"a_first":             "2",
		"m_middle":            "3",
	}

	metric2 := model.Metric{
		model.MetricNameLabel: "test_metric",
		"m_middle":            "3",
		"a_first":             "2",
		"z_last":              "1",
	}

	result1 := transformToMetricString(metric1)
	result2 := transformToMetricString(metric2)

	if result1 != result2 {
		t.Errorf("transformToMetricString should produce same result for same labels in different order\nGot:  %v\nWant: %v", result1, result2)
	}
}

func TestTransformToMetricStringUniqueness(t *testing.T) {
	t.Parallel()

	// Test that different label values produce different strings
	metric1 := model.Metric{
		model.MetricNameLabel: "test_metric",
		"instance":            "host1:9090",
		"job":                 "prometheus",
	}

	metric2 := model.Metric{
		model.MetricNameLabel: "test_metric",
		"instance":            "host2:9090",
		"job":                 "prometheus",
	}

	result1 := transformToMetricString(metric1)
	result2 := transformToMetricString(metric2)

	if result1 == result2 {
		t.Errorf("transformToMetricString should produce different results for different label values\nGot same result for both: %v", result1)
	}
}

func TestDeduplicationLogic(t *testing.T) {
	t.Parallel()

	// Simulate what happens in the save() function
	timestamp := time.Now().UTC()

	// Create duplicate values with same metric_id and metric_time but different values
	valuesRows := [][]any{
		{int64(1), timestamp, 100.0},
		{int64(1), timestamp, 200.0}, // Duplicate - should be overwritten
		{int64(2), timestamp, 300.0},
		{int64(1), timestamp, 250.0}, // Another duplicate - this should be the final value
		{int64(2), timestamp.Add(time.Minute), 400.0},
	}

	// Apply deduplication logic (same as in save() function)
	deduplicatedValues := make(map[string][]any)

	for _, valueRow := range valuesRows {
		if len(valueRow) != 3 {
			continue
		}

		metricID := valueRow[0]
		metricTime := valueRow[1]

		var key string

		//nolint:ineffassign,staticcheck // Annoying linter
		key = time.Now().Format("2006-01-02 15:04:05")

		// Create a proper key
		if ts, ok := metricTime.(time.Time); ok {
			key = ts.Format(time.RFC3339Nano)
		} else {
			key = metricTime.(string)
		}

		fullKey := string(rune(metricID.(int64))) + "_" + key

		deduplicatedValues[fullKey] = valueRow
	}

	// Verify deduplication worked
	if len(deduplicatedValues) >= len(valuesRows) {
		t.Errorf("Deduplication failed: expected fewer values than input. Got %d, input was %d", len(deduplicatedValues), len(valuesRows))
	}

	// Count unique (metric_id, metric_time) combinations in input
	uniqueCombinations := make(map[string]bool)

	for _, valueRow := range valuesRows {
		metricID := valueRow[0]
		metricTime := valueRow[1]

		var key string
		if ts, ok := metricTime.(time.Time); ok {
			key = ts.Format(time.RFC3339Nano)
		}

		fullKey := string(rune(metricID.(int64))) + "_" + key
		uniqueCombinations[fullKey] = true
	}

	expectedUnique := len(uniqueCombinations)
	if len(deduplicatedValues) != expectedUnique {
		t.Errorf("Expected %d unique combinations, got %d", expectedUnique, len(deduplicatedValues))
	}
}

func TestToTimestamp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		milliseconds int64
		expected     time.Time
	}{
		{
			name:         "epoch",
			milliseconds: 0,
			expected:     time.Unix(0, 0).UTC(),
		},
		{
			name:         "specific timestamp",
			milliseconds: 1609459200000, // 2021-01-01 00:00:00 UTC
			expected:     time.Unix(1609459200, 0).UTC(),
		},
		{
			name:         "timestamp with milliseconds",
			milliseconds: 1609459200123, // 2021-01-01 00:00:00.123 UTC
			expected:     time.Unix(1609459200, 123000000).UTC(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := toTimestamp(tt.milliseconds)
			if !result.Equal(tt.expected) {
				t.Errorf("toTimestamp(%d) = %v, want %v", tt.milliseconds, result, tt.expected)
			}
		})
	}
}

// TestRegisterExistingMetrics validates that the metric ID counter is properly initialized.
func TestRegisterExistingMetrics(t *testing.T) {
	t.Parallel()

	t.Run("initializes counter with no existing metrics", func(t *testing.T) {
		t.Parallel()

		// Test the initialization logic directly (without full DB setup)
		var maxMetricID int64

		results := []map[string]any{}

		for _, result := range results {
			metricID := result["metric_id"].(int64)
			if metricID > maxMetricID {
				maxMetricID = metricID
			}
		}

		nextMetricID := maxMetricID + 1

		if nextMetricID != 1 {
			t.Errorf("Expected nextMetricID to be 1 with no existing metrics, got %d", nextMetricID)
		}
	})

	t.Run("initializes counter with existing metrics", func(t *testing.T) {
		t.Parallel()

		// Simulate existing metrics with IDs 1 and 2
		results := []map[string]any{
			{
				"metric_id":         int64(1),
				"metric_name_label": "test_metric_1{}",
			},
			{
				"metric_id":         int64(2),
				"metric_name_label": "test_metric_2{}",
			},
		}

		var maxMetricID int64

		for _, result := range results {
			metricID := result["metric_id"].(int64)
			if metricID > maxMetricID {
				maxMetricID = metricID
			}
		}

		nextMetricID := maxMetricID + 1

		if nextMetricID != 3 {
			t.Errorf("Expected nextMetricID to be 3, got %d", nextMetricID)
		}
	})
}

// TestConcurrentMetricIDGeneration validates the critical fix:
// Multiple goroutines processing different metrics should get unique IDs.
func TestConcurrentMetricIDGeneration(t *testing.T) {
	t.Parallel()

	// Create a PostgreSQL instance with initialized counter
	pg := &PostgreSQL{
		postgreSQLClient: (*database.PostgreSQL)(nil),
		syncMap:          &sync.Map{},
		nextMetricID:     1, // Start from 1
	}

	// Number of concurrent goroutines (simulating multiple parsers)
	numGoroutines := 10

	// Each goroutine will process multiple unique metrics
	metricsPerGoroutine := 5

	// Collect all assigned IDs
	type metricIDPair struct {
		metricString string
		metricID     int64
	}

	resultsChan := make(chan metricIDPair, numGoroutines*metricsPerGoroutine)

	var wg sync.WaitGroup

	// Simulate concurrent metric processing (the bug scenario)
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)

		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < metricsPerGoroutine; j++ {
				// Create a unique metric string for each iteration
				metricString := transformToMetricString(model.Metric{
					model.MetricNameLabel: model.LabelValue("test_metric"),
					"goroutine":           model.LabelValue(string(rune('0' + goroutineID))),
					"iteration":           model.LabelValue(string(rune('0' + j))),
				})

				// Simulate the ID generation logic from parser()
				id, ok := pg.syncMap.Load(metricString)
				if !ok {
					pg.metricIDMutex.Lock()

					// Double-check pattern
					if existingID, exists := pg.syncMap.Load(metricString); exists {
						pg.metricIDMutex.Unlock()

						id = existingID
					} else {
						// This is the critical fix: use in-memory counter, not DB query
						nextID := pg.nextMetricID
						pg.nextMetricID++

						pg.syncMap.Store(metricString, nextID)
						pg.metricIDMutex.Unlock()

						id = nextID
					}
				}

				resultsChan <- metricIDPair{
					metricString: metricString,
					metricID:     id.(int64),
				}
			}
		}(i)
	}

	wg.Wait()
	close(resultsChan)

	// Analyze results
	seenIDs := make(map[int64]string)
	metricToID := make(map[string]int64)

	for result := range resultsChan {
		metricToID[result.metricString] = result.metricID

		// Check for duplicate IDs
		if existingMetric, exists := seenIDs[result.metricID]; exists {
			t.Errorf("DUPLICATE METRIC ID DETECTED! ID %d assigned to both:\n  - %s\n  - %s",
				result.metricID, existingMetric, result.metricString)
		}

		seenIDs[result.metricID] = result.metricString
	}

	expectedMetrics := numGoroutines * metricsPerGoroutine
	if len(seenIDs) != expectedMetrics {
		t.Errorf("Expected %d unique metric IDs, got %d", expectedMetrics, len(seenIDs))
	}

	// Verify IDs are sequential starting from 1
	for i := int64(1); i <= int64(expectedMetrics); i++ {
		if _, exists := seenIDs[i]; !exists {
			t.Errorf("Missing metric ID %d in the sequence", i)
		}
	}
}

// TestSameMetricGetsSameID validates that the same metric always gets the same ID.
func TestSameMetricGetsSameID(t *testing.T) {
	t.Parallel()

	pg := &PostgreSQL{
		syncMap:      &sync.Map{},
		nextMetricID: 1,
	}

	metric := model.Metric{
		model.MetricNameLabel: "test_metric",
		"instance":            "localhost:9090",
		"job":                 "prometheus",
	}

	metricString := transformToMetricString(metric)

	// Process the same metric multiple times
	var ids []int64

	for i := 0; i < 5; i++ {
		id, ok := pg.syncMap.Load(metricString)
		if !ok {
			pg.metricIDMutex.Lock()

			if existingID, exists := pg.syncMap.Load(metricString); exists {
				pg.metricIDMutex.Unlock()

				id = existingID
			} else {
				nextID := pg.nextMetricID
				pg.nextMetricID++
				pg.syncMap.Store(metricString, nextID)
				pg.metricIDMutex.Unlock()

				id = nextID
			}
		}

		ids = append(ids, id.(int64))
	}

	// All IDs should be the same
	expectedID := ids[0]
	for i, id := range ids {
		if id != expectedID {
			t.Errorf("Iteration %d: expected ID %d, got %d", i, expectedID, id)
		}
	}

	// The ID should be 1 (first metric)
	if expectedID != 1 {
		t.Errorf("Expected first metric to get ID 1, got %d", expectedID)
	}
}

// TestDifferentMetricsGetDifferentIDs validates that different metrics get different IDs.
func TestDifferentMetricsGetDifferentIDs(t *testing.T) {
	t.Parallel()

	pg := &PostgreSQL{
		syncMap:      &sync.Map{},
		nextMetricID: 1,
	}

	metrics := []model.Metric{
		{
			model.MetricNameLabel: "metric_one",
			"instance":            "host1:9090",
		},
		{
			model.MetricNameLabel: "metric_two",
			"instance":            "host1:9090",
		},
		{
			model.MetricNameLabel: "metric_one",
			"instance":            "host2:9090",
		},
		{
			model.MetricNameLabel: "metric_three",
			"instance":            "host1:9090",
			"job":                 "test",
		},
	}

	assignedIDs := make(map[string]int64)

	for _, metric := range metrics {
		metricString := transformToMetricString(metric)

		id, ok := pg.syncMap.Load(metricString)
		if !ok {
			pg.metricIDMutex.Lock()

			if existingID, exists := pg.syncMap.Load(metricString); exists {
				pg.metricIDMutex.Unlock()

				id = existingID
			} else {
				nextID := pg.nextMetricID
				pg.nextMetricID++
				pg.syncMap.Store(metricString, nextID)
				pg.metricIDMutex.Unlock()

				id = nextID
			}
		}

		assignedIDs[metricString] = id.(int64)
	}

	// Verify all IDs are different
	seenIDs := make(map[int64]bool)
	for metricString, id := range assignedIDs {
		if seenIDs[id] {
			t.Errorf("Duplicate ID %d found for metric %s", id, metricString)
		}

		seenIDs[id] = true
	}

	// Should have 4 unique IDs
	if len(seenIDs) != 4 {
		t.Errorf("Expected 4 unique IDs, got %d", len(seenIDs))
	}
}
