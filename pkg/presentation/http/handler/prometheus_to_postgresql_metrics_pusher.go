package handler

import (
	"io"
	"net/http"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"

	"prometheus-postgres-adapter/pkg/domain"
	"prometheus-postgres-adapter/pkg/usecase"
)

type (
	PrometheusToPostgreSQLMetricsPusher struct {
		uc     *usecase.PushPrometheusSamples
		logger *zerolog.Logger
	}
)

func NewPrometheusToPostgreSQLMetricsPusher(
	uc *usecase.PushPrometheusSamples,
	logger *zerolog.Logger,
) *PrometheusToPostgreSQLMetricsPusher {
	l := logger.With().Str("handler", "prometheus_to_postgresql_metrics_pusher").Logger()

	return &PrometheusToPostgreSQLMetricsPusher{
		uc:     uc,
		logger: &l,
	}
}

func (h *PrometheusToPostgreSQLMetricsPusher) Handle() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.logger.Info().Msg("Handling request")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(
				w,
				"failed to read request body",
				http.StatusInternalServerError,
			)
			h.logger.Error().Err(err).Msg("failed to read request body")

			return
		}

		reqBuf, err := snappy.Decode(nil, body)
		if err != nil {
			h.logger.Error().Err(err).Msg("failed to decode request body")
			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}

		var req prompb.WriteRequest

		err = proto.Unmarshal(reqBuf, &req)
		if err != nil {
			h.logger.Error().Err(err).Msg("failed to unmarshal request body")
			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}

		samples := protoToSamples(&req)

		err = h.uc.Execute(r.Context(), samples)
		if err != nil {
			h.logger.Error().Err(err).Msg("failed to execute use case")
			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}
	})
}

func protoToSamples(req *prompb.WriteRequest) *domain.Samples {
	var samples model.Samples

	for _, ts := range req.Timeseries {
		metric := make(model.Metric, len(ts.Labels))
		for _, l := range ts.Labels {
			metric[model.LabelName(l.Name)] = model.LabelValue(l.Value)
		}

		for _, s := range ts.Samples {
			samples = append(samples, &model.Sample{
				Metric:    metric,
				Value:     model.SampleValue(s.Value),
				Timestamp: model.Time(s.Timestamp),
			})
		}
	}

	return &domain.Samples{S: samples}
}
