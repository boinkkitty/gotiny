package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	keypb "gotiny/proto/keygen"
	urlpb "gotiny/proto/url"

	"gotiny/pkg/database"
	"gotiny/url-service/internal/handler"
	"gotiny/url-service/internal/repository"
	"gotiny/url-service/internal/service"
)

func main() {
	port := envOr("PORT", "50051")
	dsn := envOr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/gotiny?sslmode=disable")
	keygenAddr := envOr("KEY_GEN_ADDR", "localhost:50053")

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)).With(
		"service", "url",
	))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	pool, err := database.NewPool(ctx, dsn)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	keygenConn, err := grpc.NewClient(keygenAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Error("key-gen connection failed", "error", err, "addr", keygenAddr)
		os.Exit(1)
	}
	defer keygenConn.Close()

	keygenClient := keypb.NewKeyGenServiceClient(keygenConn)
	repo := repository.NewURLRepository(pool)
	svc := service.NewURLService(repo, keygenClient)

	grpcServer := grpc.NewServer()
	urlpb.RegisterURLServiceServer(grpcServer, handler.NewURLHandler(svc))

	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("url", healthpb.HealthCheckResponse_SERVING)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		slog.Error("listen failed", "error", err, "port", port)
		os.Exit(1)
	}

	go func() {
		<-ctx.Done()
		slog.Info("shutting down")
		healthServer.SetServingStatus("url", healthpb.HealthCheckResponse_NOT_SERVING)
		grpcServer.GracefulStop()
	}()

	slog.Info("url service starting", "port", port)
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
