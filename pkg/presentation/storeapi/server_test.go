package storeapi_test

import (
	"context"
	"errors"
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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockQuerier struct {
	samples     []*domain.SamplesReadFromDatabase
	minTime     int64
	hasData     bool
	labelNames  []string
	labelValues map[string][]string

	seriesExist bool

	seriesExistErr error
	labelNamesErr  error
	labelValuesErr error

	labelNamesPredicate  string
	labelValuesPredicate string
	seriesExistPredicate string
	labelNamesCalls      int
	labelValuesCalls     int
	seriesExistCalls     int
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

func (m *mockQuerier) LabelNames(_ context.Context, predicate string) ([]string, bool, error) {
	m.labelNamesPredicate = predicate
	m.labelNamesCalls++

	return m.labelNames, m.seriesExist, m.labelNamesErr
}

func (m *mockQuerier) LabelValues(_ context.Context, label, predicate string) ([]string, error) {
	m.labelValuesPredicate = predicate
	m.labelValuesCalls++

	return m.labelValues[label], m.labelValuesErr
}

func (m *mockQuerier) SeriesExist(_ context.Context, predicate string) (bool, error) {
	m.seriesExistPredicate = predicate
	m.seriesExistCalls++

	return m.seriesExist, m.seriesExistErr
}

type mockBuilder struct {
	matchers []*prompb.LabelMatcher
	err      error
}

func (m *mockBuilder) BuildSQLQuery(*prompb.Query) (string, error) { return "SELECT 1", m.err }

func (m *mockBuilder) BuildLabelsPredicate(matchers []*prompb.LabelMatcher) (string, error) {
	m.matchers = matchers

	return "PREDICATE", m.err
}

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
	return newTestServerWithBuilder(&mockBuilder{}, querier, externalLabels)
}

func newTestServerWithBuilder(
	builder storeapi.QueryBuilder,
	querier storeapi.Querier,
	externalLabels map[string]string,
) *storeapi.Server {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	return storeapi.NewServer(logger, builder, querier, externalLabels)
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
	externalLabels := map[string]string{"source": "postgres-adapter"}

	t.Run("returns the stored names, the metric name and the external labels", func(t *testing.T) {
		server := newTestServer(
			&mockQuerier{seriesExist: true, labelNames: []string{"instance", "job"}},
			externalLabels,
		)

		resp, err := server.LabelNames(context.Background(), &storepb.LabelNamesRequest{})
		require.NoError(t, err)

		assert.Equal(t, []string{"__name__", "instance", "job", "source"}, resp.Names)
	})

	t.Run("filters the names with the request matchers", func(t *testing.T) {
		builder := &mockBuilder{}
		querier := &mockQuerier{seriesExist: true, labelNames: []string{"instance"}}
		server := newTestServerWithBuilder(builder, querier, externalLabels)

		_, err := server.LabelNames(context.Background(), &storepb.LabelNamesRequest{
			Matchers: []storepb.LabelMatcher{
				{Type: storepb.LabelMatcher_EQ, Name: "__name__", Value: "up"},
				{Type: storepb.LabelMatcher_EQ, Name: "source", Value: "postgres-adapter"},
			},
		})
		require.NoError(t, err)

		// The external-label matcher is validated and dropped, the other one
		// reaches the SQL builder and its predicate reaches the querier.
		require.Len(t, builder.matchers, 1)
		assert.Equal(t, "__name__", builder.matchers[0].Name)
		assert.Equal(t, prompb.LabelMatcher_EQ, builder.matchers[0].Type)
		assert.Equal(t, "PREDICATE", querier.labelNamesPredicate)
	})

	t.Run("returns nothing when no series matches", func(t *testing.T) {
		// Not even the external labels: the store applies them to the series
		// it returns, and it returns none.
		querier := &mockQuerier{seriesExist: false, labelNames: []string{"instance"}}
		server := newTestServer(querier, externalLabels)

		resp, err := server.LabelNames(context.Background(), &storepb.LabelNamesRequest{
			Matchers: []storepb.LabelMatcher{
				{Type: storepb.LabelMatcher_EQ, Name: "__name__", Value: "missing"},
			},
		})
		require.NoError(t, err)
		assert.Empty(t, resp.Names)
	})

	t.Run("reports the labels of a series that carries none but its name", func(t *testing.T) {
		server := newTestServer(&mockQuerier{seriesExist: true, labelNames: nil}, externalLabels)

		resp, err := server.LabelNames(context.Background(), &storepb.LabelNamesRequest{})
		require.NoError(t, err)
		assert.Equal(t, []string{"__name__", "source"}, resp.Names)
	})

	t.Run("a non-matching external-label matcher returns nothing", func(t *testing.T) {
		querier := &mockQuerier{seriesExist: true, labelNames: []string{"instance"}}
		server := newTestServer(querier, externalLabels)

		resp, err := server.LabelNames(context.Background(), &storepb.LabelNamesRequest{
			Matchers: []storepb.LabelMatcher{
				{Type: storepb.LabelMatcher_EQ, Name: "source", Value: "other"},
			},
		})
		require.NoError(t, err)
		assert.Empty(t, resp.Names)
		assert.Equal(t, 0, querier.labelNamesCalls)
		assert.Equal(t, 0, querier.seriesExistCalls)
	})
}

func TestServer_LabelValues(t *testing.T) {
	externalLabels := map[string]string{"source": "postgres-adapter"}

	t.Run("external label requires a matching series, matchers or not", func(t *testing.T) {
		requests := map[string]*storepb.LabelValuesRequest{
			"without matchers": {Label: "source"},
			"with matchers": {
				Label: "source",
				Matchers: []storepb.LabelMatcher{
					{Type: storepb.LabelMatcher_EQ, Name: "__name__", Value: "up"},
				},
			},
		}

		for name, request := range requests {
			t.Run(name, func(t *testing.T) {
				t.Run("with a match it returns the value", func(t *testing.T) {
					querier := &mockQuerier{seriesExist: true}
					server := newTestServer(querier, externalLabels)

					resp, err := server.LabelValues(context.Background(), request)
					require.NoError(t, err)
					assert.Equal(t, []string{"postgres-adapter"}, resp.Values)
					assert.Equal(t, "PREDICATE", querier.seriesExistPredicate)
					// The value comes from the configuration, not from a
					// database lookup of the label.
					assert.Equal(t, 0, querier.labelValuesCalls)
				})

				t.Run("without a match it returns nothing", func(t *testing.T) {
					server := newTestServer(&mockQuerier{seriesExist: false}, externalLabels)

					resp, err := server.LabelValues(context.Background(), request)
					require.NoError(t, err)
					assert.Empty(t, resp.Values)
				})
			})
		}
	})

	t.Run("stored label returns database values", func(t *testing.T) {
		server := newTestServer(
			&mockQuerier{labelValues: map[string][]string{"instance": {"a", "b"}}},
			externalLabels,
		)

		resp, err := server.LabelValues(context.Background(), &storepb.LabelValuesRequest{Label: "instance"})
		require.NoError(t, err)
		assert.Equal(t, []string{"a", "b"}, resp.Values)
	})

	t.Run("filters the values with the request matchers", func(t *testing.T) {
		builder := &mockBuilder{}
		querier := &mockQuerier{labelValues: map[string][]string{"instance": {"a"}}}
		server := newTestServerWithBuilder(builder, querier, externalLabels)

		_, err := server.LabelValues(context.Background(), &storepb.LabelValuesRequest{
			Label: "instance",
			Matchers: []storepb.LabelMatcher{
				{Type: storepb.LabelMatcher_RE, Name: "job", Value: "node.*"},
			},
		})
		require.NoError(t, err)

		require.Len(t, builder.matchers, 1)
		assert.Equal(t, "job", builder.matchers[0].Name)
		assert.Equal(t, prompb.LabelMatcher_RE, builder.matchers[0].Type)
		assert.Equal(t, "PREDICATE", querier.labelValuesPredicate)
	})

	t.Run("a non-matching external-label matcher returns nothing", func(t *testing.T) {
		querier := &mockQuerier{labelValues: map[string][]string{"instance": {"a", "b"}}}
		server := newTestServer(querier, externalLabels)

		resp, err := server.LabelValues(context.Background(), &storepb.LabelValuesRequest{
			Label: "instance",
			Matchers: []storepb.LabelMatcher{
				{Type: storepb.LabelMatcher_EQ, Name: "source", Value: "other"},
			},
		})
		require.NoError(t, err)
		assert.Empty(t, resp.Values)
		assert.Equal(t, 0, querier.labelValuesCalls)
	})
}

func TestServer_LabelMetadataFailures(t *testing.T) {
	externalLabels := map[string]string{"source": "postgres-adapter"}
	t.Run("a database failure is reported as internal", func(t *testing.T) {
		failure := errors.New("database is unreachable")

		t.Run("looking for a matching series", func(t *testing.T) {
			server := newTestServer(&mockQuerier{seriesExistErr: failure}, externalLabels)

			_, err := server.LabelValues(context.Background(), &storepb.LabelValuesRequest{Label: "source"})
			require.Error(t, err)
			assert.Equal(t, codes.Internal, status.Code(err))
			assert.Contains(t, status.Convert(err).Message(), "matching series")
		})

		t.Run("reading the label names", func(t *testing.T) {
			server := newTestServer(&mockQuerier{seriesExist: true, labelNamesErr: failure}, externalLabels)

			_, err := server.LabelNames(context.Background(), &storepb.LabelNamesRequest{})
			require.Error(t, err)
			assert.Equal(t, codes.Internal, status.Code(err))
			assert.Contains(t, status.Convert(err).Message(), "label names")
		})

		t.Run("reading the label values", func(t *testing.T) {
			server := newTestServer(&mockQuerier{labelValuesErr: failure}, externalLabels)

			_, err := server.LabelValues(context.Background(), &storepb.LabelValuesRequest{Label: "instance"})
			require.Error(t, err)
			assert.Equal(t, codes.Internal, status.Code(err))
			assert.Contains(t, status.Convert(err).Message(), "label values")
		})
	})

	t.Run("a matcher the builder cannot translate is rejected as invalid", func(t *testing.T) {
		// The builder only ever fails on what the request carries, so it is the
		// request that is wrong, not the store.
		builder := &mockBuilder{err: errors.New("unsupported inline regex option")}
		server := newTestServerWithBuilder(builder, &mockQuerier{seriesExist: true}, externalLabels)

		_, err := server.LabelNames(context.Background(), &storepb.LabelNamesRequest{})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))

		_, err = server.LabelValues(context.Background(), &storepb.LabelValuesRequest{Label: "instance"})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))

		stream := &fakeSeriesServer{ctx: context.Background()}
		err = server.Series(&storepb.SeriesRequest{
			Matchers: []storepb.LabelMatcher{{Type: storepb.LabelMatcher_EQ, Name: "job", Value: "node"}},
		}, stream)
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
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
