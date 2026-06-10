package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	pb "gotiny/proto/redirect"

	"gotiny/pkg/database"
	"gotiny/redirect-service/internal/handler"
	"gotiny/redirect-service/internal/repository"
)

func main() {
	port := envOr("PORT", "50052")
	dsn := envOr("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/gotiny?sslmode=disable")
	redisAddr := envOr("REDIS_ADDR", "localhost:6379")

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)).With(
		"service", "redirect",
	))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	pool, err := database.NewPool(ctx, dsn)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Warn("redis connection failed, running without cache", "error", err)
		rdb = nil
	} else {
		slog.Info("redis connected", "addr", redisAddr)
	}

	var repo *repository.URLRepository
	if rdb != nil {
		repo = repository.NewURLRepository(pool, rdb)
	} else {
		repo = repository.NewURLRepository(pool, nil)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterRedirectServiceServer(grpcServer, handler.NewRedirectHandler(repo))

	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("redirect", healthpb.HealthCheckResponse_SERVING)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		slog.Error("listen failed", "error", err, "port", port)
		os.Exit(1)
	}

	go func() {
		<-ctx.Done()
		slog.Info("shutting down")
		healthServer.SetServingStatus("redirect", healthpb.HealthCheckResponse_NOT_SERVING)
		grpcServer.GracefulStop()
		if rdb != nil {
			rdb.Close()
		}
	}()

	slog.Info("redirect service starting", "port", port)
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
