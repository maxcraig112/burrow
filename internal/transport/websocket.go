package transport

import (
	"net/http"

	"github.com/gorilla/websocket"
)

// msgType identifies the intent of a server-to-client WebSocket message.
type msgType string

const (
	msgNameplate msgType = "nameplate" // initial message: nameplate + prompt for PAKE
	msgReceived  msgType = "received"  // receiver claimed; carries receiver's PAKE pubkey
	msgExpired   msgType = "expired"   // TTL elapsed with no receiver
	msgError     msgType = "error"     // protocol error (e.g. bad PAKE message)
)

// serverMsg is the JSON envelope sent to the WebSocket client.
type serverMsg struct {
	Type      msgType `json:"type"`
	Nameplate string  `json:"nameplate,omitempty"`
	PAKE      string  `json:"pake,omitempty"` // base64-encoded peer public key
	Message   string  `json:"message,omitempty"`
}

// clientMsg is the JSON envelope expected from the WebSocket client.
// After receiving the nameplate, the client must send exactly one message of
// type "pake" carrying their base64-encoded X25519 public key in Body.
type clientMsg struct {
	Type string `json:"type"` // "pake"
	Body string `json:"body"` // base64-encoded public key
}

var upgrader = websocket.Upgrader{
	// Allow all origins for development. Restrict CheckOrigin in production.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// writeJSON sends msg as a JSON text frame. The connection is not closed on error.
func writeJSON(conn *websocket.Conn, msg serverMsg) error {
	return conn.WriteJSON(msg)
}

// closeConn sends a final msg then performs a clean WebSocket close handshake.
func closeConn(conn *websocket.Conn, msg serverMsg) {
	_ = writeJSON(conn, msg)
	_ = conn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
	)
	conn.Close()
}
