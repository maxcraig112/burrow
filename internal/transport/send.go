package transport

import (
	"net/http"
	"time"

	"gopherhole/internal/exchange"
)

// Send upgrades the connection to WebSocket, issues a nameplate, exchanges
// PAKE public keys with the receiver, and notifies the sender of the outcome.
//
//	GET /send
//
// Protocol (sender side):
//  1. Connect via WebSocket.
//  2. Receive {"type":"nameplate","nameplate":"<code>"}.
//  3. Send {"type":"pake","body":"<base64 X25519 pubkey>"}.
//  4. Wait.
//  5a. On success: receive {"type":"received","nameplate":"<code>","pake":"<receiver pubkey>"}.
//  5b. On timeout: receive {"type":"expired",...}.
func (h *Handler) Send(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug().Str("remote", r.RemoteAddr).Msg("websocket upgrade requested")

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error().Str("remote", r.RemoteAddr).Err(err).Msg("websocket upgrade failed")
		return
	}
	h.logger.Debug().Str("remote", r.RemoteAddr).Msg("websocket connected")

	sess, err := h.ex.Send()
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to create session")
		closeConn(conn, serverMsg{Type: msgExpired, Message: err.Error()})
		return
	}

	if err := writeJSON(conn, serverMsg{Type: msgNameplate, Nameplate: sess.Nameplate}); err != nil {
		h.logger.Error().Str("nameplate", sess.Nameplate).Err(err).Msg("failed to send nameplate")
		conn.Close()
		return
	}
	h.logger.Info().Str("nameplate", sess.Nameplate).Msg("nameplate issued, awaiting sender PAKE")

	// Read the sender's PAKE message. Give them 30 seconds to respond.
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	var cm clientMsg
	if err := conn.ReadJSON(&cm); err != nil {
		h.logger.Error().Str("nameplate", sess.Nameplate).Err(err).Msg("failed to read PAKE from sender")
		closeConn(conn, serverMsg{Type: msgError, Message: "expected pake message"})
		return
	}
	conn.SetReadDeadline(time.Time{})

	if cm.Type != "pake" || cm.Body == "" {
		h.logger.Error().Str("nameplate", sess.Nameplate).Str("got", cm.Type).Msg("unexpected message type from sender")
		closeConn(conn, serverMsg{Type: msgError, Message: "expected {\"type\":\"pake\",\"body\":\"<base64 pubkey>\"}"})
		return
	}

	if err := h.ex.StoreSenderPAKE(sess.Nameplate, cm.Body); err != nil {
		h.logger.Error().Str("nameplate", sess.Nameplate).Err(err).Msg("failed to store sender PAKE")
		closeConn(conn, serverMsg{Type: msgError, Message: err.Error()})
		return
	}
	h.logger.Info().Str("nameplate", sess.Nameplate).Msg("sender PAKE stored, waiting for receiver")

	select {
	case <-sess.Claimed():
		h.logger.Info().Str("nameplate", sess.Nameplate).Msg("receiver arrived, closing sender with PAKE")
		closeConn(conn, serverMsg{
			Type:      msgReceived,
			Nameplate: sess.Nameplate,
			PAKE:      sess.ReceiverPAKE(),
			Message:   "a receiver has claimed your nameplate",
		})
	case <-time.After(h.ttl):
		h.logger.Info().Str("nameplate", sess.Nameplate).Msg("session expired with no receiver")
		closeConn(conn, serverMsg{
			Type:      msgExpired,
			Nameplate: sess.Nameplate,
			Message:   exchange.ErrExpired.Message,
		})
	}
}
