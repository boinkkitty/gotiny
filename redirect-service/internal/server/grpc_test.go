package server_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "gotiny/proto/redirect"
	"gotiny/redirect-service/internal/domain"
	"gotiny/redirect-service/internal/server"
)

type mockResolver struct {
	url string
	err error
}

func (m *mockResolver) Resolve(_ context.Context, _ string) (string, error) {
	return m.url, m.err
}

func (m *mockResolver) InvalidateCache(_ context.Context, _ string) error {
	return nil
}

func TestResolve_Success(t *testing.T) {
	h := server.NewRedirectHandler(&mockResolver{url: "https://example.com"})

	resp, err := h.Resolve(context.Background(), &pb.ResolveRequest{ShortCode: "abc1234"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.OriginalUrl != "https://example.com" {
		t.Errorf("expected https://example.com, got %q", resp.OriginalUrl)
	}
}

func TestResolve_EmptyShortCode(t *testing.T) {
	h := server.NewRedirectHandler(&mockResolver{})

	_, err := h.Resolve(context.Background(), &pb.ResolveRequest{ShortCode: ""})
	if err == nil {
		t.Fatal("expected error for empty short code")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", st.Code())
	}
}

func TestResolve_NotFound(t *testing.T) {
	h := server.NewRedirectHandler(&mockResolver{err: domain.ErrNotFound})

	_, err := h.Resolve(context.Background(), &pb.ResolveRequest{ShortCode: "missing"})
	if err == nil {
		t.Fatal("expected error for not found")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Errorf("expected NotFound, got %v", st.Code())
	}
}

func TestResolve_InternalError(t *testing.T) {
	h := server.NewRedirectHandler(&mockResolver{err: errors.New("db connection lost")})

	_, err := h.Resolve(context.Background(), &pb.ResolveRequest{ShortCode: "abc1234"})
	if err == nil {
		t.Fatal("expected error for internal failure")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("expected Unavailable, got %v", st.Code())
	}
}

func TestInvalidateCache_Success(t *testing.T) {
	h := server.NewRedirectHandler(&mockResolver{})

	resp, err := h.InvalidateCache(context.Background(), &pb.InvalidateCacheRequest{ShortCode: "abc1234"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestInvalidateCache_EmptyShortCode(t *testing.T) {
	h := server.NewRedirectHandler(&mockResolver{})

	_, err := h.InvalidateCache(context.Background(), &pb.InvalidateCacheRequest{ShortCode: ""})
	if err == nil {
		t.Fatal("expected error for empty short code")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", st.Code())
	}
}
