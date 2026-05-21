package handler

import (
	"io"
	"log/slog"
	"net/http"
	"prometheus-postgres-adapter/pkg/domain"
	"prometheus-postgres-adapter/pkg/usecase"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

type ReadPrometheusMetrics struct {
	uc     *usecase.ReadPrometheusSamples
	logger *slog.Logger
}

func NewReadPrometheusMetrics(
	uc *usecase.ReadPrometheusSamples,
	logger *slog.Logger,
) *ReadPrometheusMetrics {
	return &ReadPrometheusMetrics{
		uc:     uc,
		logger: logger.With(slog.String("handler", "prometheus_reader")),
	}
}

//nolint:gocognit,funlen // Handling requests requires a lot of steps, but is not complex
func (h *ReadPrometheusMetrics) Handle() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		h.logger.InfoContext(ctx, "handling request")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusInternalServerError)
			h.logger.ErrorContext(ctx, "failed to read request body", slog.Any("error_message", err))

			return
		}

		reqBuf, err := snappy.Decode(nil, body)
		if err != nil {
			h.logger.ErrorContext(ctx, "failed to decode request body", slog.Any("error_message", err))
			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}

		var req prompb.ReadRequest

		if err := proto.Unmarshal(reqBuf, &req); err != nil {
			h.logger.ErrorContext(ctx, "failed to unmarshal request body", slog.Any("error_message", err))
			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}

		readRequest := domain.ReadRequest{
			R: &req,
		}

		resp, err := h.uc.Execute(ctx, &readRequest)
		if err != nil {
			h.logger.ErrorContext(ctx, "failed to execute use case", slog.Any("error_message", err))
			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}

		data, err := proto.Marshal(resp.R)
		if err != nil {
			h.logger.ErrorContext(ctx, "failed to marshal response", slog.Any("error_message", err))
			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}

		w.Header().Set("Content-Encoding", "snappy")
		w.Header().Set("Content-Type", "application/x-protobuf")

		encodedBody := snappy.Encode(nil, data)

		_, err = w.Write(encodedBody)
		if err != nil {
			h.logger.ErrorContext(ctx, "failed to write response", slog.Any("error_message", err))
			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}
	})
}
