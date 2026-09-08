package querybuilder_test

import (
	"prometheus-postgres-adapter/pkg/infrastructure/querybuilder"
	"testing"
	"time"

	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQL_BuildSQLQuery(t *testing.T) {
	builder := querybuilder.NewSQL()

	t.Run("metric name equality", func(t *testing.T) {
		query := &prompb.Query{
			StartTimestampMs: 1000000,
			EndTimestampMs:   2000000,
			Matchers: []*prompb.LabelMatcher{
				{Type: prompb.LabelMatcher_EQ, Name: "__name__", Value: "cpu_usage"},
			},
		}

		sql, err := builder.BuildSQLQuery(query)
		require.NoError(t, err)
		assert.Contains(t, sql, "l.metric_name = 'cpu_usage'")
		assert.Contains(t, sql, "v.metric_time >= '")
		assert.Contains(t, sql, "v.metric_time <= '")
	})

	t.Run("label equality", func(t *testing.T) {
		query := &prompb.Query{
			StartTimestampMs: 1000000,
			EndTimestampMs:   2000000,
			Matchers: []*prompb.LabelMatcher{
				{Type: prompb.LabelMatcher_EQ, Name: "host", Value: "server1"},
			},
		}

		sql, err := builder.BuildSQLQuery(query)
		require.NoError(t, err)
		assert.Contains(t, sql, "l.metric_labels @> '{\"host\":\"server1\"}'")
	})

	t.Run("empty label value equality", func(t *testing.T) {
		query := &prompb.Query{
			StartTimestampMs: 1000000,
			EndTimestampMs:   2000000,
			Matchers: []*prompb.LabelMatcher{
				{Type: prompb.LabelMatcher_EQ, Name: "host", Value: ""},
			},
		}

		sql, err := builder.BuildSQLQuery(query)
		require.NoError(t, err)
		assert.Contains(
			t,
			sql,
			"((l.metric_labels ? 'host') = false OR (l.metric_labels->>'host' = ''))")
	})

	t.Run("empty metric name equality", func(t *testing.T) {
		query := &prompb.Query{
			StartTimestampMs: 1000000,
			EndTimestampMs:   2000000,
			Matchers: []*prompb.LabelMatcher{
				{Type: prompb.LabelMatcher_EQ, Name: "__name__", Value: ""},
			},
		}

		sql, err := builder.BuildSQLQuery(query)
		require.NoError(t, err)
		assert.Contains(t, sql, "(l.metric_name IS NULL OR l.metric_name = '')")
	})

	t.Run("label inequality", func(t *testing.T) {
		query := &prompb.Query{
			StartTimestampMs: 1000000,
			EndTimestampMs:   2000000,
			Matchers: []*prompb.LabelMatcher{
				{Type: prompb.LabelMatcher_NEQ, Name: "host", Value: "server1"},
			},
		}

		sql, err := builder.BuildSQLQuery(query)
		require.NoError(t, err)
		assert.Contains(t, sql, "l.metric_labels->>'host' != 'server1'")
	})

	t.Run("metric name inequality", func(t *testing.T) {
		query := &prompb.Query{
			StartTimestampMs: 1000000,
			EndTimestampMs:   2000000,
			Matchers: []*prompb.LabelMatcher{
				{Type: prompb.LabelMatcher_NEQ, Name: "__name__", Value: "cpu_usage"},
			},
		}

		sql, err := builder.BuildSQLQuery(query)
		require.NoError(t, err)
		assert.Contains(t, sql, "l.metric_name != 'cpu_usage'")
	})

	t.Run("label regex match", func(t *testing.T) {
		query := &prompb.Query{
			StartTimestampMs: 1000000,
			EndTimestampMs:   2000000,
			Matchers: []*prompb.LabelMatcher{
				{Type: prompb.LabelMatcher_RE, Name: "host", Value: "server.*"},
			},
		}

		sql, err := builder.BuildSQLQuery(query)
		require.NoError(t, err)
		assert.Contains(t, sql, "l.metric_labels->>'host' ~ '^server.*$'")
	})

	t.Run("metric name regex match", func(t *testing.T) {
		query := &prompb.Query{
			StartTimestampMs: 1000000,
			EndTimestampMs:   2000000,
			Matchers: []*prompb.LabelMatcher{
				{Type: prompb.LabelMatcher_RE, Name: "__name__", Value: "cpu.*"},
			},
		}

		sql, err := builder.BuildSQLQuery(query)
		require.NoError(t, err)
		assert.Contains(t, sql, "l.metric_name ~ '^cpu.*$'")
	})

	t.Run("label regex not match", func(t *testing.T) {
		query := &prompb.Query{
			StartTimestampMs: 1000000,
			EndTimestampMs:   2000000,
			Matchers: []*prompb.LabelMatcher{
				{Type: prompb.LabelMatcher_NRE, Name: "host", Value: "server.*"},
			},
		}

		sql, err := builder.BuildSQLQuery(query)
		require.NoError(t, err)
		assert.Contains(t, sql, "l.metric_labels->>'host' !~ '^server.*$'")
	})

	t.Run("metric name regex not match", func(t *testing.T) {
		query := &prompb.Query{
			StartTimestampMs: 1000000,
			EndTimestampMs:   2000000,
			Matchers: []*prompb.LabelMatcher{
				{Type: prompb.LabelMatcher_NRE, Name: "__name__", Value: "cpu.*"},
			},
		}

		sql, err := builder.BuildSQLQuery(query)
		require.NoError(t, err)
		assert.Contains(t, sql, "l.metric_name !~ '^cpu.*$'")
	})

	t.Run("regex with anchors", func(t *testing.T) {
		testCases := []struct {
			name          string
			inputRegex    string
			expectedRegex string
		}{
			{"empty regex", "", ""},
			{"already anchored", "^cpu$", "^cpu$"},
			{"start anchored", "^cpu", "^cpu$"},
			{"end anchored", "cpu$", "^cpu$"},
			{"unanchored", "cpu", "^cpu$"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				query := &prompb.Query{
					StartTimestampMs: 1000000,
					EndTimestampMs:   2000000,
					Matchers: []*prompb.LabelMatcher{
						{Type: prompb.LabelMatcher_RE, Name: "host", Value: tc.inputRegex},
					},
				}

				sql, err := builder.BuildSQLQuery(query)
				require.NoError(t, err)

				expectedPattern := "l.metric_labels->>'host' ~ '" + tc.expectedRegex + "'"
				assert.Contains(t, sql, expectedPattern)
			})
		}
	})

	t.Run("complex query", func(t *testing.T) {
		query := &prompb.Query{
			StartTimestampMs: 1000000,
			EndTimestampMs:   2000000,
			Matchers: []*prompb.LabelMatcher{
				{Type: prompb.LabelMatcher_EQ, Name: "__name__", Value: "cpu_usage"},
				{Type: prompb.LabelMatcher_EQ, Name: "host", Value: "server1"},
				{Type: prompb.LabelMatcher_RE, Name: "instance", Value: "prod.*"},
				{Type: prompb.LabelMatcher_NEQ, Name: "region", Value: "us-west-1"},
			},
		}

		sql, err := builder.BuildSQLQuery(query)
		require.NoError(t, err)
		assert.Contains(t, sql, "l.metric_name = 'cpu_usage'")
		assert.Contains(t, sql, "l.metric_labels @> '{\"host\":\"server1\"}'")
		assert.Contains(t, sql, "l.metric_labels->>'instance' ~ '^prod.*$'")
		assert.Contains(t, sql, "l.metric_labels->>'region' != 'us-west-1'")
	})

	t.Run("timestamp formatting", func(t *testing.T) {
		startMs := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
		endMs := time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC).UnixMilli()

		query := &prompb.Query{
			StartTimestampMs: startMs,
			EndTimestampMs:   endMs,
			Matchers: []*prompb.LabelMatcher{
				{Type: prompb.LabelMatcher_EQ, Name: "__name__", Value: "cpu_usage"},
			},
		}

		sql, err := builder.BuildSQLQuery(query)
		require.NoError(t, err)
		assert.Contains(t, sql, "v.metric_time >= '2023-01-01T00:00:00Z'")
		assert.Contains(t, sql, "v.metric_time <= '2023-01-02T00:00:00Z'")
	})

	t.Run("escape single quotes", func(t *testing.T) {
		query := &prompb.Query{
			StartTimestampMs: 1000000,
			EndTimestampMs:   2000000,
			Matchers: []*prompb.LabelMatcher{
				{Type: prompb.LabelMatcher_EQ, Name: "__name__", Value: "metric'with'quotes"},
			},
		}

		sql, err := builder.BuildSQLQuery(query)
		require.NoError(t, err)
		assert.Contains(t, sql, "l.metric_name = 'metric''with''quotes'")
	})

	t.Run("error on unknown matcher type", func(t *testing.T) {
		query := &prompb.Query{
			StartTimestampMs: 1000000,
			EndTimestampMs:   2000000,
			Matchers: []*prompb.LabelMatcher{
				{Type: 999, Name: "host", Value: "server1"}, // Invalid matcher type
			},
		}

		_, err := builder.BuildSQLQuery(query)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown match type")
	})

	t.Run("error on unknown metric name matcher type", func(t *testing.T) {
		query := &prompb.Query{
			StartTimestampMs: 1000000,
			EndTimestampMs:   2000000,
			Matchers: []*prompb.LabelMatcher{
				{Type: 999, Name: "__name__", Value: "cpu_usage"}, // Invalid matcher type
			},
		}

		_, err := builder.BuildSQLQuery(query)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown metric name match type")
	})
}

func TestSQL_BuildLabelsPredicate(t *testing.T) {
	builder := querybuilder.NewSQL()

	t.Run("no matcher selects every series", func(t *testing.T) {
		predicate, err := builder.BuildLabelsPredicate(nil)
		require.NoError(t, err)
		assert.Equal(t, "TRUE", predicate)
	})

	t.Run("metric name equality", func(t *testing.T) {
		predicate, err := builder.BuildLabelsPredicate([]*prompb.LabelMatcher{
			{Type: prompb.LabelMatcher_EQ, Name: "__name__", Value: "cpu_usage"},
		})
		require.NoError(t, err)
		assert.Equal(t, "l.metric_name = 'cpu_usage'", predicate)
	})

	t.Run("label equality uses JSONB containment", func(t *testing.T) {
		predicate, err := builder.BuildLabelsPredicate([]*prompb.LabelMatcher{
			{Type: prompb.LabelMatcher_EQ, Name: "host", Value: "server1"},
		})
		require.NoError(t, err)
		assert.Equal(t, `l.metric_labels @> '{"host":"server1"}'`, predicate)
	})

	t.Run("several matchers are joined with AND", func(t *testing.T) {
		predicate, err := builder.BuildLabelsPredicate([]*prompb.LabelMatcher{
			{Type: prompb.LabelMatcher_EQ, Name: "__name__", Value: "cpu_usage"},
			{Type: prompb.LabelMatcher_RE, Name: "instance", Value: "prod.*"},
			{Type: prompb.LabelMatcher_EQ, Name: "host", Value: "server1"},
		})
		require.NoError(t, err)
		assert.Equal(
			t,
			`l.metric_name = 'cpu_usage'`+
				` AND l.metric_labels->>'instance' ~ '^prod.*$'`+
				` AND l.metric_labels @> '{"host":"server1"}'`,
			predicate,
		)
	})

	t.Run("no time bound and no samples table", func(t *testing.T) {
		predicate, err := builder.BuildLabelsPredicate([]*prompb.LabelMatcher{
			{Type: prompb.LabelMatcher_EQ, Name: "__name__", Value: "cpu_usage"},
		})
		require.NoError(t, err)
		assert.NotContains(t, predicate, "metric_time")
		assert.NotContains(t, predicate, "metric_values")
	})

	t.Run("escape single quotes", func(t *testing.T) {
		predicate, err := builder.BuildLabelsPredicate([]*prompb.LabelMatcher{
			{Type: prompb.LabelMatcher_EQ, Name: "__name__", Value: "metric'with'quotes"},
		})
		require.NoError(t, err)
		assert.Equal(t, "l.metric_name = 'metric''with''quotes'", predicate)
	})

	t.Run("error on unknown matcher type", func(t *testing.T) {
		_, err := builder.BuildLabelsPredicate([]*prompb.LabelMatcher{
			{Type: 999, Name: "host", Value: "server1"},
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown match type")
	})
}
