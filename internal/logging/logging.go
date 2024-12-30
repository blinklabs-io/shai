package logging

import (
	"log/slog"
	"os"
	"time"

	"github.com/blinklabs-io/shai/internal/config"
)

var globalLogger *slog.Logger

func Configure() {
	cfg := config.GetConfig()
	var level slog.Level
	switch cfg.Logging.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				// Format the time attribute to use RFC3339 or your custom format
				// Rename the time key to timestamp
				return slog.String(
					"timestamp",
					a.Value.Time().Format(time.RFC3339),
				)
			}
			return a
		},
		Level: level,
	})
	globalLogger = slog.New(handler).With("component", "main")

}

func GetLogger() *slog.Logger {
	if globalLogger == nil {
		Configure()
	}
	return globalLogger
}
