package storeapi_test

import (
	"context"
	"io"
	"log/slog"
	"math"
	"prometheus-postgres-adapter/pkg/domain"
	"prometheus-postgres-adapter/pkg/presentation/storeapi"
	"testing"
	"time"

	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/thanos/pkg/store/storepb"
)

type mockQuerier struct {
	samples     []*domain.SamplesReadFromDatabase
	minTime     int64
	hasData     bool
	labelNames  []string
	labelValues map[string][]string
}

func (m *mockQuerier) QueryDatabaseSamples(
	_ context.Context,
	_ string,
	_ ...any,
) ([]*domain.SamplesReadFromDatabase, error) {
	return m.samples, nil
}

func (m *mockQuerier) MinSampleTimestamp(_ context.Context) (int64, bool, error) {
	return m.minTime, m.hasData, nil
}

func (m *mockQuerier) LabelNames(_ context.Context) ([]string, error) {
	return m.labelNames, nil
}

func (m *mockQuerier) LabelValues(_ context.Context, label string) ([]string, error) {
	return m.labelValues[label], nil
}

type mockBuilder struct{}

func (mockBuilder) BuildSQLQuery(*prompb.Query) (string, error) { return "SELECT 1", nil }

type fakeSeriesServer struct {
	storepb.Store_SeriesServer
	ctx       context.Context //nolint:containedctx // Test stub for the gRPC stream.
	responses []*storepb.SeriesResponse
}

func (f *fakeSeriesServer) Context() context.Context { return f.ctx }

func (f *fakeSeriesServer) Send(response *storepb.SeriesResponse) error {
	f.responses = append(f.responses, response)

	return nil
}

func newTestServer(querier storeapi.Querier, externalLabels map[string]string) *storeapi.Server {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	return storeapi.NewServer(logger, mockBuilder{}, querier, externalLabels)
}

func TestServer_Info(t *testing.T) {
	t.Run("with data advertises the oldest timestamp and external labels", func(t *testing.T) {
		server := newTestServer(
			&mockQuerier{minTime: 1000, hasData: true},
			map[string]string{"source": "postgres-adapter"},
		)

		resp, err := server.Info(context.Background(), nil)
		require.NoError(t, err)

		assert.Equal(t, "store", resp.ComponentType)
		require.NotNil(t, resp.Store)
		assert.Equal(t, int64(1000), resp.Store.MinTime)
		assert.Equal(t, int64(math.MaxInt64), resp.Store.MaxTime)

		require.Len(t, resp.LabelSets, 1)
		assert.Equal(t, "postgres-adapter", labelMap(resp.LabelSets[0].Labels)["source"])
	})

	t.Run("empty database advertises an unreachable min time", func(t *testing.T) {
		server := newTestServer(&mockQuerier{hasData: false}, nil)

		resp, err := server.Info(context.Background(), nil)
		require.NoError(t, err)
		assert.Equal(t, int64(math.MaxInt64), resp.Store.MinTime)
		assert.Empty(t, resp.LabelSets)
	})
}

func TestServer_Series(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	querier := &mockQuerier{
		samples: []*domain.SamplesReadFromDatabase{
			dbRow("cluster_storage_free", map[string]string{"instance": "a"}, base, 1),
			dbRow("cluster_storage_free", map[string]string{"instance": "a"}, base.Add(time.Hour), 2),
		},
	}

	server := newTestServer(querier, map[string]string{"source": "postgres-adapter"})

	stream := &fakeSeriesServer{ctx: context.Background()}
	err := server.Series(&storepb.SeriesRequest{MinTime: 0, MaxTime: math.MaxInt64}, stream)
	require.NoError(t, err)

	require.Len(t, stream.responses, 1)

	series := stream.responses[0].GetSeries()
	require.NotNil(t, series)

	lset := labelMap(series.Labels)
	assert.Equal(t, "cluster_storage_free", lset["__name__"])
	assert.Equal(t, "a", lset["instance"])
	assert.Equal(t, "postgres-adapter", lset["source"])

	samples := decodeChunks(t, series.Chunks)
	require.Len(t, samples, 2)
}

func TestServer_Series_SkipChunks(t *testing.T) {
	querier := &mockQuerier{
		samples: []*domain.SamplesReadFromDatabase{
			dbRow("m", map[string]string{"instance": "a"}, time.Unix(0, 0), 1),
		},
	}

	server := newTestServer(querier, nil)

	stream := &fakeSeriesServer{ctx: context.Background()}
	err := server.Series(&storepb.SeriesRequest{SkipChunks: true}, stream)
	require.NoError(t, err)

	require.Len(t, stream.responses, 1)
	assert.Empty(t, stream.responses[0].GetSeries().Chunks)
}

func TestServer_LabelNames(t *testing.T) {
	server := newTestServer(
		&mockQuerier{labelNames: []string{"instance", "job"}},
		map[string]string{"source": "postgres-adapter"},
	)

	resp, err := server.LabelNames(context.Background(), &storepb.LabelNamesRequest{})
	require.NoError(t, err)

	assert.Equal(t, []string{"__name__", "instance", "job", "source"}, resp.Names)
}

func TestServer_LabelValues(t *testing.T) {
	server := newTestServer(
		&mockQuerier{labelValues: map[string][]string{"instance": {"a", "b"}}},
		map[string]string{"source": "postgres-adapter"},
	)

	t.Run("external label returns its configured value", func(t *testing.T) {
		resp, err := server.LabelValues(context.Background(), &storepb.LabelValuesRequest{Label: "source"})
		require.NoError(t, err)
		assert.Equal(t, []string{"postgres-adapter"}, resp.Values)
	})

	t.Run("stored label returns database values", func(t *testing.T) {
		resp, err := server.LabelValues(context.Background(), &storepb.LabelValuesRequest{Label: "instance"})
		require.NoError(t, err)
		assert.Equal(t, []string{"a", "b"}, resp.Values)
	})
}

func TestServer_Series_ExternalLabelMatcher(t *testing.T) {
	querier := &mockQuerier{
		samples: []*domain.SamplesReadFromDatabase{
			dbRow("cluster_storage_free", map[string]string{"instance": "a"}, time.Unix(0, 0), 1),
		},
	}
	server := newTestServer(querier, map[string]string{"source": "postgres-adapter"})

	t.Run("a matching external-label matcher still returns the series", func(t *testing.T) {
		stream := &fakeSeriesServer{ctx: context.Background()}
		err := server.Series(&storepb.SeriesRequest{
			Matchers: []storepb.LabelMatcher{
				{Type: storepb.LabelMatcher_EQ, Name: "source", Value: "postgres-adapter"},
			},
		}, stream)
		require.NoError(t, err)

		require.Len(t, stream.responses, 1)
		assert.Equal(t, "postgres-adapter", labelMap(stream.responses[0].GetSeries().Labels)["source"])
	})

	t.Run("a non-matching external-label matcher returns nothing", func(t *testing.T) {
		stream := &fakeSeriesServer{ctx: context.Background()}
		err := server.Series(&storepb.SeriesRequest{
			Matchers: []storepb.LabelMatcher{
				{Type: storepb.LabelMatcher_EQ, Name: "source", Value: "other"},
			},
		}, stream)
		require.NoError(t, err)
		assert.Empty(t, stream.responses)
	})
}
