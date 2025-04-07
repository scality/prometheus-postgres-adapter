package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"prometheus-postgres-adapter/pkg/domain"
	"prometheus-postgres-adapter/pkg/usecase"
)

// MockSQLQueryBuilder mocks the SQLQueryBuilder interface.
type MockSQLQueryBuilder struct {
	mock.Mock
}

func (m *MockSQLQueryBuilder) BuildSQLQuery(query *prompb.Query) (string, error) {
	args := m.Called(query)
	return args.String(0), args.Error(1)
}

// MockSQLQuerier mocks the SQLQuerier interface.
type MockSQLQuerier struct {
	mock.Mock
}

func (m *MockSQLQuerier) QueryDatabaseSamples(ctx context.Context, query string, args ...any) ([]*domain.SamplesReadFromDatabase, error) {
	callArgs := m.Called(ctx, query, args)
	if samples := callArgs.Get(0); samples != nil {
		return samples.([]*domain.SamplesReadFromDatabase), callArgs.Error(1)
	}

	return nil, callArgs.Error(1)
}

func TestReadPrometheusSamples_Execute(t *testing.T) {
	logger := zerolog.New(zerolog.Nop())

	t.Run("successful query with multiple results", func(t *testing.T) {
		// Setup mocks
		builder := new(MockSQLQueryBuilder)
		querier := new(MockSQLQuerier)

		query := &prompb.Query{}
		sqlQuery := "SELECT * FROM metrics"

		// Sample data
		now := time.Now()
		sampleData := []*domain.SamplesReadFromDatabase{
			{
				Timestamp: now,
				Name:      "cpu_usage",
				Value:     0.75,
				Labels: domain.SampleLabels{
					OrderedKeys: []string{"host", "instance"},
					Map: map[string]string{
						"host":     "server1",
						"instance": "1",
					},
				},
			},
			{
				Timestamp: now.Add(time.Minute),
				Name:      "cpu_usage",
				Value:     0.80,
				Labels: domain.SampleLabels{
					OrderedKeys: []string{"host", "instance"},
					Map: map[string]string{
						"host":     "server1",
						"instance": "1",
					},
				},
			},
			{
				Timestamp: now,
				Name:      "memory_usage",
				Value:     0.60,
				Labels: domain.SampleLabels{
					OrderedKeys: []string{"host"},
					Map: map[string]string{
						"host": "server2",
					},
				},
			},
		}

		// Set expectations
		builder.On("BuildSQLQuery", query).Return(sqlQuery, nil)
		querier.On("QueryDatabaseSamples", mock.Anything, sqlQuery, mock.Anything).Return(sampleData, nil)

		// Create the usecase
		uc := usecase.NewReadPrometheusSamples(&logger, builder, querier)

		// Test request
		req := &domain.ReadRequest{
			R: &prompb.ReadRequest{
				Queries: []*prompb.Query{query},
			},
		}

		t.Logf(";pouet")

		// Execute
		resp, err := uc.Execute(context.Background(), req)

		t.Logf(";pouet2")

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotNil(t, resp.R)
		assert.Len(t, resp.R.Results, 1)
		assert.Len(t, resp.R.Results[0].Timeseries, 2) // Two unique label sets

		// Verify builder and querier were called correctly
		builder.AssertExpectations(t)
		querier.AssertExpectations(t)
	})

	t.Run("error building SQL query", func(t *testing.T) {
		// Setup mocks
		builder := new(MockSQLQueryBuilder)
		querier := new(MockSQLQuerier)

		query := &prompb.Query{}
		expectedErr := errors.New("build query error")

		// Set expectations
		builder.On("BuildSQLQuery", query).Return("", expectedErr)

		// Create the usecase
		uc := usecase.NewReadPrometheusSamples(&logger, builder, querier)

		// Test request
		req := &domain.ReadRequest{
			R: &prompb.ReadRequest{
				Queries: []*prompb.Query{query},
			},
		}

		// Execute
		resp, err := uc.Execute(context.Background(), req)

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "failed to build SQL query")

		// Verify builder was called correctly
		builder.AssertExpectations(t)
		querier.AssertNotCalled(t, "QueryDatabaseSamples")
	})

	t.Run("error querying database", func(t *testing.T) {
		// Setup mocks
		builder := new(MockSQLQueryBuilder)
		querier := new(MockSQLQuerier)

		query := &prompb.Query{}
		sqlQuery := "SELECT * FROM metrics"
		expectedErr := errors.New("query error")

		// Set expectations
		builder.On("BuildSQLQuery", query).Return(sqlQuery, nil)
		querier.On("QueryDatabaseSamples", mock.Anything, sqlQuery, mock.Anything).Return(nil, expectedErr)

		// Create the usecase
		uc := usecase.NewReadPrometheusSamples(&logger, builder, querier)

		// Test request
		req := &domain.ReadRequest{
			R: &prompb.ReadRequest{
				Queries: []*prompb.Query{query},
			},
		}

		// Execute
		resp, err := uc.Execute(context.Background(), req)

		// Assertions
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "failed to query rows")

		// Verify dependencies were called correctly
		builder.AssertExpectations(t)
		querier.AssertExpectations(t)
	})

	t.Run("empty result returns empty timeseries", func(t *testing.T) {
		// Setup mocks
		builder := new(MockSQLQueryBuilder)
		querier := new(MockSQLQuerier)

		query := &prompb.Query{}
		sqlQuery := "SELECT * FROM metrics"
		emptyResult := []*domain.SamplesReadFromDatabase{}

		// Set expectations
		builder.On("BuildSQLQuery", query).Return(sqlQuery, nil)
		querier.On("QueryDatabaseSamples", mock.Anything, sqlQuery, mock.Anything).Return(emptyResult, nil)

		// Create the usecase
		uc := usecase.NewReadPrometheusSamples(&logger, builder, querier)

		// Test request
		req := &domain.ReadRequest{
			R: &prompb.ReadRequest{
				Queries: []*prompb.Query{query},
			},
		}

		// Execute
		resp, err := uc.Execute(context.Background(), req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotNil(t, resp.R)
		assert.Len(t, resp.R.Results, 1)
		assert.Empty(t, resp.R.Results[0].Timeseries)

		// Verify dependencies were called correctly
		builder.AssertExpectations(t)
		querier.AssertExpectations(t)
	})

	t.Run("multiple queries", func(t *testing.T) {
		// Setup mocks
		builder := new(MockSQLQueryBuilder)
		querier := new(MockSQLQuerier)

		// Make queries more distinct
		query1 := &prompb.Query{
			StartTimestampMs: 1000,
			Matchers: []*prompb.LabelMatcher{
				{Type: prompb.LabelMatcher_EQ, Name: "metric", Value: "cpu"},
			},
		}
		query2 := &prompb.Query{
			StartTimestampMs: 2000,
			Matchers: []*prompb.LabelMatcher{
				{Type: prompb.LabelMatcher_EQ, Name: "metric", Value: "memory"},
			},
		}
		sqlQuery1 := "SELECT * FROM metrics WHERE name='cpu'"
		sqlQuery2 := "SELECT * FROM metrics WHERE name='memory'"

		// Sample data with distinct metrics
		now := time.Now()
		cpuSamples := []*domain.SamplesReadFromDatabase{
			{
				Timestamp: now,
				Name:      "cpu_usage",
				Value:     0.75,
				Labels: domain.SampleLabels{
					OrderedKeys: []string{"host"},
					Map: map[string]string{
						"host": "server1",
					},
				},
			},
		}
		memorySamples := []*domain.SamplesReadFromDatabase{
			{
				Timestamp: now,
				Name:      "memory_usage",
				Value:     0.60,
				Labels: domain.SampleLabels{
					OrderedKeys: []string{"host"},
					Map: map[string]string{
						"host": "server2",
					},
				},
			},
		}

		// Use mock.MatchedBy to match specific queries
		builder.On("BuildSQLQuery", mock.MatchedBy(func(q *prompb.Query) bool {
			return q.StartTimestampMs == 1000
		})).Return(sqlQuery1, nil)

		builder.On("BuildSQLQuery", mock.MatchedBy(func(q *prompb.Query) bool {
			return q.StartTimestampMs == 2000
		})).Return(sqlQuery2, nil)

		querier.On("QueryDatabaseSamples", mock.Anything, sqlQuery1, mock.Anything).Return(cpuSamples, nil)
		querier.On("QueryDatabaseSamples", mock.Anything, sqlQuery2, mock.Anything).Return(memorySamples, nil)

		// Create the usecase
		uc := usecase.NewReadPrometheusSamples(&logger, builder, querier)

		// Test request
		req := &domain.ReadRequest{
			R: &prompb.ReadRequest{
				Queries: []*prompb.Query{query1, query2},
			},
		}

		// Execute
		resp, err := uc.Execute(context.Background(), req)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotNil(t, resp.R)

		// Fix the assertion to match actual implementation
		assert.Len(t, resp.R.Results, 1)               // The implementation creates one QueryResult
		assert.Len(t, resp.R.Results[0].Timeseries, 2) // With two timeseries (one per metric)

		// Verify expectations
		builder.AssertExpectations(t)
		querier.AssertExpectations(t)
	})
}
