package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	keypb "gotiny/proto/keygen"
	urlpb "gotiny/proto/url"

	"gotiny/pkg/config"
	"gotiny/pkg/database"
	"gotiny/pkg/grpcutil"
	"gotiny/pkg/logger"
	"gotiny/url-service/internal/adapter/postgres"
	"gotiny/url-service/internal/server"
	"gotiny/url-service/internal/service"
)

func main() {
	port := config.EnvOr("PORT", "50051")
	dsn := config.EnvOr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/gotiny?sslmode=disable")
	keygenAddr := config.EnvOr("KEY_GEN_ADDR", "localhost:50053")

	logger.Init("url")

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
	repo := postgres.NewURLRepository(pool)
	svc := service.NewURLService(repo, keygenClient)
	h := server.NewURLHandler(svc)

	if err := grpcutil.RunServer(ctx, grpcutil.ServerConfig{
		Port:        port,
		ServiceName: "url",
		RegisterFunc: func(s *grpc.Server) {
			urlpb.RegisterURLServiceServer(s, h)
		},
	}); err != nil {
		slog.Error("serve failed", "error", err)
		os.Exit(1)
	}
}
