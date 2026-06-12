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
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	pb "gotiny/proto/user"

	"gotiny/pkg/database"
	"gotiny/user-service/internal/handler"
	"gotiny/user-service/internal/repository"
	"gotiny/user-service/internal/service"
)

func main() {
	port := envOr("PORT", "50054")
	dsn := envOr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/gotiny?sslmode=disable")
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		slog.Error("JWT_SECRET environment variable is required")
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)).With(
		"service", "user",
	))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	pool, err := database.NewPool(ctx, dsn)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	repo := repository.NewUserRepository(pool)
	svc := service.NewUserService(repo, jwtSecret)

	grpcServer := grpc.NewServer()
	pb.RegisterUserServiceServer(grpcServer, handler.NewUserHandler(svc))

	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("user", healthpb.HealthCheckResponse_SERVING)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		slog.Error("listen failed", "error", err, "port", port)
		os.Exit(1)
	}

	go func() {
		<-ctx.Done()
		slog.Info("shutting down")
		healthServer.SetServingStatus("user", healthpb.HealthCheckResponse_NOT_SERVING)
		grpcServer.GracefulStop()
	}()

	slog.Info("user service starting", "port", port)
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
