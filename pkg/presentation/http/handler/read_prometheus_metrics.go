package handler

import (
	"io"
	"net/http"
	"prometheus-postgres-adapter/pkg/domain"
	"prometheus-postgres-adapter/pkg/usecase"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"
)

type ReadPrometheusMetrics struct {
	uc     *usecase.ReadPrometheusSamples
	logger *zerolog.Logger
}

func NewReadPrometheusMetrics(
	uc *usecase.ReadPrometheusSamples,
	logger *zerolog.Logger,
) *ReadPrometheusMetrics {
	l := logger.With().Str("handler", "prometheus_reader").Logger()

	return &ReadPrometheusMetrics{
		uc:     uc,
		logger: &l,
	}
}

//nolint:gocognit,funlen // Handling requests requires a lot of steps, but is not complex
func (h *ReadPrometheusMetrics) Handle() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.logger.Info().Msg("handling request")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusInternalServerError)
			h.logger.Error().Err(err).Msg("failed to read request body")

			return
		}

		reqBuf, err := snappy.Decode(nil, body)
		if err != nil {
			h.logger.Error().Err(err).Msg("failed to decode request body")
			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}

		var req prompb.ReadRequest

		if err := proto.Unmarshal(reqBuf, &req); err != nil {
			h.logger.Error().Err(err).Msg("failed to unmarshal request body")
			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}

		readRequest := domain.ReadRequest{
			R: &req,
		}

		resp, err := h.uc.Execute(r.Context(), &readRequest)
		if err != nil {
			h.logger.Error().Err(err).Msg("failed to execute use case")
			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}

		data, err := proto.Marshal(resp.R)
		if err != nil {
			h.logger.Error().Err(err).Msg("failed to marshal response")
			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}

		w.Header().Set("Content-Encoding", "snappy")
		w.Header().Set("Content-Type", "application/x-protobuf")

		encodedBody := snappy.Encode(nil, data)

		_, err = w.Write(encodedBody)
		if err != nil {
			h.logger.Error().Err(err).Msg("failed to write response")
			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}
	})
}
