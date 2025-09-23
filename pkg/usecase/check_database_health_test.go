package usecase_test

import (
	"context"
	"errors"
	"prometheus-postgres-adapter/pkg/usecase"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockHealthChecker struct {
	mock.Mock
}

func (m *MockHealthChecker) CheckHealth(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func TestCheckDatabaseHealth_Execute(t *testing.T) {
	// Create a test logger that doesn't output
	logger := zerolog.New(zerolog.Nop())

	t.Run("success case", func(t *testing.T) {
		// Setup mock
		healthChecker := new(MockHealthChecker)
		healthChecker.On("CheckHealth", mock.Anything).Return(nil)

		// Create usecase
		uc := usecase.NewCheckDatabaseHealth(healthChecker, &logger)

		// Execute
		err := uc.Execute(context.Background())

		// Verify
		assert.NoError(t, err)
		healthChecker.AssertExpectations(t)
	})

	t.Run("failure case", func(t *testing.T) {
		// Setup mock
		healthChecker := new(MockHealthChecker)
		mockError := errors.New("database connection failed") //nolint:err113 // Come on
		healthChecker.On("CheckHealth", mock.Anything).Return(mockError)

		// Create usecase
		uc := usecase.NewCheckDatabaseHealth(healthChecker, &logger)

		// Execute - the usecase should now return the error
		err := uc.Execute(context.Background())

		// Verify
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database health check failed")
		assert.Contains(t, err.Error(), "database connection failed")
		healthChecker.AssertExpectations(t)
	})
}
