package handler_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "gotiny/proto/keygen"
	urlpb "gotiny/proto/url"
	"gotiny/url-service/internal/handler"
	"gotiny/url-service/internal/repository"
	"gotiny/url-service/internal/service"
)

type mockRepo struct{}

func (m *mockRepo) Store(_ context.Context, _, _ string, _ int64) error { return nil }
func (m *mockRepo) GetByShortCode(_ context.Context, _ string, _ int64) (*repository.URL, error) {
	return &repository.URL{ShortCode: "x", OriginalURL: "https://example.com"}, nil
}
func (m *mockRepo) ListByUserID(_ context.Context, _ int64, _, _ int32) ([]*repository.URL, int32, error) {
	return nil, 0, nil
}
func (m *mockRepo) DeleteByShortCode(_ context.Context, _ string, _ int64) error { return nil }

type mockRepoFail struct{}

func (m *mockRepoFail) Store(_ context.Context, _, _ string, _ int64) error {
	return errors.New("db error")
}
func (m *mockRepoFail) GetByShortCode(_ context.Context, _ string, _ int64) (*repository.URL, error) {
	return nil, errors.New("db error")
}
func (m *mockRepoFail) ListByUserID(_ context.Context, _ int64, _, _ int32) ([]*repository.URL, int32, error) {
	return nil, 0, errors.New("db error")
}
func (m *mockRepoFail) DeleteByShortCode(_ context.Context, _ string, _ int64) error {
	return errors.New("db error")
}

type mockKeyGen struct {
	resp *pb.GetKeyResponse
	err  error
}

func (m *mockKeyGen) GetKey(_ context.Context, _ *pb.GetKeyRequest, _ ...grpc.CallOption) (*pb.GetKeyResponse, error) {
	return m.resp, m.err
}

func ctxWithUserID(userID string) context.Context {
	md := metadata.Pairs("x-user-id", userID)
	return metadata.NewIncomingContext(context.Background(), md)
}

func TestCreateShortURL_EmptyURL(t *testing.T) {
	svc := service.NewURLService(&mockRepo{}, &mockKeyGen{resp: &pb.GetKeyResponse{Key: "x"}})
	h := handler.NewURLHandler(svc)

	ctx := ctxWithUserID("1")
	_, err := h.CreateShortURL(ctx, &urlpb.CreateShortURLRequest{OriginalUrl: ""})
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", st.Code())
	}
}

func TestCreateShortURL_MissingUserID(t *testing.T) {
	svc := service.NewURLService(&mockRepo{}, &mockKeyGen{resp: &pb.GetKeyResponse{Key: "x"}})
	h := handler.NewURLHandler(svc)

	_, err := h.CreateShortURL(context.Background(), &urlpb.CreateShortURLRequest{OriginalUrl: "https://example.com"})
	if err == nil {
		t.Fatal("expected error for missing user_id")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", st.Code())
	}
}

func TestCreateShortURL_Success(t *testing.T) {
	svc := service.NewURLService(&mockRepo{}, &mockKeyGen{resp: &pb.GetKeyResponse{Key: "short1"}})
	h := handler.NewURLHandler(svc)

	ctx := ctxWithUserID("1")
	resp, err := h.CreateShortURL(ctx, &urlpb.CreateShortURLRequest{OriginalUrl: "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ShortCode != "short1" {
		t.Errorf("expected short1, got %q", resp.ShortCode)
	}
}

func TestCreateShortURL_ServiceError(t *testing.T) {
	svc := service.NewURLService(&mockRepoFail{}, &mockKeyGen{resp: &pb.GetKeyResponse{Key: "x"}})
	h := handler.NewURLHandler(svc)

	ctx := ctxWithUserID("1")
	_, err := h.CreateShortURL(ctx, &urlpb.CreateShortURLRequest{OriginalUrl: "https://example.com"})
	if err == nil {
		t.Fatal("expected error when service fails")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal {
		t.Errorf("expected Internal, got %v", st.Code())
	}
}

func TestDeleteURL_NotFound(t *testing.T) {
	repo := &mockRepoNotFound{}
	svc := service.NewURLService(repo, &mockKeyGen{})
	h := handler.NewURLHandler(svc)

	_, err := h.DeleteURL(context.Background(), &urlpb.DeleteURLRequest{ShortCode: "missing", UserId: 1})
	if err == nil {
		t.Fatal("expected error for not found")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Errorf("expected NotFound, got %v", st.Code())
	}
}

func TestDeleteURL_NotOwner(t *testing.T) {
	repo := &mockRepoNotOwner{}
	svc := service.NewURLService(repo, &mockKeyGen{})
	h := handler.NewURLHandler(svc)

	_, err := h.DeleteURL(context.Background(), &urlpb.DeleteURLRequest{ShortCode: "abc1234", UserId: 99})
	if err == nil {
		t.Fatal("expected error for not owner")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", st.Code())
	}
}

type mockRepoNotFound struct{ mockRepo }

func (m *mockRepoNotFound) DeleteByShortCode(_ context.Context, _ string, _ int64) error {
	return repository.ErrNotFound
}

type mockRepoNotOwner struct{ mockRepo }

func (m *mockRepoNotOwner) DeleteByShortCode(_ context.Context, _ string, _ int64) error {
	return repository.ErrNotOwner
}
