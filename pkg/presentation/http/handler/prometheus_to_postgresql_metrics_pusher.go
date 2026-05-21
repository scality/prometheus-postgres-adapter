package handler

import (
	"io"
	"log/slog"
	"net/http"
	"prometheus-postgres-adapter/pkg/domain"
	"prometheus-postgres-adapter/pkg/usecase"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
)

type (
	PrometheusToPostgreSQLMetricsPusher struct {
		uc     *usecase.PushPrometheusSamples
		logger *slog.Logger
	}
)

func NewPrometheusToPostgreSQLMetricsPusher(
	uc *usecase.PushPrometheusSamples,
	logger *slog.Logger,
) *PrometheusToPostgreSQLMetricsPusher {
	return &PrometheusToPostgreSQLMetricsPusher{
		uc:     uc,
		logger: logger.With(slog.String("handler", "prometheus_to_postgresql_metrics_pusher")),
	}
}

func (h *PrometheusToPostgreSQLMetricsPusher) Handle() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		h.logger.InfoContext(ctx, "Handling request")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(
				w,
				"failed to read request body",
				http.StatusInternalServerError,
			)
			h.logger.ErrorContext(ctx, "failed to read request body", slog.Any("error_message", err))

			return
		}

		reqBuf, err := snappy.Decode(nil, body)
		if err != nil {
			h.logger.ErrorContext(ctx, "failed to decode request body", slog.Any("error_message", err))
			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}

		var req prompb.WriteRequest

		err = proto.Unmarshal(reqBuf, &req)
		if err != nil {
			h.logger.ErrorContext(ctx, "failed to unmarshal request body", slog.Any("error_message", err))
			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}

		samples := protoToSamples(&req)

		err = h.uc.Execute(ctx, samples)
		if err != nil {
			h.logger.ErrorContext(ctx, "failed to execute use case", slog.Any("error_message", err))
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
