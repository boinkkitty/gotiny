package handler_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "gotiny/proto/keygen"
	urlpb "gotiny/proto/url"
	"gotiny/url-service/internal/handler"
	"gotiny/url-service/internal/service"
)

type mockRepo struct{}

func (m *mockRepo) Store(_ context.Context, _, _ string) error { return nil }

type mockRepoFail struct{}

func (m *mockRepoFail) Store(_ context.Context, _, _ string) error {
	return errors.New("db error")
}

type mockKeyGen struct {
	resp *pb.GetKeyResponse
	err  error
}

func (m *mockKeyGen) GetKey(_ context.Context, _ *pb.GetKeyRequest, _ ...grpc.CallOption) (*pb.GetKeyResponse, error) {
	return m.resp, m.err
}

func TestCreateShortURL_EmptyURL(t *testing.T) {
	svc := service.NewURLService(&mockRepo{}, &mockKeyGen{resp: &pb.GetKeyResponse{Key: "x"}})
	h := handler.NewURLHandler(svc)

	_, err := h.CreateShortURL(context.Background(), &urlpb.CreateShortURLRequest{OriginalUrl: ""})
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", st.Code())
	}
}

func TestCreateShortURL_Success(t *testing.T) {
	svc := service.NewURLService(&mockRepo{}, &mockKeyGen{resp: &pb.GetKeyResponse{Key: "short1"}})
	h := handler.NewURLHandler(svc)

	resp, err := h.CreateShortURL(context.Background(), &urlpb.CreateShortURLRequest{OriginalUrl: "https://example.com"})
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

	_, err := h.CreateShortURL(context.Background(), &urlpb.CreateShortURLRequest{OriginalUrl: "https://example.com"})
	if err == nil {
		t.Fatal("expected error when service fails")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal {
		t.Errorf("expected Internal, got %v", st.Code())
	}
}
