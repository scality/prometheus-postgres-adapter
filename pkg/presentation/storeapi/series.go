package storeapi

import (
	"maps"
	"prometheus-postgres-adapter/pkg/domain"
	"sort"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/scality/go-errors"
	"github.com/thanos-io/thanos/pkg/store/labelpb"
	"github.com/thanos-io/thanos/pkg/store/storepb"
)

// ErrCreateXORChunkAppender is returned when an XOR chunk appender cannot be created.
var ErrCreateXORChunkAppender = errors.New("failed to create XOR chunk appender")

// maxSamplesPerChunk matches Prometheus' convention for the maximum number of
// samples encoded in a single XOR chunk.
const maxSamplesPerChunk = 120

type sample struct {
	timestamp int64
	value     float64
}

type seriesAccumulator struct {
	lset    labels.Labels
	samples []sample
}

// BuildSeries groups database rows into sorted Thanos StoreAPI series. The
// configured external labels are applied to every series and take precedence on
// name collision (like the Thanos sidecar), while the metric name stays
// authoritative. Samples are encoded into XOR chunks.
func BuildSeries(
	rows []*domain.SamplesReadFromDatabase,
	externalLabels map[string]string,
) ([]storepb.Series, error) {
	accumulators := groupRowsIntoSeries(rows, externalLabels)

	series := make([]storepb.Series, 0, len(accumulators))

	for _, acc := range accumulators {
		chunks, err := encodeXORChunks(acc.samples)
		if err != nil {
			return nil, err
		}

		series = append(series, storepb.Series{
			Labels: zLabels(acc.lset),
			Chunks: chunks,
		})
	}

	return series, nil
}

// zLabels converts a Prometheus label set into Thanos ZLabels by copying each
// Name/Value.
//
// We deliberately avoid labelpb.ZLabelsFromPromLabels: it does an unsafe pointer
// cast (*(*[]ZLabel)(unsafe.Pointer(&lset))) that only holds when labels.Labels
// is a []Label slice. Prometheus v0.312.0 defaults to the "stringlabels" build,
// where labels.Labels is instead a single packed string with a different memory
// layout, so that cast reinterprets the bytes wrongly and yields corrupt labels
// on the wire. Ranging over the set is correct for any labels representation.
func zLabels(lset labels.Labels) []labelpb.ZLabel {
	zls := make([]labelpb.ZLabel, 0, lset.Len())

	lset.Range(func(label labels.Label) {
		zls = append(zls, labelpb.ZLabel{Name: label.Name, Value: label.Value})
	})

	return zls
}

// groupRowsIntoSeries groups the rows by their label set, sorts each series'
// samples by timestamp and returns the accumulators sorted by label set, as
// required by the Thanos StoreAPI.
func groupRowsIntoSeries(
	rows []*domain.SamplesReadFromDatabase,
	externalLabels map[string]string,
) []*seriesAccumulator {
	groups := make(map[string]*seriesAccumulator)

	for _, row := range rows {
		key := row.Labels.Key(row.Name)

		acc, ok := groups[key]
		if !ok {
			acc = &seriesAccumulator{lset: promLabels(row.Name, row.Labels.Map, externalLabels)}
			groups[key] = acc
		}

		acc.samples = append(acc.samples, sample{
			timestamp: row.Timestamp.UnixMilli(),
			value:     row.Value,
		})
	}

	accumulators := make([]*seriesAccumulator, 0, len(groups))

	for _, acc := range groups {
		sort.Slice(acc.samples, func(i, j int) bool {
			return acc.samples[i].timestamp < acc.samples[j].timestamp
		})

		accumulators = append(accumulators, acc)
	}

	sort.Slice(accumulators, func(i, j int) bool {
		return labels.Compare(accumulators[i].lset, accumulators[j].lset) < 0
	})

	return accumulators
}

// promLabels merges the metric name, the stored labels and the external labels
// into a sorted label set. External labels override stored labels on name
// collision; the metric name is always authoritative.
func promLabels(name string, stored, external map[string]string) labels.Labels {
	merged := make(map[string]string, len(stored)+len(external)+1)

	maps.Copy(merged, stored)
	maps.Copy(merged, external)

	merged[model.MetricNameLabel] = name

	return labels.FromMap(merged)
}

func encodeXORChunks(samples []sample) ([]storepb.AggrChunk, error) {
	// ceil(len(samples) / maxSamplesPerChunk): pre-size for one chunk per batch.
	chunks := make([]storepb.AggrChunk, 0, (len(samples)+maxSamplesPerChunk-1)/maxSamplesPerChunk)

	for start := 0; start < len(samples); start += maxSamplesPerChunk {
		end := min(start+maxSamplesPerChunk, len(samples))
		batch := samples[start:end]

		chunk := chunkenc.NewXORChunk()

		appender, err := chunk.Appender()
		if err != nil {
			return nil, errors.Wrap(ErrCreateXORChunkAppender, errors.CausedBy(err))
		}

		for _, s := range batch {
			// XOR chunks ignore the start-timestamp argument, hence 0.
			appender.Append(0, s.timestamp, s.value)
		}

		chunks = append(chunks, storepb.AggrChunk{
			MinTime: batch[0].timestamp,
			MaxTime: batch[len(batch)-1].timestamp,
			Raw:     &storepb.Chunk{Type: storepb.Chunk_XOR, Data: chunk.Bytes()},
		})
	}

	return chunks, nil
}
