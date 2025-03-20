package usecase

import (
	"context"

	"prometheus-postgres-adapter/pkg/domain"
	"prometheus-postgres-adapter/pkg/service"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type PushSamples struct {
	logger *zerolog.Logger

	messageQueuePusher service.MessageQueuePusher
}

func NewPushSamples(
	logger *zerolog.Logger,
	messageQueuePusher service.MessageQueuePusher,
) *PushSamples {
	l := logger.With().Str("usecase", "push_samples").Logger()

	return &PushSamples{
		logger:             &l,
		messageQueuePusher: messageQueuePusher,
	}
}

func (uc *PushSamples) Execute(_ context.Context, samples *domain.Samples) error {
	uc.logger.Debug().Msg("Executing push samples use case ")

	err := uc.messageQueuePusher.Push(samples)
	if err != nil {
		uc.logger.Debug().Err(err).Msg("Failed to push samples")

		return errors.Wrap(err, "failed to push samples")
	}

	return nil
}
