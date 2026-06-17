package di

import (
	"context"
	"log/slog"
	"prometheus-postgres-adapter/pkg/presentation/storeapi"

	"github.com/thanos-io/thanos/pkg/info/infopb"
	"github.com/thanos-io/thanos/pkg/store/storepb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

func (c *Container) GetGRPCServer() *grpc.Server {
	if c.grpcServer == nil {
		logger := c.GetLogger()

		c.grpcServer = grpc.NewServer(
			grpc.ChainUnaryInterceptor(logErrorUnaryInterceptor(logger)),
			grpc.ChainStreamInterceptor(logErrorStreamInterceptor(logger)),
		)

		storeServer := storeapi.NewServer(
			c.getSQLQueryBuilder(),
			c.getPostgreSQLClient(),
			c.cfg.StoreAPI.ExternalLabels,
		)

		storepb.RegisterStoreServer(c.grpcServer, storeServer)
		infopb.RegisterInfoServer(c.grpcServer, storeServer)

		// Expose the standard gRPC health service for Kubernetes gRPC probes.
		healthServer := health.NewServer()
		healthServer.SetServingStatus("", healthgrpc.HealthCheckResponse_SERVING)
		healthgrpc.RegisterHealthServer(c.grpcServer, healthServer)
	}

	return c.grpcServer
}

// logErrorUnaryInterceptor logs failed unary gRPC calls once, at the server
// boundary, so the handlers can simply return their status errors.
func logErrorUnaryInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		resp, err := handler(ctx, req)
		if err != nil {
			code := status.Code(err)
			logger.LogAttrs(ctx, logLevelForCode(code), "gRPC call failed",
				slog.String("method", info.FullMethod),
				slog.String("code", code.String()),
				slog.Any("error", err),
			)
		}

		return resp, err //nolint:wrapcheck // propagate the gRPC status error verbatim
	}
}

// logErrorStreamInterceptor logs failed streaming gRPC calls once, at the server
// boundary.
func logErrorStreamInterceptor(logger *slog.Logger) grpc.StreamServerInterceptor {
	return func(
		srv any,
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		err := handler(srv, stream)
		if err != nil {
			code := status.Code(err)
			logger.LogAttrs(stream.Context(), logLevelForCode(code), "gRPC stream failed",
				slog.String("method", info.FullMethod),
				slog.String("code", code.String()),
				slog.Any("error", err),
			)
		}

		return err //nolint:wrapcheck // propagate the gRPC status error verbatim
	}
}

// logLevelForCode maps a gRPC status code to a slog level. Client-class codes
// (the caller's mistake, e.g. an invalid matcher) are logged below Error so
// they neither pollute the error logs nor trip error-rate alerting; only
// server-class codes are logged at Error.
func logLevelForCode(code codes.Code) slog.Level {
	switch code { //nolint:exhaustive // the default deliberately covers all server-class codes
	case codes.InvalidArgument, codes.NotFound, codes.AlreadyExists,
		codes.FailedPrecondition, codes.OutOfRange, codes.Unauthenticated,
		codes.PermissionDenied, codes.Canceled:
		return slog.LevelWarn
	default:
		return slog.LevelError
	}
}
