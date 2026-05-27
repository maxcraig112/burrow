package relay

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// TunnelHub manages web-upload tunnels. A receiver registers a persistent
// control connection; the hub's HTTP server proxies browser requests through
// that connection to the receiver.
type TunnelHub struct {
	mu      sync.Mutex
	entries map[string]*tunnelEntry
	baseURL string // e.g. "http://relay-host:8082"
	logger  zerolog.Logger
}

type tunnelEntry struct {
	ctrl net.Conn
	data chan net.Conn
}

func NewTunnelHub(baseURL string, logger zerolog.Logger) *TunnelHub {
	return &TunnelHub{
		entries: make(map[string]*tunnelEntry),
		baseURL: baseURL,
		logger:  logger,
	}
}

// register handles a "tunnel TOKEN" control connection. Blocks until the
// receiver disconnects.
func (h *TunnelHub) register(ctrl net.Conn, token string) {
	entry := &tunnelEntry{
		ctrl: ctrl,
		data: make(chan net.Conn, 4),
	}
	h.mu.Lock()
	h.entries[token] = entry
	h.mu.Unlock()

	url := h.baseURL + "/t/" + token + "/"
	fmt.Fprintf(ctrl, "ok %s\n", url)
	h.logger.Info().Str("token", abbrev(token)).Str("url", url).Msg("web tunnel registered")

	// Drain the control connection; it carries no data from the receiver —
	// keeping it open is the liveness signal.
	io.Copy(io.Discard, ctrl)

	h.mu.Lock()
	delete(h.entries, token)
	h.mu.Unlock()
	ctrl.Close()
	h.logger.Info().Str("token", abbrev(token)).Msg("web tunnel closed")
}

// acceptData handles a "tunnel-data TOKEN" connection from the receiver and
// delivers it to the waiting HTTP handler.
func (h *TunnelHub) acceptData(conn net.Conn, token string) {
	h.mu.Lock()
	entry, ok := h.entries[token]
	h.mu.Unlock()

	if !ok {
		fmt.Fprintf(conn, "error: no such tunnel\n")
		conn.Close()
		return
	}
	fmt.Fprintf(conn, "ok\n")

	select {
	case entry.data <- conn:
	case <-time.After(15 * time.Second):
		conn.Close()
	}
}

// ServeHTTP proxies an incoming browser request through the tunnel to the
// receiver. Path format: /t/TOKEN/rest
func (h *TunnelHub) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	token, subpath, ok := parseTunnelPath(req.URL.Path)
	if !ok {
		http.NotFound(w, req)
		return
	}

	h.mu.Lock()
	entry, ok := h.entries[token]
	h.mu.Unlock()
	if !ok {
		http.NotFound(w, req)
		return
	}

	// Tell the receiver a request is incoming.
	if _, err := fmt.Fprintf(entry.ctrl, "request\n"); err != nil {
		http.Error(w, "tunnel unavailable", http.StatusServiceUnavailable)
		return
	}

	var dataConn net.Conn
	select {
	case dataConn = <-entry.data:
	case <-time.After(15 * time.Second):
		http.Error(w, "receiver did not respond", http.StatusGatewayTimeout)
		return
	}
	defer dataConn.Close()

	// Rewrite URL to strip /t prefix before forwarding to the receiver.
	req.URL.Path = "/" + token + subpath
	req.RequestURI = req.URL.RequestURI()
	req.Close = true // one request per data connection

	// Hijack the browser connection so we can copy raw bytes.
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack not supported", http.StatusInternalServerError)
		return
	}
	browserConn, brw, err := hj.Hijack()
	if err != nil {
		return
	}
	defer browserConn.Close()

	// Forward the full HTTP request (headers + body) to the receiver.
	if err := req.Write(dataConn); err != nil {
		return
	}

	// Flush any bytes already buffered by the HTTP server's read buffer.
	if n := brw.Reader.Buffered(); n > 0 {
		buf := make([]byte, n)
		brw.Reader.Read(buf) //nolint:errcheck
		dataConn.Write(buf)  //nolint:errcheck
	}

	// Stream the response from the receiver back to the browser.
	io.Copy(browserConn, dataConn)
}

// parseTunnelPath extracts the token and sub-path from /t/TOKEN/rest.
func parseTunnelPath(path string) (token, subpath string, ok bool) {
	if !strings.HasPrefix(path, "/t/") {
		return "", "", false
	}
	rest := path[3:]
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return rest, "/", true
	}
	return rest[:slash], rest[slash:], true
}

func abbrev(s string) string {
	if len(s) > 8 {
		return s[:8] + "..."
	}
	return s
}
