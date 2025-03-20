package usecase

import (
	"context"
	"database/sql"
	"time"

	"prometheus-postgres-adapter/pkg/domain"

	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"
)

type (
	ReadSamples struct {
		logger *zerolog.Logger

		builder SQLQueryBuilder
		querier SQLQuerier
	}

	SQLQueryBuilder interface {
		BuildSQLQuery(*prompb.Query) (string, error)
	}

	SQLQuerier interface {
		Query(context.Context, string, ...any) (*sql.Rows, error)
	}
)

func NewReadSamples(
	logger *zerolog.Logger,
	builder SQLQueryBuilder,
	querier SQLQuerier,
) *ReadSamples {
	l := logger.With().Str("usecase", "read_samples").Logger()

	return &ReadSamples{
		logger:  &l,
		builder: builder,
		querier: querier,
	}
}

func (uc *ReadSamples) Execute(
	ctx context.Context,
	req *domain.ReadRequest,
) (*domain.ReadResponse, error) {
	labelsToSeries := map[string]*prompb.TimeSeries{}

	for _, query := range req.R.Queries {
		sqlQuery, err := uc.builder.BuildSQLQuery(query)
		if err != nil {
			return nil, errors.Wrap(err, "failed to build SQL query")
		}

		rows, err := uc.querier.Query(ctx, sqlQuery)
		if err != nil {
			return nil, errors.Wrap(err, "failed to query rows")
		}

		for rows.Next() {
			var (
				value  float64
				name   string
				labels domain.SampleLabels
				t      time.Time
			)

			err := rows.Scan(&t, &name, &value, &labels)
			if err != nil {
				rows.Close()

				return nil, errors.Wrap(err, "failed to scan rows")
			}

			key := labels.Key(name)

			timeserie, ok := labelsToSeries[key]
			if !ok {
				labelPairs := make([]prompb.Label, 0, len(labels.OrderedKeys)+1)
				labelPairs = append(labelPairs, prompb.Label{
					Name:  model.MetricNameLabel,
					Value: name,
				})

				for _, k := range labels.OrderedKeys {
					labelPairs = append(labelPairs, prompb.Label{
						Name:  k,
						Value: labels.Map[k],
					})
				}

				labelsToSeries[key] = &prompb.TimeSeries{
					Labels:  labelPairs,
					Samples: make([]prompb.Sample, 0, 100),
				}
			}

			timeserie.Samples = append(timeserie.Samples, prompb.Sample{
				Timestamp: t.UnixNano() / 1000000,
				Value:     value,
			})
		}
	}

	resp := prompb.ReadResponse{
		Results: []*prompb.QueryResult{
			{
				Timeseries: make([]*prompb.TimeSeries, 0, len(labelsToSeries)),
			},
		},
	}
	for _, ts := range labelsToSeries {
		resp.Results[0].Timeseries = append(resp.Results[0].Timeseries, ts)
	}

	return &domain.ReadResponse{
		R: &resp,
	}, nil
}
