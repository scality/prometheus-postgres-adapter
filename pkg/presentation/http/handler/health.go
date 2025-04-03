package handler

import (
	"net/http"
	"prometheus-postgres-adapter/pkg/usecase"

	"github.com/rs/zerolog"
)

type Health struct {
	uc     *usecase.CheckDatabaseHealth
	logger *zerolog.Logger
}

func NewHealth(
	uc *usecase.CheckDatabaseHealth,
	logger *zerolog.Logger,
) *Health {
	l := logger.With().Str("handler", "health").Logger()

	return &Health{
		uc:     uc,
		logger: &l,
	}
}

func (h *Health) Handle() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.logger.Info().Msg("handling request")

		ctx := r.Context()

		err := h.uc.Execute(ctx)
		if err != nil {
			h.logger.Error().Err(err).Msg("failed to check database health")
			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}

		w.WriteHeader(http.StatusOK)

		h.logger.Info().Msg("database health check passed")
	})
}
