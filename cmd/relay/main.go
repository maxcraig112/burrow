package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maxcraig112/burrow/internal/logging"
	"github.com/maxcraig112/burrow/internal/relay"
	"github.com/maxcraig112/env"
)

func main() {
	logger := logging.New()

	tunnelPublicURL := env.Get("TUNNEL_PUBLIC_URL", "http://localhost:8082")
	tunnelBind := env.Get("TUNNEL_BIND", ":8082")
	uploadDir := env.Get("UPLOAD_DIR")
	if uploadDir == "" {
		if cwd, err := os.Getwd(); err == nil {
			uploadDir = cwd
		}
	}

	hub := relay.NewTunnelHub(tunnelPublicURL, uploadDir, logger)
	r := relay.New(2*time.Minute, logger, hub)

	ln, err := net.Listen("tcp", ":9090")
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to listen")
	}
	logger.Info().Str("addr", ":9090").Msg("relay listening")

	httpSrv := &http.Server{
		Addr:         tunnelBind,
		Handler:      hub,
		ReadTimeout:  5 * time.Minute,
		WriteTimeout: 10 * time.Minute,
	}
	go func() {
		logger.Info().Str("addr", tunnelBind).Msg("tunnel HTTP listening")
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error().Err(err).Msg("tunnel HTTP server error")
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		ln.Close()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpSrv.Shutdown(shutCtx)
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
