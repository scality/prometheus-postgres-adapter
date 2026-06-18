package storeapi_test

import (
	"prometheus-postgres-adapter/pkg/presentation/storeapi"
	"testing"

	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/thanos/pkg/store/storepb"
)

func TestPromQueryFromSeriesRequest(t *testing.T) {
	t.Run("maps time range and all matcher types", func(t *testing.T) {
		req := &storepb.SeriesRequest{
			MinTime: 1000,
			MaxTime: 2000,
			Matchers: []storepb.LabelMatcher{
				{Type: storepb.LabelMatcher_EQ, Name: "__name__", Value: "cluster_storage_free"},
				{Type: storepb.LabelMatcher_NEQ, Name: "job", Value: "x"},
				{Type: storepb.LabelMatcher_RE, Name: "instance", Value: "prod.*"},
				{Type: storepb.LabelMatcher_NRE, Name: "region", Value: "us.*"},
			},
		}

		query, matched, err := storeapi.PromQueryFromSeriesRequest(req, nil)
		require.NoError(t, err)
		assert.True(t, matched)

		assert.Equal(t, int64(1000), query.StartTimestampMs)
		assert.Equal(t, int64(2000), query.EndTimestampMs)

		require.Len(t, query.Matchers, 4)
		assert.Equal(t, prompb.LabelMatcher_EQ, query.Matchers[0].Type)
		assert.Equal(t, "__name__", query.Matchers[0].Name)
		assert.Equal(t, "cluster_storage_free", query.Matchers[0].Value)
		assert.Equal(t, prompb.LabelMatcher_NEQ, query.Matchers[1].Type)
		assert.Equal(t, prompb.LabelMatcher_RE, query.Matchers[2].Type)
		assert.Equal(t, prompb.LabelMatcher_NRE, query.Matchers[3].Type)
	})

	t.Run("empty matchers yields empty query", func(t *testing.T) {
		query, matched, err := storeapi.PromQueryFromSeriesRequest(
			&storepb.SeriesRequest{MinTime: 1, MaxTime: 2},
			nil,
		)
		require.NoError(t, err)
		assert.True(t, matched)
		assert.Empty(t, query.Matchers)
	})

	t.Run("external-label matcher is validated and dropped from the query", func(t *testing.T) {
		req := &storepb.SeriesRequest{
			Matchers: []storepb.LabelMatcher{
				{Type: storepb.LabelMatcher_EQ, Name: "__name__", Value: "cluster_storage_free"},
				{Type: storepb.LabelMatcher_EQ, Name: "source", Value: "postgres-adapter"},
			},
		}

		query, matched, err := storeapi.PromQueryFromSeriesRequest(
			req,
			map[string]string{"source": "postgres-adapter"},
		)
		require.NoError(t, err)
		assert.True(t, matched)

		// The "source" external-label matcher is removed; only __name__ reaches the SQL.
		require.Len(t, query.Matchers, 1)
		assert.Equal(t, "__name__", query.Matchers[0].Name)
	})

	t.Run("non-matching external-label matcher excludes the store", func(t *testing.T) {
		req := &storepb.SeriesRequest{
			Matchers: []storepb.LabelMatcher{
				{Type: storepb.LabelMatcher_EQ, Name: "source", Value: "something-else"},
			},
		}

		query, matched, err := storeapi.PromQueryFromSeriesRequest(
			req,
			map[string]string{"source": "postgres-adapter"},
		)
		require.NoError(t, err)
		assert.False(t, matched)
		assert.Nil(t, query)
	})

	t.Run("error on unknown matcher type", func(t *testing.T) {
		req := &storepb.SeriesRequest{
			Matchers: []storepb.LabelMatcher{
				{Type: storepb.LabelMatcher_Type(999), Name: "x", Value: "y"},
			},
		}

		_, _, err := storeapi.PromQueryFromSeriesRequest(req, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported label matcher type")
	})
}
