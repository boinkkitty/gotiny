package service

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"

	pb "gotiny/proto/keygen"
)

type mockURLRepo struct {
	storeErr error
}

func (m *mockURLRepo) Store(_ context.Context, _, _ string) error {
	return m.storeErr
}

type mockKeyGenClient struct {
	resp *pb.GetKeyResponse
	err  error
}

func (m *mockKeyGenClient) GetKey(_ context.Context, _ *pb.GetKeyRequest, _ ...grpc.CallOption) (*pb.GetKeyResponse, error) {
	return m.resp, m.err
}

func TestCreateShortURL_Success(t *testing.T) {
	svc := NewURLService(
		&mockURLRepo{},
		&mockKeyGenClient{resp: &pb.GetKeyResponse{Key: "abc1234"}},
	)

	code, err := svc.CreateShortURL(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != "abc1234" {
		t.Errorf("expected abc1234, got %q", code)
	}
}

func TestCreateShortURL_KeyGenFails(t *testing.T) {
	svc := NewURLService(
		&mockURLRepo{},
		&mockKeyGenClient{err: errors.New("keygen unavailable")},
	)

	_, err := svc.CreateShortURL(context.Background(), "https://example.com")
	if err == nil {
		t.Fatal("expected error when keygen fails")
	}
}

func TestCreateShortURL_StoreFails(t *testing.T) {
	svc := NewURLService(
		&mockURLRepo{storeErr: errors.New("unique constraint violation")},
		&mockKeyGenClient{resp: &pb.GetKeyResponse{Key: "abc1234"}},
	)

	_, err := svc.CreateShortURL(context.Background(), "https://example.com")
	if err == nil {
		t.Fatal("expected error when store fails")
	}
}
