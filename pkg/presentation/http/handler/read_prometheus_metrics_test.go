package handler_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"prometheus-postgres-adapter/pkg/domain"
	"prometheus-postgres-adapter/pkg/presentation/http/handler"
	"prometheus-postgres-adapter/pkg/usecase"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
	"github.com/scality/go-errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errUnsupportedMatcher stands for what the query builder reports when a
// matcher cannot be translated, an unsupported inline regex option for one.
var errUnsupportedMatcher = errors.New("unsupported matcher")

type stubBuilder struct{ err error }

func (b stubBuilder) BuildSQLQuery(*prompb.Query) (string, error) { return "SELECT 1", b.err }

type stubQuerier struct{}

func (stubQuerier) QueryDatabaseSamples(
	context.Context,
	string,
	...any,
) ([]*domain.SamplesReadFromDatabase, error) {
	return nil, nil
}

func readRequest(t *testing.T) *http.Request {
	t.Helper()

	body, err := proto.Marshal(&prompb.ReadRequest{Queries: []*prompb.Query{{}}})
	require.NoError(t, err)

	return httptest.NewRequest(http.MethodPost, "/read", bytes.NewReader(snappy.Encode(nil, body)))
}

func TestReadPrometheusMetrics_Handle(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)

	t.Run("a matcher that cannot be translated is a bad request", func(t *testing.T) {
		useCase := usecase.NewReadPrometheusSamples(logger, stubBuilder{err: errUnsupportedMatcher}, stubQuerier{})
		recorder := httptest.NewRecorder()

		handler.NewReadPrometheusMetrics(useCase, logger).Handle().ServeHTTP(recorder, readRequest(t))

		// The store works; the request asked for something it cannot express.
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})

	t.Run("a query that runs is answered", func(t *testing.T) {
		useCase := usecase.NewReadPrometheusSamples(logger, stubBuilder{}, stubQuerier{})
		recorder := httptest.NewRecorder()

		handler.NewReadPrometheusMetrics(useCase, logger).Handle().ServeHTTP(recorder, readRequest(t))

		assert.Equal(t, http.StatusOK, recorder.Code)
	})
}
