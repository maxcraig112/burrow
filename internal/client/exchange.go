package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/maxcraig112/burrow/internal/pake"
)

// ExchangeAddr and RelayAddr must be set before calling any client functions.
// Use `burrow config` to save them, or set EXCHANGE_ADDR / RELAY_ADDR env vars.
var (
	ExchangeAddr = ""
	RelayAddr    = ""
)

type wsMsg struct {
	Type      string `json:"type"`
	Nameplate string `json:"nameplate,omitempty"`
	PAKE      string `json:"pake,omitempty"`
	Message   string `json:"message,omitempty"`
}

type pakem struct {
	Type string `json:"type"`
	Body string `json:"body"`
}

// SendSession represents an in-progress send exchange. Call WaitForReceiver
// to block until a receiver claims the nameplate.
type SendSession struct {
	Nameplate string
	kp        *pake.KeyPair
	conn      *websocket.Conn
}

// exchangeHTTPURL builds an HTTP(S) URL for the given path.
// ExchangeAddr may be "host:port" (plain) or a full URL like
// "https://service.run.app" (used when deployed behind TLS).
func exchangeHTTPURL(path string) string {
	if strings.Contains(ExchangeAddr, "://") {
		return ExchangeAddr + path
	}
	return "http://" + ExchangeAddr + path
}

// exchangeWSURL builds a WS(S) URL, converting https→wss automatically.
func exchangeWSURL(path string) string {
	if strings.Contains(ExchangeAddr, "://") {
		addr := strings.Replace(ExchangeAddr, "https://", "wss://", 1)
		addr = strings.Replace(addr, "http://", "ws://", 1)
		return addr + path
	}
	return "ws://" + ExchangeAddr + path
}

// StartSend connects to the exchange server, obtains a nameplate, and sends
// the sender's public key. It returns immediately with the nameplate so the
// caller can display it before blocking on WaitForReceiver.
func StartSend() (*SendSession, error) {
	conn, _, err := websocket.DefaultDialer.Dial(exchangeWSURL("/send"), nil)
	if err != nil {
		return nil, fmt.Errorf("connect to exchange: %w", err)
	}

	var msg wsMsg
	if err := conn.ReadJSON(&msg); err != nil || msg.Type != "nameplate" {
		conn.Close()
		return nil, fmt.Errorf("expected nameplate from exchange server")
	}

	kp, err := pake.GenerateKeyPair()
	if err != nil {
		conn.Close()
		return nil, err
	}

	if err := conn.WriteJSON(pakem{Type: "pake", Body: kp.PublicKeyBase64()}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("send PAKE: %w", err)
	}

	return &SendSession{Nameplate: msg.Nameplate, kp: kp, conn: conn}, nil
}

// WaitForReceiver blocks until a receiver claims the nameplate or the session
// expires. On success it returns the derived keys ready for relay and file
// transfer.
func (s *SendSession) WaitForReceiver() (*pake.DerivedKeys, error) {
	defer s.conn.Close()

	var msg wsMsg
	if err := s.conn.ReadJSON(&msg); err != nil {
		return nil, fmt.Errorf("read exchange response: %w", err)
	}
	switch msg.Type {
	case "expired":
		return nil, fmt.Errorf("timed out: no receiver connected within the session window")
	case "error":
		return nil, fmt.Errorf("exchange error: %s", msg.Message)
	case "received":
	default:
		return nil, fmt.Errorf("unexpected message type %q from exchange server", msg.Type)
	}

	return pake.DeriveKeys(s.kp.Private, msg.PAKE, s.Nameplate)
}

// Receive connects to the exchange server as a receiver, sends the receiver's
// public key, and returns the derived keys on success.
func Receive(nameplate string) (*pake.DerivedKeys, error) {
	kp, err := pake.GenerateKeyPair()
	if err != nil {
		return nil, err
	}

	body, _ := json.Marshal(map[string]string{
		"nameplate": nameplate,
		"pake":      kp.PublicKeyBase64(),
	})

	resp, err := http.Post(
		exchangeHTTPURL("/receive"),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to exchange: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		json.NewDecoder(resp.Body).Decode(&errBody)
		return nil, fmt.Errorf("%s: %s", errBody.Code, errBody.Message)
	}

	var result struct {
		PAKE string `json:"pake"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode exchange response: %w", err)
	}

	return pake.DeriveKeys(kp.Private, result.PAKE, nameplate)
}
