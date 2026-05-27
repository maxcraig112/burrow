package transport

import (
	"encoding/json"
	"errors"
	"net/http"

	"gopherhole/internal/exchange"
)

// receiveRequest is the JSON body expected by the Receive endpoint.
type receiveRequest struct {
	Nameplate string `json:"nameplate"`
	PAKE      string `json:"pake"` // base64-encoded X25519 public key
}

// receiveResponse is the JSON body returned on a successful claim.
// The PAKE field carries the sender's base64-encoded public key so the
// receiver can derive the shared secret via ECDH(receiver_privkey, sender_pubkey).
type receiveResponse struct {
	Status    string `json:"status"`
	Nameplate string `json:"nameplate"`
	PAKE      string `json:"pake"` // sender's base64-encoded public key
}

// Receive claims a pending session by nameplate and completes the PAKE exchange.
//
//	POST /receive
//	Body: {"nameplate":"swift-copper-leaps","pake":"<base64 X25519 pubkey>"}
func (h *Handler) Receive(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug().Str("remote", r.RemoteAddr).Msg("receive request")

	var req receiveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error().Err(err).Msg("failed to decode receive request")
		writeError(w, http.StatusBadRequest, &exchange.ExchangeError{
			Code:    "BAD_REQUEST",
			Message: "request body must be JSON with \"nameplate\" and \"pake\" fields",
		})
		return
	}
	if req.Nameplate == "" {
		writeError(w, http.StatusBadRequest, &exchange.ExchangeError{
			Code:    "BAD_REQUEST",
			Message: "nameplate must not be empty",
		})
		return
	}
	if req.PAKE == "" {
		writeError(w, http.StatusBadRequest, &exchange.ExchangeError{
			Code:    "BAD_REQUEST",
			Message: "pake must not be empty",
		})
		return
	}

	h.logger.Debug().Str("nameplate", req.Nameplate).Msg("attempting to claim nameplate")

	senderPAKE, err := h.ex.Receive(req.Nameplate, req.PAKE)
	if err != nil {
		switch {
		case errors.Is(err, exchange.ErrNotFound):
			writeError(w, http.StatusNotFound, err)
		case errors.Is(err, exchange.ErrAlreadyClaimed):
			writeError(w, http.StatusConflict, err)
		case errors.Is(err, exchange.ErrPAKENotReady):
			writeError(w, http.StatusServiceUnavailable, err)
		case errors.Is(err, exchange.ErrPAKEInvalid):
			writeError(w, http.StatusBadRequest, err)
		default:
			h.logger.Error().Str("nameplate", req.Nameplate).Err(err).Msg("unexpected error during receive")
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	h.logger.Info().Str("nameplate", req.Nameplate).Msg("exchange complete")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(receiveResponse{
		Status:    "confirmed",
		Nameplate: req.Nameplate,
		PAKE:      senderPAKE,
	})
}
