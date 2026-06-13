package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	pb "gotiny/proto/keygen"

	"gotiny/key-gen-service/internal/adapter/postgres"
	"gotiny/key-gen-service/internal/server"
	"gotiny/key-gen-service/internal/service"
	"gotiny/pkg/config"
	"gotiny/pkg/database"
	"gotiny/pkg/grpcutil"
	"gotiny/pkg/logger"
)

func main() {
	port := config.EnvOr("PORT", "50053")
	dsn := config.EnvOr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/gotiny?sslmode=disable")

	instanceID := uuid.New().String()

	logger.Init("key-gen")
	slog.SetDefault(slog.Default().With("instance_id", instanceID))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	pool, err := database.NewPool(ctx, dsn)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	repo := postgres.NewKeysRepository(pool)
	cfg := service.DefaultConfig(instanceID)
	svc := service.NewKeyGenService(repo, cfg)

	if err := svc.Init(ctx); err != nil {
		slog.Error("service init failed", "error", err)
		os.Exit(1)
	}

	h := server.NewKeyGenHandler(svc)

	if err := grpcutil.RunServer(ctx, grpcutil.ServerConfig{
		Port:        port,
		ServiceName: "keygen",
		RegisterFunc: func(s *grpc.Server) {
			pb.RegisterKeyGenServiceServer(s, h)
		},
	}); err != nil {
		slog.Error("serve failed", "error", err)
		os.Exit(1)
	}
}
