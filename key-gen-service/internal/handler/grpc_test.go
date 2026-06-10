package handler_test

import (
	"context"
	"testing"

	"gotiny/key-gen-service/internal/handler"
	"gotiny/key-gen-service/internal/service"
	pb "gotiny/proto/keygen"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockRepo struct {
	claimFunc func(ctx context.Context, instanceID string, batchSize int) ([]string, error)
}

func (m *mockRepo) ClaimBatch(ctx context.Context, instanceID string, batchSize int) ([]string, error) {
	return m.claimFunc(ctx, instanceID, batchSize)
}

func (m *mockRepo) CountAvailable(_ context.Context) (int64, error) { return 100000, nil }
func (m *mockRepo) GenerateBatch(_ context.Context, count int) (int64, error) {
	return int64(count), nil
}

func newTestService(repo *mockRepo) *service.KeyGenService {
	svc := service.NewKeyGenService(repo, service.Config{
		InstanceID: "test", BufferSize: 10, RefillAt: 2, PoolThreshold: 1000, PoolBatchSize: 100,
	})
	svc.Init(context.Background())
	return svc
}

func TestGetKeySuccess(t *testing.T) {
	repo := &mockRepo{
		claimFunc: func(_ context.Context, _ string, n int) ([]string, error) {
			keys := make([]string, n)
			for i := range keys {
				keys[i] = "abc1234"
			}
			return keys, nil
		},
	}

	h := handler.NewKeyGenHandler(newTestService(repo))
	resp, err := h.GetKey(context.Background(), &pb.GetKeyRequest{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Key == "" {
		t.Error("expected non-empty key")
	}
}

func TestGetKeyPoolExhausted(t *testing.T) {
	repo := &mockRepo{
		claimFunc: func(_ context.Context, _ string, _ int) ([]string, error) {
			return nil, nil
		},
	}

	svc := service.NewKeyGenService(repo, service.Config{
		InstanceID: "test", BufferSize: 5, RefillAt: 0, PoolThreshold: 1000, PoolBatchSize: 100,
	})
	svc.Init(context.Background())

	h := handler.NewKeyGenHandler(svc)
	_, err := h.GetKey(context.Background(), &pb.GetKeyRequest{})
	if err == nil {
		t.Fatal("expected error")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.ResourceExhausted {
		t.Errorf("expected ResourceExhausted, got %v", st.Code())
	}
}
