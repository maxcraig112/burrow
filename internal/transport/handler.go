package transport

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/maxcraig112/burrow/internal/exchange"
	"github.com/rs/zerolog"
)

// Handler holds shared state for the exchange HTTP handlers.
type Handler struct {
	ex     *exchange.Exchange
	ttl    time.Duration
	logger zerolog.Logger
}

// NewHandler creates a Handler backed by ex. ttl must match the TTL used when
// creating ex so that the WebSocket timeout aligns with cache expiry.
func NewHandler(ex *exchange.Exchange, ttl time.Duration, logger zerolog.Logger) *Handler {
	return &Handler{ex: ex, ttl: ttl, logger: logger}
}

// errorResponse is the JSON shape returned for all error responses.
type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeError writes a JSON error response. If err is an *ExchangeError its
// Code and Message are used; otherwise a generic internal error is returned.
func writeError(w http.ResponseWriter, status int, err error) {
	var xErr *exchange.ExchangeError
	resp := errorResponse{Code: "INTERNAL_ERROR", Message: "an unexpected error occurred"}
	if errors.As(err, &xErr) {
		resp.Code = string(xErr.Code)
		resp.Message = xErr.Message
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}
