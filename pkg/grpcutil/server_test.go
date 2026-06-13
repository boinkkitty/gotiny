package grpcutil

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
)

func TestRunServer_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- RunServer(ctx, ServerConfig{
			Port:        "0",
			ServiceName: "test",
			RegisterFunc: func(s *grpc.Server) {
			},
		})
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not shut down in time")
	}
}
