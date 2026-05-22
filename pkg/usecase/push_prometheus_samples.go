package usecase

import (
	"context"
	"log/slog"
	"prometheus-postgres-adapter/pkg/domain"

	"github.com/pkg/errors"
)

type (
	PushPrometheusSamples struct {
		logger *slog.Logger

		messageQueuePusher messageQueuePusher
	}

	messageQueuePusher interface {
		Push(samples *domain.Samples) error
	}
)

func NewPushPrometheusSamples(
	logger *slog.Logger,
	messageQueuePusher messageQueuePusher,
) *PushPrometheusSamples {
	return &PushPrometheusSamples{
		logger:             logger.With(slog.String("usecase", "push_prometheus_samples")),
		messageQueuePusher: messageQueuePusher,
	}
}

func (uc *PushPrometheusSamples) Execute(ctx context.Context, samples *domain.Samples) error {
	uc.logger.DebugContext(ctx, "Executing use case")

	err := uc.messageQueuePusher.Push(samples)
	if err != nil {
		return errors.Wrap(err, "failed to push samples")
	}

	return nil
}
