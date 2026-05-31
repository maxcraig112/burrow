package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/maxcraig112/burrow/internal/exchange"
	"github.com/maxcraig112/burrow/internal/transport"
	"github.com/rs/zerolog"
)

const ttl = 5 * time.Minute

func main() {
	envFile := flag.String("env", ".env", "path to .env file")
	flag.Parse()

	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).
		With().Timestamp().Logger()

	if err := godotenv.Load(*envFile); err != nil && !os.IsNotExist(err) {
		logger.Warn().Str("path", *envFile).Err(err).Msg("could not load env file")
	}

	logger = logger.Level(parseLevel(logger))

	ex := exchange.New(ttl, logger)
	h := transport.NewHandler(ex, ttl, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /send", h.Send)
	mux.HandleFunc("POST /receive", h.Receive)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
		// ReadTimeout covers the initial HTTP upgrade; WriteTimeout must
		// accommodate the full WebSocket session lifetime.
		ReadTimeout:  10 * time.Second,
		WriteTimeout: ttl + 10*time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info().Str("addr", srv.Addr).Msg("server listening")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("server error")
		}
	}()

	<-ctx.Done()
	logger.Info().Msg("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("shutdown error")
	}
}

func parseLevel(logger zerolog.Logger) zerolog.Level {
	lvl := os.Getenv("LOG_LEVEL")
	if lvl == "" {
		return zerolog.InfoLevel
	}
	level, err := zerolog.ParseLevel(lvl)
	if err != nil {
		logger.Warn().Str("value", lvl).Msg("invalid LOG_LEVEL, defaulting to info")
		return zerolog.InfoLevel
	}
	return level
}
