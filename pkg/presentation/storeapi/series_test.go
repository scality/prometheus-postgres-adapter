package storeapi_test

import (
	"prometheus-postgres-adapter/pkg/domain"
	"prometheus-postgres-adapter/pkg/presentation/storeapi"
	"sort"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/thanos/pkg/store/labelpb"
	"github.com/thanos-io/thanos/pkg/store/storepb"
)

func dbRow(name string, lbls map[string]string, ts time.Time, value float64) *domain.SamplesReadFromDatabase {
	keys := make([]string, 0, len(lbls))
	for k := range lbls {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return &domain.SamplesReadFromDatabase{
		Name:      name,
		Value:     value,
		Timestamp: ts,
		Labels:    domain.SampleLabels{Map: lbls, OrderedKeys: keys},
	}
}

// labelMap reads ZLabels by value, avoiding labelpb's unsafe PromLabels cast,
// which is incompatible with the default stringlabels build.
func labelMap(zls []labelpb.ZLabel) map[string]string {
	out := make(map[string]string, len(zls))
	for _, zl := range zls {
		out[zl.Name] = zl.Value
	}

	return out
}

type timestampValue struct {
	timestamp int64
	value     float64
}

func decodeChunks(t *testing.T, chunks []storepb.AggrChunk) []timestampValue {
	t.Helper()

	var out []timestampValue

	for _, chunk := range chunks {
		require.NotNil(t, chunk.Raw)
		require.Equal(t, storepb.Chunk_XOR, chunk.Raw.Type)

		decoded, err := chunkenc.FromData(chunkenc.EncXOR, chunk.Raw.Data)
		require.NoError(t, err)

		iter := decoded.Iterator(nil)
		for iter.Next() != chunkenc.ValNone {
			ts, value := iter.At()
			out = append(out, timestampValue{timestamp: ts, value: value})
		}

		require.NoError(t, iter.Err())
	}

	return out
}

func TestBuildSeries(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	external := map[string]string{"source": "postgres-adapter"}

	rows := []*domain.SamplesReadFromDatabase{
		dbRow("cluster_storage_free", map[string]string{"instance": "b"}, base, 10),
		dbRow("cluster_storage_free", map[string]string{"instance": "a"}, base, 1),
		dbRow("cluster_storage_free", map[string]string{"instance": "a"}, base.Add(time.Hour), 2),
	}

	series, err := storeapi.BuildSeries(rows, external)
	require.NoError(t, err)
	require.Len(t, series, 2)

	// Series are sorted by label set: instance="a" comes before instance="b".
	first := labelMap(series[0].Labels)
	assert.Equal(t, "cluster_storage_free", first[model.MetricNameLabel])
	assert.Equal(t, "a", first["instance"])
	assert.Equal(t, "postgres-adapter", first["source"])

	second := labelMap(series[1].Labels)
	assert.Equal(t, "b", second["instance"])

	// Samples round-trip through the XOR chunks, in timestamp order.
	samples := decodeChunks(t, series[0].Chunks)
	require.Len(t, samples, 2)
	assert.Equal(t, base.UnixMilli(), samples[0].timestamp)
	assert.InDelta(t, 1.0, samples[0].value, 0)
	assert.Equal(t, base.Add(time.Hour).UnixMilli(), samples[1].timestamp)
	assert.InDelta(t, 2.0, samples[1].value, 0)
}

func TestBuildSeries_ExternalLabelOverridesStored(t *testing.T) {
	rows := []*domain.SamplesReadFromDatabase{
		dbRow(
			"m",
			map[string]string{"prometheus": "metalk8s-monitoring/prometheus-operator-prometheus"},
			time.Unix(0, 0),
			1,
		),
	}

	series, err := storeapi.BuildSeries(rows, map[string]string{"prometheus": "overridden"})
	require.NoError(t, err)
	require.Len(t, series, 1)

	lset := labelMap(series[0].Labels)
	assert.Equal(t, "overridden", lset["prometheus"])
}

func TestBuildSeries_Empty(t *testing.T) {
	series, err := storeapi.BuildSeries(nil, nil)
	require.NoError(t, err)
	assert.Empty(t, series)
}

func TestBuildSeries_MultipleChunks(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	const sampleCount = 250 // > 2 * maxSamplesPerChunk

	rows := make([]*domain.SamplesReadFromDatabase, 0, sampleCount)
	for i := range sampleCount {
		rows = append(rows, dbRow("m", map[string]string{"instance": "a"}, base.Add(time.Duration(i)*time.Minute), float64(i)))
	}

	series, err := storeapi.BuildSeries(rows, nil)
	require.NoError(t, err)
	require.Len(t, series, 1)
	assert.Len(t, series[0].Chunks, 3) // 120 + 120 + 10

	samples := decodeChunks(t, series[0].Chunks)
	require.Len(t, samples, sampleCount)
	assert.Equal(t, base.UnixMilli(), samples[0].timestamp)
	assert.InDelta(t, float64(sampleCount-1), samples[sampleCount-1].value, 0)
}
