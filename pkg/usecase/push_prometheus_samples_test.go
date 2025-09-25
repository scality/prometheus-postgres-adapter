package usecase_test

import (
	"context"
	"prometheus-postgres-adapter/pkg/domain"
	"prometheus-postgres-adapter/pkg/usecase"
	"testing"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

// MockMessageQueuePusher is a mock implementation of the messageQueuePusher interface.
type MockMessageQueuePusher struct {
	PushFunc func(samples *domain.Samples) error
}

func (m *MockMessageQueuePusher) Push(samples *domain.Samples) error {
	return m.PushFunc(samples)
}

func TestPushPrometheusSamples_Execute(t *testing.T) {
	// Create a test logger
	logger := zerolog.New(zerolog.Nop())

	t.Run("Successful push", func(t *testing.T) {
		// Setup mock
		mockPusher := &MockMessageQueuePusher{
			PushFunc: func(samples *domain.Samples) error {
				return nil
			},
		}

		// Create usecase
		uc := usecase.NewPushPrometheusSamples(&logger, mockPusher)

		// Execute usecase
		samples := &domain.Samples{}
		err := uc.Execute(context.Background(), samples)

		// Check results
		assert.NoError(t, err)
	})

	t.Run("Push failure", func(t *testing.T) {
		// Setup mock
		mockPusher := &MockMessageQueuePusher{
			PushFunc: func(samples *domain.Samples) error {
				return errors.New("push error")
			},
		}

		// Create usecase
		uc := usecase.NewPushPrometheusSamples(&logger, mockPusher)

		// Execute usecase
		samples := &domain.Samples{}
		err := uc.Execute(context.Background(), samples)

		// Check results
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to push samples")
	})
}
