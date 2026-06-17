package storeapi

import (
	"context"
	"math"
	"prometheus-postgres-adapter/pkg/domain"
	"sort"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/scality/go-errors"
	"github.com/thanos-io/thanos/pkg/info/infopb"
	"github.com/thanos-io/thanos/pkg/store/labelpb"
	"github.com/thanos-io/thanos/pkg/store/storepb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrSendSeriesResponse is returned when a series response cannot be sent to the client.
var ErrSendSeriesResponse = errors.New("failed to send series response")

// componentType is the Thanos component type advertised through the Info API.
const componentType = "store"

type (
	// QueryBuilder turns a Prometheus query into the SQL executed against the database.
	QueryBuilder interface {
		BuildSQLQuery(*prompb.Query) (string, error)
	}

	// Querier reads samples and metadata from the database.
	Querier interface {
		QueryDatabaseSamples(
			ctx context.Context,
			query string,
			args ...any,
		) ([]*domain.SamplesReadFromDatabase, error)
		MinSampleTimestamp(ctx context.Context) (int64, bool, error)
		LabelNames(ctx context.Context) ([]string, error)
		LabelValues(ctx context.Context, label string) ([]string, error)
	}

	// Server implements the Thanos StoreAPI (storepb.StoreServer) and Info API
	// (infopb.InfoServer) on top of the PostgreSQL adapter. Errors are returned
	// as gRPC status errors and logged once by the server's logging interceptor.
	Server struct {
		builder        QueryBuilder
		querier        Querier
		externalLabels map[string]string
	}
)

var (
	_ storepb.StoreServer = (*Server)(nil)
	_ infopb.InfoServer   = (*Server)(nil)
)

func NewServer(builder QueryBuilder, querier Querier, externalLabels map[string]string) *Server {
	return &Server{
		builder:        builder,
		querier:        querier,
		externalLabels: externalLabels,
	}
}

// Info advertises the store metadata (external labels and time range) to Thanos.
func (s *Server) Info(ctx context.Context, _ *infopb.InfoRequest) (*infopb.InfoResponse, error) {
	minTime := int64(math.MaxInt64)

	oldest, hasData, err := s.querier.MinSampleTimestamp(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get minimum sample timestamp: %v", err)
	}

	if hasData {
		minTime = oldest
	}

	return &infopb.InfoResponse{
		ComponentType: componentType,
		LabelSets:     s.labelSets(),
		Store: &infopb.StoreInfo{
			MinTime: minTime,
			MaxTime: math.MaxInt64,
		},
	}, nil
}

// Series streams the samples matching the request, encoded as XOR chunks.
func (s *Server) Series(req *storepb.SeriesRequest, srv storepb.Store_SeriesServer) error {
	ctx := srv.Context()

	query, matched, err := PromQueryFromSeriesRequest(req, s.externalLabels)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid series request: %v", err)
	}

	if !matched {
		// An external-label matcher excludes this store; nothing to return.
		return nil
	}

	sqlQuery, err := s.builder.BuildSQLQuery(query)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to build SQL query: %v", err)
	}

	rows, err := s.querier.QueryDatabaseSamples(ctx, sqlQuery)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to query samples: %v", err)
	}

	series, err := BuildSeries(rows, s.externalLabels)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to build series: %v", err)
	}

	for i := range series {
		current := series[i]
		if req.SkipChunks {
			current.Chunks = nil
		}

		if err := srv.Send(storepb.NewSeriesResponse(&current)); err != nil {
			return errors.Wrap(ErrSendSeriesResponse, errors.CausedBy(err))
		}
	}

	return nil
}

// LabelNames returns the label names available in the store.
func (s *Server) LabelNames(
	ctx context.Context,
	_ *storepb.LabelNamesRequest,
) (*storepb.LabelNamesResponse, error) {
	names, err := s.querier.LabelNames(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get label names: %v", err)
	}

	unique := map[string]struct{}{model.MetricNameLabel: {}}

	for _, name := range names {
		unique[name] = struct{}{}
	}

	for name := range s.externalLabels {
		unique[name] = struct{}{}
	}

	result := make([]string, 0, len(unique))
	for name := range unique {
		result = append(result, name)
	}

	sort.Strings(result)

	return &storepb.LabelNamesResponse{Names: result}, nil
}

// LabelValues returns the values of a given label name.
func (s *Server) LabelValues(
	ctx context.Context,
	req *storepb.LabelValuesRequest,
) (*storepb.LabelValuesResponse, error) {
	if value, ok := s.externalLabels[req.Label]; ok {
		return &storepb.LabelValuesResponse{Values: []string{value}}, nil
	}

	values, err := s.querier.LabelValues(ctx, req.Label)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get label values: %v", err)
	}

	return &storepb.LabelValuesResponse{Values: values}, nil
}

func (s *Server) labelSets() []labelpb.ZLabelSet {
	if len(s.externalLabels) == 0 {
		return nil
	}

	return labelpb.ZLabelSetsFromPromLabels(labels.FromMap(s.externalLabels))
}
