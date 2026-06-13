package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	pb "gotiny/proto/user"

	"gotiny/pkg/config"
	"gotiny/pkg/database"
	"gotiny/pkg/grpcutil"
	"gotiny/pkg/logger"
	"gotiny/user-service/internal/adapter/postgres"
	"gotiny/user-service/internal/server"
	"gotiny/user-service/internal/service"
)

func main() {
	port := config.EnvOr("PORT", "50054")
	dsn := config.EnvOr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/gotiny?sslmode=disable")
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		slog.Error("JWT_SECRET environment variable is required")
		os.Exit(1)
	}

	logger.Init("user")

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	pool, err := database.NewPool(ctx, dsn)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	repo := postgres.NewUserRepository(pool)
	svc := service.NewUserService(repo, jwtSecret)
	h := server.NewUserHandler(svc)

	if err := grpcutil.RunServer(ctx, grpcutil.ServerConfig{
		Port:        port,
		ServiceName: "user",
		RegisterFunc: func(s *grpc.Server) {
			pb.RegisterUserServiceServer(s, h)
		},
	}); err != nil {
		slog.Error("serve failed", "error", err)
		os.Exit(1)
	}
}
