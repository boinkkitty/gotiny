package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	redirectpb "gotiny/proto/redirect"
	urlpb "gotiny/proto/url"
	userpb "gotiny/proto/user"

	"gotiny/api-gateway/internal/handler"
)

func main() {
	port := envOr("PORT", "8080")
	urlServiceAddr := envOr("URL_SERVICE_ADDR", "localhost:50051")
	redirectServiceAddr := envOr("REDIRECT_SERVICE_ADDR", "localhost:50052")
	userServiceAddr := envOr("USER_SERVICE_ADDR", "localhost:50054")
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		slog.Error("JWT_SECRET environment variable is required")
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)).With(
		"service", "api-gateway",
	))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	urlConn, err := grpc.NewClient(urlServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Error("url service connection failed", "error", err, "addr", urlServiceAddr)
		os.Exit(1)
	}
	defer urlConn.Close()

	redirectConn, err := grpc.NewClient(redirectServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Error("redirect service connection failed", "error", err, "addr", redirectServiceAddr)
		os.Exit(1)
	}
	defer redirectConn.Close()

	userConn, err := grpc.NewClient(userServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Error("user service connection failed", "error", err, "addr", userServiceAddr)
		os.Exit(1)
	}
	defer userConn.Close()

	urlClient := urlpb.NewURLServiceClient(urlConn)
	redirectClient := redirectpb.NewRedirectServiceClient(redirectConn)
	userClient := userpb.NewUserServiceClient(userConn)

	h := handler.NewHTTPHandler(urlClient, redirectClient, userClient)
	authMiddleware := handler.JWTMiddleware([]byte(jwtSecret))

	mux := http.NewServeMux()

	// Public routes (no auth)
	mux.HandleFunc("POST /register", h.Register)
	mux.HandleFunc("POST /login", h.Login)
	mux.HandleFunc("POST /refresh", h.Refresh)
	mux.HandleFunc("POST /logout", h.Logout)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	// Protected routes (require JWT)
	protectedMux := http.NewServeMux()
	protectedMux.HandleFunc("POST /shorten", h.Shorten)
	protectedMux.HandleFunc("GET /urls", h.ListURLs)
	protectedMux.HandleFunc("DELETE /urls/{code}", h.DeleteURL)
	mux.Handle("POST /shorten", authMiddleware(protectedMux))
	mux.Handle("GET /urls", authMiddleware(protectedMux))
	mux.Handle("DELETE /urls/{code}", authMiddleware(protectedMux))

	// Redirect (public, no auth)
	mux.HandleFunc("GET /{code}", h.Redirect)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", port),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		slog.Info("shutting down")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		server.Shutdown(shutdownCtx)
	}()

	slog.Info("api gateway starting", "port", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
