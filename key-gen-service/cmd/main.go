package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	pb "gotiny/proto/keygen"

	"gotiny/key-gen-service/internal/handler"
	"gotiny/key-gen-service/internal/repository"
	"gotiny/key-gen-service/internal/service"
	"gotiny/pkg/database"
)

func main() {
	port := envOr("PORT", "50053")
	dsn := envOr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/gotiny?sslmode=disable")

	instanceID := uuid.New().String()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)).With(
		"service", "key-gen",
		"instance_id", instanceID,
	))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	pool, err := database.NewPool(ctx, dsn)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	repo := repository.NewKeysRepository(pool)
	cfg := service.DefaultConfig(instanceID)
	svc := service.NewKeyGenService(repo, cfg)

	if err := svc.Init(ctx); err != nil {
		slog.Error("service init failed", "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterKeyGenServiceServer(grpcServer, handler.NewKeyGenHandler(svc))

	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("keygen", healthpb.HealthCheckResponse_SERVING)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		slog.Error("listen failed", "error", err, "port", port)
		os.Exit(1)
	}

	go func() {
		<-ctx.Done()
		slog.Info("shutting down")
		healthServer.SetServingStatus("keygen", healthpb.HealthCheckResponse_NOT_SERVING)
		grpcServer.GracefulStop()
	}()

	slog.Info("key-gen service starting", "port", port)
	if err := grpcServer.Serve(lis); err != nil {
		slog.Error("serve failed", "error", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
