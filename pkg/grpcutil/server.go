package grpcutil

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type ServerConfig struct {
	Port         string
	ServiceName  string
	RegisterFunc func(s *grpc.Server)
}

func RunServer(ctx context.Context, cfg ServerConfig) error {
	grpcServer := grpc.NewServer()
	cfg.RegisterFunc(grpcServer)

	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus(cfg.ServiceName, healthpb.HealthCheckResponse_SERVING)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.Port))
	if err != nil {
		return fmt.Errorf("listen on port %s: %w", cfg.Port, err)
	}

	go func() {
		<-ctx.Done()
		slog.Info("shutting down")
		healthServer.SetServingStatus(cfg.ServiceName, healthpb.HealthCheckResponse_NOT_SERVING)

		stopped := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(stopped)
		}()

		select {
		case <-stopped:
		case <-time.After(10 * time.Second):
			slog.Warn("graceful stop timed out, forcing shutdown")
			grpcServer.Stop()
		}
	}()

	slog.Info(cfg.ServiceName+" service starting", "port", cfg.Port)
	return grpcServer.Serve(lis)
}
