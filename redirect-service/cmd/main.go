package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	goredis "github.com/redis/go-redis/v9"
	"google.golang.org/grpc"

	pb "gotiny/proto/redirect"

	"gotiny/pkg/config"
	"gotiny/pkg/database"
	"gotiny/pkg/grpcutil"
	"gotiny/pkg/logger"
	"gotiny/redirect-service/internal/adapter/postgres"
	redisadapter "gotiny/redirect-service/internal/adapter/redis"
	svcport "gotiny/redirect-service/internal/port"
	"gotiny/redirect-service/internal/server"
	"gotiny/redirect-service/internal/service"
)

func main() {
	port := config.EnvOr("PORT", "50052")
	dsn := config.EnvOr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/gotiny?sslmode=disable")
	redisAddr := config.EnvOr("REDIS_ADDR", "localhost:6379")

	logger.Init("redirect")

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	pool, err := database.NewPool(ctx, dsn)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	reader := postgres.NewURLRepository(pool)

	var cache svcport.URLCache
	rdb := goredis.NewClient(&goredis.Options{Addr: redisAddr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Warn("redis connection failed, running without cache", "error", err)
		rdb.Close()
	} else {
		slog.Info("redis connected", "addr", redisAddr)
		cache = redisadapter.NewURLCache(rdb)
		defer rdb.Close()
	}

	svc := service.NewRedirectService(reader, cache)
	h := server.NewRedirectHandler(svc)

	if err := grpcutil.RunServer(ctx, grpcutil.ServerConfig{
		Port:        port,
		ServiceName: "redirect",
		RegisterFunc: func(s *grpc.Server) {
			pb.RegisterRedirectServiceServer(s, h)
		},
	}); err != nil {
		slog.Error("serve failed", "error", err)
		os.Exit(1)
	}
}
