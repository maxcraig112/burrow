package main

import (
	"context"
	"flag"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"gopherhole/internal/relay"
)

func main() {
	envFile := flag.String("env", ".env", "path to .env file")
	flag.Parse()

	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).
		With().Timestamp().Logger()

	if err := godotenv.Load(*envFile); err != nil && !os.IsNotExist(err) {
		logger.Warn().Str("path", *envFile).Err(err).Msg("could not load env file")
	}
	logger = logger.Level(parseLevel(logger))

	r := relay.New(2*time.Minute, logger)

	ln, err := net.Listen("tcp", ":9090")
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to listen")
	}
	logger.Info().Str("addr", ":9090").Msg("relay listening")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				logger.Info().Msg("relay shutting down")
				return
			default:
				logger.Error().Err(err).Msg("accept error")
			}
			continue
		}
		go r.Handle(conn)
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
