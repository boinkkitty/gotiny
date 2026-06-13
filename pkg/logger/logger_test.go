package logger

import (
	"log/slog"
	"testing"
)

func TestInit(t *testing.T) {
	Init("test-service")

	handler := slog.Default().Handler()
	if handler == nil {
		t.Fatal("expected non-nil handler after Init")
	}
}
