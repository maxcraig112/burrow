// Package profiling optionally exposes net/http/pprof on a dedicated address.
package profiling

import (
	"net/http"
	_ "net/http/pprof" // registers /debug/pprof/* on http.DefaultServeMux

	"github.com/rs/zerolog"
)

// Serve starts a net/http/pprof server on addr in a background goroutine.
func Serve(addr string, logger zerolog.Logger) {
	if addr == "" {
		return
	}
	go func() {
		logger.Info().Str("addr", addr).Msg("pprof listening on /debug/pprof/")
		if err := http.ListenAndServe(addr, nil); err != nil {
			logger.Error().Err(err).Msg("pprof server error")
		}
	}()
}
