package usecase

import (
	"context"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"

	"prometheus-postgres-adapter/pkg/domain"
)

type (
	PushPrometheusSamples struct {
		logger *zerolog.Logger

		messageQueuePusher messageQueuePusher
	}

	messageQueuePusher interface {
		Push(samples *domain.Samples) error
	}
)

func NewPushPrometheusSamples(
	logger *zerolog.Logger,
	messageQueuePusher messageQueuePusher,
) *PushPrometheusSamples {
	l := logger.With().Str("usecase", "push_prometheus_samples").Logger()

	logger.Debug().Msg("PushPrometheusSamples initialized")
	return &PushPrometheusSamples{
		logger:             &l,
		messageQueuePusher: messageQueuePusher,
	}
}

func (uc *PushPrometheusSamples) Execute(_ context.Context, samples *domain.Samples) error {
	uc.logger.Debug().Msg("Executing use case")

	err := uc.messageQueuePusher.Push(samples)
	if err != nil {
		return errors.Wrap(err, "failed to push samples")
	}

	return nil
}
