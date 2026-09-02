package logging

import (
	"os"
	"time"

	"github.com/maxcraig112/burrow/internal/envfile"
	"github.com/maxcraig112/env"
	"github.com/rs/zerolog"
)

// New builds a timestamped console logger, loads the -env file into the process
// environment, and sets the level from LOG_LEVEL (defaulting to info).
func New() zerolog.Logger {
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).
		With().Timestamp().Logger()
	envfile.Load(logger)
	return logger.Level(levelFromEnv(logger))
}

func levelFromEnv(logger zerolog.Logger) zerolog.Level {
	lvl := env.Get("LOG_LEVEL", "info")
	level, err := zerolog.ParseLevel(lvl)
	if err != nil {
		logger.Warn().Str("value", lvl).Msg("invalid LOG_LEVEL, defaulting to info")
		return zerolog.InfoLevel
	}
	return level
}
