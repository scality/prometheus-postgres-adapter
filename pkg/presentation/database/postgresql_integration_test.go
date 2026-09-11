package database_test

import (
	"context"
	"io"
	"log/slog"
	"math"
	"os"
	"prometheus-postgres-adapter/pkg/infrastructure/querybuilder"
	"prometheus-postgres-adapter/pkg/presentation/database"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The unit tests above run against a mock pool, so they can only assert on the
// SQL text. This one executes it: it is the only place where the queries are
// checked to be valid PostgreSQL and to mean what they are meant to mean (the
// JSONB label lookups, the byte-wise ordering, the PromQL matcher semantics).
//
// It needs a throwaway database, whose tables it drops and recreates:
//
//	docker run --rm -d -e POSTGRES_PASSWORD=test -p 5432:5432 postgres:16
//	PPA_TEST_POSTGRES_DSN='postgres://postgres:test@127.0.0.1:5432/postgres?sslmode=disable' \
//	  go test ./pkg/presentation/database/
func TestPostgreSQLAgainstDatabase(t *testing.T) {
	dsn := os.Getenv("PPA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set PPA_TEST_POSTGRES_DSN to a throwaway database to run this test")
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)

	t.Cleanup(pool.Close)

	_, err = pool.Exec(ctx, `
		DROP TABLE IF EXISTS metric_labels;
		CREATE TABLE metric_labels (
			metric_id BIGINT PRIMARY KEY,
			metric_name TEXT NOT NULL,
			metric_name_label TEXT NOT NULL,
			metric_labels jsonb,
			UNIQUE(metric_name, metric_labels)
		);
		INSERT INTO metric_labels VALUES
			(1, 'up',       'up{...}',       '{"instance":"__hidden","job":"node"}'),
			(2, 'up',       'up{...}',       '{"instance":"b"}'),
			(3, 'node_cpu', 'node_cpu{...}', '{"instance":"c","job":"kubelet","cpu":"0"}'),
			(4, '__zz',     '__zz{...}',     '{"instance":"d","broken":null,"empty":"","__meta":"x"}');
	`)
	require.NoError(t, err)

	builder := querybuilder.NewSQL()
	client := database.NewPostgreSQL(slog.New(slog.NewTextHandler(io.Discard, nil)), pool)

	predicate := func(matchers ...*prompb.LabelMatcher) string {
		built, err := builder.BuildLabelsPredicate(matchers)
		require.NoError(t, err)

		return built
	}

	all := predicate()
	upOnly := predicate(&prompb.LabelMatcher{Type: prompb.LabelMatcher_EQ, Name: "__name__", Value: "up"})
	missing := predicate(&prompb.LabelMatcher{Type: prompb.LabelMatcher_EQ, Name: "__name__", Value: "missing"})

	t.Run("label names follow the matchers", func(t *testing.T) {
		// __meta first is the C collation: a locale-aware one ignores the
		// underscores and would sort it after job.
		names, exist, err := client.LabelNames(ctx, all)
		require.NoError(t, err)
		assert.True(t, exist)
		assert.ElementsMatch(t, []string{"__meta", "cpu", "instance", "job"}, names)

		names, exist, err = client.LabelNames(ctx, upOnly)
		require.NoError(t, err)
		assert.True(t, exist)
		assert.ElementsMatch(t, []string{"instance", "job"}, names)

		names, exist, err = client.LabelNames(ctx, missing)
		require.NoError(t, err)
		assert.False(t, exist)
		assert.Empty(t, names)
	})

	t.Run("label values are sorted byte by byte", func(t *testing.T) {
		// A locale-aware collation ignores the underscores and would return
		// __hidden between b and c.
		values, err := client.LabelValues(ctx, "instance", all)
		require.NoError(t, err)
		assert.Equal(t, []string{"__hidden", "b", "c", "d"}, values)
	})

	t.Run("a label with no value of its own is not a label", func(t *testing.T) {
		// A JSON null reads as SQL NULL and an empty value is how a label is
		// absent everywhere else in Prometheus. Neither is a value, so neither
		// is a name either: a dropdown must not offer what resolves to nothing.
		names, _, err := client.LabelNames(ctx, all)
		require.NoError(t, err)
		assert.NotContains(t, names, "broken")
		assert.NotContains(t, names, "empty")

		for _, label := range []string{"broken", "empty"} {
			values, err := client.LabelValues(ctx, label, all)
			require.NoError(t, err)
			assert.Empty(t, values)
		}
	})

	t.Run("a row whose labels are not an object does not fail the request", func(t *testing.T) {
		// jsonb_each_text refuses a scalar with SQLSTATE 22023, so one such row
		// would fail every metadata request for as long as it is stored --
		// every label dropdown in Grafana, over one row nothing wrote. The
		// query reads it as a series carrying no label instead, so the answer
		// has to be the one it was before that row existed.
		before, _, err := client.LabelNames(ctx, all)
		require.NoError(t, err)

		_, err = pool.Exec(ctx, `INSERT INTO metric_labels VALUES (5, 'scalar', 'scalar{}', 'null'::jsonb)`)
		require.NoError(t, err)

		defer func() {
			_, err := pool.Exec(ctx, `DELETE FROM metric_labels WHERE metric_id = 5`)
			require.NoError(t, err)
		}()

		after, exist, err := client.LabelNames(ctx, all)
		require.NoError(t, err)
		assert.True(t, exist)
		assert.ElementsMatch(t, before, after, "it contributes no label name, and hides none")
	})

	t.Run("metric names read the metric_name column, sorted byte by byte", func(t *testing.T) {
		// __zz first is the C collation too: stripped of its underscores it
		// would come last.
		values, err := client.LabelValues(ctx, "__name__", all)
		require.NoError(t, err)
		assert.Equal(t, []string{"__zz", "node_cpu", "up"}, values)
	})

	t.Run("a series missing the label is matched as carrying it empty", func(t *testing.T) {
		// Series 2 has no job label: PromQL selects it on job!="node" and on
		// job!~"node".
		notNode := predicate(&prompb.LabelMatcher{Type: prompb.LabelMatcher_NEQ, Name: "job", Value: "node"})
		values, err := client.LabelValues(ctx, "instance", notNode)
		require.NoError(t, err)
		assert.Equal(t, []string{"b", "c", "d"}, values)

		notNodeRegex := predicate(&prompb.LabelMatcher{Type: prompb.LabelMatcher_NRE, Name: "job", Value: "node"})
		values, err = client.LabelValues(ctx, "instance", notNodeRegex)
		require.NoError(t, err)
		assert.Equal(t, []string{"b", "c", "d"}, values)

		// A series carrying no job at all is matched as carrying it empty.
		emptyJob := predicate(&prompb.LabelMatcher{Type: prompb.LabelMatcher_EQ, Name: "job", Value: ""})
		values, err = client.LabelValues(ctx, "instance", emptyJob)
		require.NoError(t, err)
		assert.Equal(t, []string{"b", "d"}, values)
	})

	t.Run("an alternation is anchored as a whole", func(t *testing.T) {
		anyJob := predicate(&prompb.LabelMatcher{Type: prompb.LabelMatcher_RE, Name: "job", Value: "nod|kubelet"})
		values, err := client.LabelValues(ctx, "instance", anyJob)
		require.NoError(t, err)
		// "nod" must not match "node": only the kubelet series is selected.
		assert.Equal(t, []string{"c"}, values)
	})

	t.Run("series existence follows the predicate", func(t *testing.T) {
		exist, err := client.SeriesExist(ctx, upOnly)
		require.NoError(t, err)
		assert.True(t, exist)

		exist, err = client.SeriesExist(ctx, missing)
		require.NoError(t, err)
		assert.False(t, exist)
	})

	t.Run("the samples query runs", func(t *testing.T) {
		// Series 5 satisfies both matchers below and its only sample sits
		// before the window, so the time bounds are the one thing keeping it
		// out of the answer. Without it every sample stored matched the window
		// and the bounds could have been dropped from the query unnoticed.
		_, err := pool.Exec(ctx, `
			INSERT INTO metric_labels VALUES
				(5, 'up', 'up{...}', '{"instance":"e","job":"node"}');
			DROP TABLE IF EXISTS metric_values;
			CREATE TABLE metric_values (
				metric_id BIGINT, metric_time TIMESTAMPTZ, metric_value FLOAT8
			);
			INSERT INTO metric_values VALUES
				(1, '2026-01-01T00:00:00Z', 1),
				(2, '2026-01-01T00:00:00Z', 2),
				(3, '2026-01-01T00:00:00Z', 3),
				(5, '2024-01-01T00:00:00Z', 5);
		`)
		require.NoError(t, err)

		query, err := builder.BuildSQLQuery(&prompb.Query{
			StartTimestampMs: 1735689600000, // 2025-01-01
			EndTimestampMs:   1798761600000, // 2027-01-01
			Matchers: []*prompb.LabelMatcher{
				{Type: prompb.LabelMatcher_EQ, Name: "__name__", Value: "up"},
				{Type: prompb.LabelMatcher_RE, Name: "job", Value: "node|kubelet"},
			},
		})
		require.NoError(t, err)

		samples, err := client.QueryDatabaseSamples(ctx, query)
		require.NoError(t, err)
		require.Len(t, samples, 1, "series 5 matches the matchers and is out of the window")
		assert.InEpsilon(t, float64(1), samples[0].Value, 0.001)
		assert.Equal(t, "up", samples[0].Name)
	})

	t.Run("the samples query runs without time bounds", func(t *testing.T) {
		// What Thanos forwards for /api/v1/series with no time range: years
		// PostgreSQL cannot store unless the builder clamps them. Series 5's
		// sample, the one the window above excluded, is what says the clamped
		// bounds still reach back.
		query, err := builder.BuildSQLQuery(&prompb.Query{
			StartTimestampMs: math.MinInt64,
			EndTimestampMs:   math.MaxInt64,
			Matchers: []*prompb.LabelMatcher{
				{Type: prompb.LabelMatcher_EQ, Name: "__name__", Value: "up"},
			},
		})
		require.NoError(t, err)

		samples, err := client.QueryDatabaseSamples(ctx, query)
		require.NoError(t, err)
		assert.Len(t, samples, 3)
	})
}
