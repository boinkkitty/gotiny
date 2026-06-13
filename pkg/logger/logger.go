package logger

import (
	"log/slog"
	"os"
)

func Init(serviceName string) {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)).With(
		"service", serviceName,
	))
}
