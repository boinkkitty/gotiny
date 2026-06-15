package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"google.golang.org/grpc"

	pb "gotiny/proto/keygen"

	"gotiny/key-gen-service/internal/adapter/postgres"
	redisadapter "gotiny/key-gen-service/internal/adapter/redis"
	"gotiny/key-gen-service/internal/port"
	"gotiny/key-gen-service/internal/server"
	"gotiny/key-gen-service/internal/service"
	"gotiny/pkg/config"
	"gotiny/pkg/database"
	"gotiny/pkg/grpcutil"
	"gotiny/pkg/logger"
)

func main() {
	listenPort := config.EnvOr("PORT", "50053")
	dsn := config.EnvOr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/gotiny?sslmode=disable")
	redisAddr := config.EnvOr("REDIS_ADDR", "")

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

	var (
		queue port.KeyQueue
		lock  port.RefillLock
	)

	if redisAddr != "" {
		rdb := goredis.NewClient(&goredis.Options{Addr: redisAddr})
		defer rdb.Close()
		queue = redisadapter.NewKeyQueue(rdb)
		lock = redisadapter.NewRefillLock(rdb, instanceID, cfg.LockTTL)
		slog.Info("redis queue enabled", "addr", redisAddr)
	}

	svc := service.NewKeyGenService(repo, queue, lock, cfg)

	if err := svc.Init(ctx); err != nil {
		slog.Error("service init failed", "error", err)
		os.Exit(1)
	}

	h := server.NewKeyGenHandler(svc)

	if err := grpcutil.RunServer(ctx, grpcutil.ServerConfig{
		Port:        listenPort,
		ServiceName: "keygen",
		RegisterFunc: func(s *grpc.Server) {
			pb.RegisterKeyGenServiceServer(s, h)
		},
	}); err != nil {
		slog.Error("serve failed", "error", err)
		os.Exit(1)
	}
}
