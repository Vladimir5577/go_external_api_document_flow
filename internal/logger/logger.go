package logger

import (
	"log/slog"
	"os"
)

// Setup задаёт глобальный логгер: для local/dev — текст и Debug, иначе — JSON и Info.
func Setup(env string) {
	var handler slog.Handler

	if env == "local" || env == "dev" {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	} else {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
}
