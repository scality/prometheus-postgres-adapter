package querybuilder_test

import (
	"math"
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
		assert.Contains(t, sql, "COALESCE(l.metric_labels->>'host', '') = ''")
	})

	t.Run("the four metric name matchers read the column the same way", func(t *testing.T) {
		// metric_name is NOT NULL in the schema this reads. Defending against
		// NULL on the equality alone made the answer to a NULL name depend on
		// which matcher asked, a comparison with NULL being NULL.
		for expected, matcher := range map[string]*prompb.LabelMatcher{
			"l.metric_name = ''":         {Type: prompb.LabelMatcher_EQ, Name: "__name__", Value: ""},
			"l.metric_name = 'up'":       {Type: prompb.LabelMatcher_EQ, Name: "__name__", Value: "up"},
			"l.metric_name != 'up'":      {Type: prompb.LabelMatcher_NEQ, Name: "__name__", Value: "up"},
			"l.metric_name ~ '^(?:up)$'": {Type: prompb.LabelMatcher_RE, Name: "__name__", Value: "up"},
			"l.metric_name !~ '^(?:u)$'": {Type: prompb.LabelMatcher_NRE, Name: "__name__", Value: "u"},
		} {
			t.Run(expected, func(t *testing.T) {
				sql, err := builder.BuildSQLQuery(&prompb.Query{Matchers: []*prompb.LabelMatcher{matcher}})
				require.NoError(t, err)
				assert.Contains(t, sql, expected)
				assert.NotContains(t, sql, "IS NULL")
			})
		}
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
		assert.Contains(t, sql, "COALESCE(l.metric_labels->>'host', '') != 'server1'")
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
		assert.Contains(t, sql, "COALESCE(l.metric_labels->>'host', '') ~ '^(?:server.*)$'")
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
		assert.Contains(t, sql, "l.metric_name ~ '^(?:cpu.*)$'")
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
		assert.Contains(t, sql, "COALESCE(l.metric_labels->>'host', '') !~ '^(?:server.*)$'")
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
		assert.Contains(t, sql, "l.metric_name !~ '^(?:cpu.*)$'")
	})

	t.Run("regex with anchors", func(t *testing.T) {
		testCases := []struct {
			name          string
			inputRegex    string
			expectedRegex string
		}{
			{"empty regex", "", "^(?:)$"},
			{"already anchored", "^cpu$", "^(?:^cpu$)$"},
			{"start anchored", "^cpu", "^(?:^cpu)$"},
			{"end anchored", "cpu$", "^(?:cpu$)$"},
			{"unanchored", "cpu", "^(?:cpu)$"},
			{"alternation", "node|kubelet", "^(?:node|kubelet)$"},
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

				expectedPattern := "COALESCE(l.metric_labels->>'host', '') ~ '" + tc.expectedRegex + "'"
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
		assert.Contains(t, sql, "COALESCE(l.metric_labels->>'instance', '') ~ '^(?:prod.*)$'")
		assert.Contains(t, sql, "COALESCE(l.metric_labels->>'region', '') != 'us-west-1'")
	})

	t.Run("a bound keeps its fraction of a second", func(t *testing.T) {
		// Truncating it would drop the samples of the last second of the range.
		query := &prompb.Query{
			StartTimestampMs: 1700000000250,
			EndTimestampMs:   1700000060750,
			Matchers: []*prompb.LabelMatcher{
				{Type: prompb.LabelMatcher_EQ, Name: "__name__", Value: "cpu_usage"},
			},
		}

		sql, err := builder.BuildSQLQuery(query)
		require.NoError(t, err)
		assert.Contains(t, sql, "v.metric_time >= '2023-11-14T22:13:20.25Z'")
		assert.Contains(t, sql, "v.metric_time <= '2023-11-14T22:14:20.75Z'")
	})

	t.Run("a time range with no bounds", func(t *testing.T) {
		// Prometheus and Thanos say "no bound" with the extremes of int64
		// milliseconds, which are years RFC 3339 cannot write and PostgreSQL
		// cannot store. /api/v1/series without a time range sends exactly that.
		query := &prompb.Query{
			StartTimestampMs: math.MinInt64,
			EndTimestampMs:   math.MaxInt64,
			Matchers: []*prompb.LabelMatcher{
				{Type: prompb.LabelMatcher_EQ, Name: "__name__", Value: "cpu_usage"},
			},
		}

		sql, err := builder.BuildSQLQuery(query)
		require.NoError(t, err)
		assert.Contains(t, sql, "v.metric_time >= '0001-01-01T00:00:00Z'")
		assert.Contains(t, sql, "v.metric_time <= '9999-12-31T23:59:59Z'")
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
				` AND COALESCE(l.metric_labels->>'instance', '') ~ '^(?:prod.*)$'`+
				` AND l.metric_labels @> '{"host":"server1"}'`,
			predicate,
		)
	})

	t.Run("a missing label is matched as an empty value", func(t *testing.T) {
		// From the PromQL docs, a series that does not carry the label at all
		// is matched as if it carried it empty, so != and !~ must select it.
		predicate, err := builder.BuildLabelsPredicate([]*prompb.LabelMatcher{
			{Type: prompb.LabelMatcher_NEQ, Name: "job", Value: "batch"},
		})
		require.NoError(t, err)
		assert.Equal(t, `COALESCE(l.metric_labels->>'job', '') != 'batch'`, predicate)

		predicate, err = builder.BuildLabelsPredicate([]*prompb.LabelMatcher{
			{Type: prompb.LabelMatcher_NRE, Name: "job", Value: "batch"},
		})
		require.NoError(t, err)
		assert.Equal(t, `COALESCE(l.metric_labels->>'job', '') !~ '^(?:batch)$'`, predicate)
	})

	t.Run("two equality matchers on one label select nothing", func(t *testing.T) {
		predicate, err := builder.BuildLabelsPredicate([]*prompb.LabelMatcher{
			{Type: prompb.LabelMatcher_EQ, Name: "job", Value: "a"},
			{Type: prompb.LabelMatcher_EQ, Name: "job", Value: "b"},
		})
		require.NoError(t, err)
		// One containment per matcher: a label holding two values at once is a
		// contradiction the predicate states rather than one to detect.
		assert.Equal(
			t,
			`l.metric_labels @> '{"job":"a"}' AND l.metric_labels @> '{"job":"b"}'`,
			predicate,
		)
	})

	t.Run("a pattern is passed on as it is, inside the anchors", func(t *testing.T) {
		// The builder anchors and escapes; it does not otherwise read the
		// pattern. These are what a client writes, and every one of them means
		// the same to PostgreSQL as it does to RE2.
		for _, tc := range []struct{ pattern, expected string }{
			{`[]a](?:b)`, `^(?:[]a](?:b))$`},
			{`[^]a](?:b)`, `^(?:[^]a](?:b))$`},
			{`[[:digit:][:space:]]+`, `^(?:[[:digit:][:space:]]+)$`},
			{`[0-9]{1,3}`, `^(?:[0-9]{1,3})$`},
			{`[^\W-]+`, `^(?:[^\W-]+)$`},
		} {
			t.Run(tc.pattern, func(t *testing.T) {
				predicate, err := builder.BuildLabelsPredicate([]*prompb.LabelMatcher{
					{Type: prompb.LabelMatcher_RE, Name: "host", Value: tc.pattern},
				})
				require.NoError(t, err)
				assert.Contains(t, predicate, tc.expected)
			})
		}
	})

	t.Run("an escaped parenthesis is left alone", func(t *testing.T) {
		// PostgreSQL reads these perfectly well, so nothing here touches them.
		for _, tc := range []struct{ pattern, expected string }{
			{`prod\(?us\)?`, `^(?:prod\(?us\)?)$`},
			{`a\\(?:b)`, `^(?:a\\(?:b))$`},
		} {
			t.Run(tc.pattern, func(t *testing.T) {
				predicate, err := builder.BuildLabelsPredicate([]*prompb.LabelMatcher{
					{Type: prompb.LabelMatcher_RE, Name: "host", Value: tc.pattern},
				})
				require.NoError(t, err)
				assert.Contains(t, predicate, tc.expected)
			})
		}
	})

	t.Run("the classes both engines have are left alone", func(t *testing.T) {
		// ASCII in RE2, Unicode-aware here: a difference in what they cover
		// rather than in what they are, and one this store lives with.
		predicate, err := builder.BuildLabelsPredicate([]*prompb.LabelMatcher{
			{Type: prompb.LabelMatcher_RE, Name: "host", Value: `\w+\d\s`},
		})
		require.NoError(t, err)
		assert.Contains(t, predicate, `^(?:\w+\d\s)$`)
	})

	t.Run("a non-capturing group is left alone", func(t *testing.T) {
		predicate, err := builder.BuildLabelsPredicate([]*prompb.LabelMatcher{
			{Type: prompb.LabelMatcher_RE, Name: "host", Value: "(?:node)"},
		})
		require.NoError(t, err)
		assert.Contains(t, predicate, "^(?:(?:node))$")
	})

	t.Run("an equality matcher is not read as a pattern", func(t *testing.T) {
		// "(?" is a legal label value, and only a regex matcher makes it one.
		predicate, err := builder.BuildLabelsPredicate([]*prompb.LabelMatcher{
			{Type: prompb.LabelMatcher_EQ, Name: "host", Value: "(?"},
		})
		require.NoError(t, err)
		assert.Equal(t, `l.metric_labels @> '{"host":"(?"}'`, predicate)
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
