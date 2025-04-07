package messagequeue_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"prometheus-postgres-adapter/pkg/domain"
	"prometheus-postgres-adapter/pkg/presentation/messagequeue"
)

func TestNewChan(t *testing.T) {
	// Test that NewChan creates a non-nil Chan with initialized queue
	c := messagequeue.NewChan()
	assert.NotNil(t, c)
}

func TestChan_PushAndPop(t *testing.T) {
	// Test Push and Pop work together
	c := messagequeue.NewChan()
	samples := &domain.Samples{} // Empty samples for testing

	// Use goroutine for Push to avoid blocking
	go func() {
		err := c.Push(samples)
		assert.NoError(t, err)
	}()

	// Pop should receive the same samples
	poppedSamples := c.Pop()
	assert.Equal(t, samples, poppedSamples)
}

func TestChan_MultiplePushAndPop(t *testing.T) {
	// Test multiple Push/Pop operations
	c := messagequeue.NewChan()
	samples1 := &domain.Samples{}
	samples2 := &domain.Samples{}

	// Push first sample
	go func() {
		err := c.Push(samples1)
		assert.NoError(t, err)
	}()

	// Pop first sample
	poppedSamples1 := c.Pop()
	assert.Equal(t, samples1, poppedSamples1)

	// Push second sample
	go func() {
		err := c.Push(samples2)
		assert.NoError(t, err)
	}()

	// Pop second sample
	poppedSamples2 := c.Pop()
	assert.Equal(t, samples2, poppedSamples2)
}

func TestChan_PushError(t *testing.T) {
	// Test that Push doesn't return an error
	c := messagequeue.NewChan()
	samples := &domain.Samples{}

	// Use channel to get result from goroutine
	resultCh := make(chan error)
	go func() {
		resultCh <- c.Push(samples)
	}()

	// Pop to unblock the Push
	poppedSamples := c.Pop()
	assert.Equal(t, samples, poppedSamples)

	// Check Push error
	err := <-resultCh
	assert.NoError(t, err)
}
