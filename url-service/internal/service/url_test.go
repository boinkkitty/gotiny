package service

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"

	pb "gotiny/proto/keygen"
	"gotiny/url-service/internal/repository"
)

type mockURLRepo struct {
	storeErr    error
	getResult   *repository.URL
	getErr      error
	listResult  []*repository.URL
	listTotal   int32
	listErr     error
	deleteErr   error
}

func (m *mockURLRepo) Store(_ context.Context, _, _ string, _ int64) error {
	return m.storeErr
}

func (m *mockURLRepo) GetByShortCode(_ context.Context, _ string, _ int64) (*repository.URL, error) {
	return m.getResult, m.getErr
}

func (m *mockURLRepo) ListByUserID(_ context.Context, _ int64, _, _ int32) ([]*repository.URL, int32, error) {
	return m.listResult, m.listTotal, m.listErr
}

func (m *mockURLRepo) DeleteByShortCode(_ context.Context, _ string, _ int64) error {
	return m.deleteErr
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

	code, err := svc.CreateShortURL(context.Background(), "https://example.com", 1)
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

	_, err := svc.CreateShortURL(context.Background(), "https://example.com", 1)
	if err == nil {
		t.Fatal("expected error when keygen fails")
	}
}

func TestCreateShortURL_StoreFails(t *testing.T) {
	svc := NewURLService(
		&mockURLRepo{storeErr: errors.New("unique constraint violation")},
		&mockKeyGenClient{resp: &pb.GetKeyResponse{Key: "abc1234"}},
	)

	_, err := svc.CreateShortURL(context.Background(), "https://example.com", 1)
	if err == nil {
		t.Fatal("expected error when store fails")
	}
}

func TestDeleteURL_Success(t *testing.T) {
	svc := NewURLService(&mockURLRepo{}, &mockKeyGenClient{})

	err := svc.DeleteURL(context.Background(), "abc1234", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteURL_NotOwner(t *testing.T) {
	svc := NewURLService(
		&mockURLRepo{deleteErr: repository.ErrNotOwner},
		&mockKeyGenClient{},
	)

	err := svc.DeleteURL(context.Background(), "abc1234", 99)
	if !errors.Is(err, repository.ErrNotOwner) {
		t.Fatalf("expected ErrNotOwner, got %v", err)
	}
}

func TestListURLs_DefaultLimit(t *testing.T) {
	svc := NewURLService(&mockURLRepo{listTotal: 0}, &mockKeyGenClient{})

	_, total, err := svc.ListURLs(context.Background(), 1, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
}
