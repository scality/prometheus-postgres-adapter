package handler

import (
	"io"
	"net/http"

	"prom-adapter/pkg/domain"
	"prom-adapter/pkg/usecase"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"

	"github.com/rs/zerolog"
)

type (
	PrometheusToPostgreSQLMetricsPusher struct {
		uc     *usecase.PushSamples
		logger *zerolog.Logger
	}
)

func NewPrometheusToPostgreSQLMetricsPusher(
	uc *usecase.PushSamples,
	logger *zerolog.Logger,
) *PrometheusToPostgreSQLMetricsPusher {
	l := logger.With().Str("handler", "prometheus_writer").Logger()

	return &PrometheusToPostgreSQLMetricsPusher{
		uc:     uc,
		logger: &l,
	}
}

func (h *PrometheusToPostgreSQLMetricsPusher) Handle() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

//
// func NewPrometheusReader(uc *usecase.ReadPrometheusMetrics, logger *zerolog.Logger) *PrometheusReader {
// 	l := logger.With().Str("handler", "prometheus_reader").Logger()
//
// 	return &PrometheusReader{
// 		uc:     uc,
// 		logger: &l,
// 	}
// }
//
// func (h *PrometheusReader) Handle() http.Handler {
// 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		body, err := io.ReadAll(r.Body)
// 		if err != nil {
// 			http.Error(w, "failed to read request body", http.StatusInternalServerError)
// 			h.logger.Error().Err(err).Msg("failed to read request body")
//
// 			return
// 		}
//
// 		reqBuf, err := snappy.Decode(nil, body)
// 		if err != nil {
// 			h.logger.Error().Err(err).Msg("failed to decode request body")
// 			http.Error(w, err.Error(), http.StatusBadRequest)
//
// 			return
// 		}
//
// 		var req prompb.ReadRequest
//
// 		if err := proto.Unmarshal(reqBuf, &req); err != nil {
// 			h.logger.Error().Err(err).Msg("failed to unmarshal request body")
// 			http.Error(w, err.Error(), http.StatusBadRequest)
// 			return
// 		}
//
// 		resp, err := h.uc.Execute(r.Context(), &req)
// 		if err != nil {
// 			h.logger.Error().Err(err).Msg("failed to execute use case")
// 			http.Error(w, err.Error(), http.StatusInternalServerError)
//
// 			return
// 		}
//
// 		data, err := proto.Marshal(resp)
// 		if err != nil {
// 			h.logger.Error().Err(err).Msg("failed to marshal response")
// 			http.Error(w, err.Error(), http.StatusInternalServerError)
//
// 			return
// 		}
//
// 		w.Header().Set("Content-Encoding", "snappy")
// 		w.Header().Set("Content-Type", "application/x-protobuf")
//
// 		encodedBody := snappy.Encode(nil, data)
//
// 		_, err = w.Write(encodedBody)
// 		if err != nil {
// 			h.logger.Error().Err(err).Msg("failed to write response")
// 			http.Error(w, err.Error(), http.StatusInternalServerError)
//
// 			return
// 		}
// 	})
// }
