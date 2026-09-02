package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maxcraig112/burrow/internal/exchange"
	"github.com/maxcraig112/burrow/internal/logging"
	"github.com/maxcraig112/burrow/internal/transport"
	"github.com/maxcraig112/env"
)

const ttl = 5 * time.Minute

func main() {
	logger := logging.New()

	ex := exchange.New(ttl, logger, exchange.FlagsFromEnv())
	h := transport.NewHandler(ex, ttl, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /send", h.Send)
	mux.HandleFunc("POST /receive", h.Receive)

	port := env.Get("PORT", "8080")
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
