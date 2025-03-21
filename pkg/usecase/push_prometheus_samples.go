package usecase

import (
	"context"
	"prometheus-postgres-adapter/pkg/domain"
	"prometheus-postgres-adapter/pkg/service"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type PushPrometheusSamples struct {
	logger *zerolog.Logger

	messageQueuePusher service.MessageQueuePusher
}

func NewPushPrometheusSamples(
	logger *zerolog.Logger,
	messageQueuePusher service.MessageQueuePusher,
) *PushPrometheusSamples {
	l := logger.With().Str("usecase", "push_prometheus_samples").Logger()

	return &PushPrometheusSamples{
		logger:             &l,
		messageQueuePusher: messageQueuePusher,
	}
}

func (uc *PushPrometheusSamples) Execute(_ context.Context, samples *domain.Samples) error {
	uc.logger.Debug().Msg("Executing push samples use case")

	err := uc.messageQueuePusher.Push(samples)
	if err != nil {
		uc.logger.Debug().Err(err).Msg("Failed to push samples")

		return errors.Wrap(err, "failed to push samples")
	}

	return nil
}
