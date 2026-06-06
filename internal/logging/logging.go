package logging

import (
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/blinklabs-io/shai/internal/config"
)

var (
	mu           sync.RWMutex
	globalLogger *slog.Logger
)

// Configure builds the global logger from the current configuration, replacing
// any previously configured logger. It is safe for concurrent use.
func Configure() {
	logger := buildLogger()
	mu.Lock()
	globalLogger = logger
	mu.Unlock()
}

// GetLogger returns the global logger, configuring it on first use. It is safe
// for concurrent use.
func GetLogger() *slog.Logger {
	mu.RLock()
	logger := globalLogger
	mu.RUnlock()
	if logger != nil {
		return logger
	}

	mu.Lock()
	defer mu.Unlock()
	// Re-check under the write lock: another goroutine may have configured the
	// logger between releasing the read lock and acquiring the write lock.
	if globalLogger == nil {
		globalLogger = buildLogger()
	}
	return globalLogger
}

// buildLogger constructs a logger from the current configuration. It touches no
// shared state, so it runs outside the lock.
func buildLogger() *slog.Logger {
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
	return slog.New(handler).With("component", "main")
}
