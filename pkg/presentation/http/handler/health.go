package handler

import (
	"log/slog"
	"net/http"
	"prometheus-postgres-adapter/pkg/usecase"
)

type Health struct {
	uc     *usecase.CheckDatabaseHealth
	logger *slog.Logger
}

func NewHealth(
	uc *usecase.CheckDatabaseHealth,
	logger *slog.Logger,
) *Health {
	return &Health{
		uc:     uc,
		logger: logger.With(slog.String("handler", "health")),
	}
}

func (h *Health) Handle() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		h.logger.InfoContext(ctx, "handling request")

		err := h.uc.Execute(ctx)
		if err != nil {
			h.logger.ErrorContext(ctx, "failed to check database health", slog.Any("error_message", err))
			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}

		w.WriteHeader(http.StatusOK)

		h.logger.InfoContext(ctx, "database health check passed")
	})
}
