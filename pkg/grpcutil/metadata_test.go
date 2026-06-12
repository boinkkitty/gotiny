package grpcutil

import (
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestUserIDFromContext_Success(t *testing.T) {
	md := metadata.Pairs(UserIDKey, "42")
	ctx := metadata.NewIncomingContext(t.Context(), md)

	id, ok := UserIDFromContext(ctx)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if id != 42 {
		t.Errorf("expected 42, got %d", id)
	}
}

func TestUserIDFromContext_Missing(t *testing.T) {
	_, ok := UserIDFromContext(t.Context())
	if ok {
		t.Fatal("expected ok=false for missing metadata")
	}
}

func TestUserIDFromContext_InvalidValue(t *testing.T) {
	md := metadata.Pairs(UserIDKey, "not-a-number")
	ctx := metadata.NewIncomingContext(t.Context(), md)

	_, ok := UserIDFromContext(ctx)
	if ok {
		t.Fatal("expected ok=false for invalid value")
	}
}

func TestContextWithUserID(t *testing.T) {
	ctx := ContextWithUserID(t.Context(), 99)
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("expected outgoing metadata")
	}
	vals := md.Get(UserIDKey)
	if len(vals) == 0 || vals[0] != "99" {
		t.Errorf("expected '99', got %v", vals)
	}
}
