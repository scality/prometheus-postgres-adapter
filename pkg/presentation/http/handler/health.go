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

func (health *Health) Handle() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		err := health.uc.Execute(ctx)
		if err != nil {
			health.logger.Error().Err(err).Msg("failed to check database health")
			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}

		w.WriteHeader(http.StatusOK)

		health.logger.Info().Msg("database health check passed")
	})
}
