package usecase

import (
	"context"

	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"

	"prometheus-postgres-adapter/pkg/domain"
)

type (
	ReadPrometheusSamples struct {
		logger *zerolog.Logger

		builder SQLQueryBuilder
		querier SQLQuerier
	}

	SQLQueryBuilder interface {
		BuildSQLQuery(*prompb.Query) (string, error)
	}

	SQLQuerier interface {
		QueryDatabaseSamples(
			ctx context.Context,
			query string,
			args ...any,
		) ([]*domain.SamplesReadFromDatabase, error)
	}
)

func NewReadPrometheusSamples(
	logger *zerolog.Logger,
	builder SQLQueryBuilder,
	querier SQLQuerier,
) *ReadPrometheusSamples {
	l := logger.With().Str("usecase", "read_samples").Logger()

	logger.Debug().Msg("ReadPrometheusSamples initialized")

	return &ReadPrometheusSamples{
		logger:  &l,
		builder: builder,
		querier: querier,
	}
}

//nolint:gocognit,funlen // The sample transformation part takes a lot of lines but is quite simple
func (uc *ReadPrometheusSamples) Execute(
	ctx context.Context,
	req *domain.ReadRequest,
) (*domain.ReadResponse, error) {
	uc.logger.Debug().Msg("Executing use case")

	labelsToSeries := map[string]*prompb.TimeSeries{}

	for _, query := range req.R.Queries {
		// Build the SQL query according to the Prometheus query
		sqlQuery, err := uc.builder.BuildSQLQuery(query)
		if err != nil {
			return nil, errors.Wrap(err, "failed to build SQL query")
		}

		// Query the database
		samples, err := uc.querier.QueryDatabaseSamples(ctx, sqlQuery)
		if err != nil {
			return nil, errors.Wrap(err, "failed to query rows")
		}

		for _, sample := range samples {
			// Convert the labels to a string key
			key := sample.Labels.Key(sample.Name)

			// Check if the key already exists
			timeserie, ok := labelsToSeries[key]
			if !ok {
				labelPairs := make([]prompb.Label, 0, len(sample.Labels.OrderedKeys)+1)

				// Add the metric name label
				labelPairs = append(labelPairs, prompb.Label{
					Name:  model.MetricNameLabel,
					Value: sample.Name,
				})

				// Add the other labels
				for _, k := range sample.Labels.OrderedKeys {
					labelPairs = append(labelPairs, prompb.Label{
						Name:  k,
						Value: sample.Labels.Map[k],
					})
				}

				// Create a new timeserie for the key
				labelsToSeries[key] = &prompb.TimeSeries{
					Labels:  labelPairs,
					Samples: make([]prompb.Sample, 0),
				}
			}

			if timeserie == nil {
				timeserie = &prompb.TimeSeries{}
			}

			// Append the sample to the timeserie
			timeserie.Samples = append(timeserie.Samples, prompb.Sample{
				Timestamp: sample.Timestamp.UnixNano() / 1000000, //nolint:mnd // Convert to milliseconds
				Value:     sample.Value,
			})
		}
	}

	// Create the response
	resp := prompb.ReadResponse{
		Results: []*prompb.QueryResult{
			{
				Timeseries: make([]*prompb.TimeSeries, 0, len(labelsToSeries)),
			},
		},
	}

	// Fill it with the timeseries
	for _, ts := range labelsToSeries {
		resp.Results[0].Timeseries = append(resp.Results[0].Timeseries, ts)
	}

	return &domain.ReadResponse{
		R: &resp,
	}, nil
}
