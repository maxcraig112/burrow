package relay

import (
	"bufio"
	"embed"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

//go:embed static
var staticFS embed.FS

// TunnelHub proxies browser HTTP requests to a `burrow receive-web` process
// over its control connection to the relay.
type TunnelHub struct {
	mu      sync.Mutex
	entries map[string]*tunnelEntry
	baseURL string
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

// register handles a "tunnel TOKEN" control connection and blocks until the
// receiver disconnects.
func (h *TunnelHub) register(ctrl net.Conn, token string) {
	entry := &tunnelEntry{ctrl: ctrl, data: make(chan net.Conn, 4)}
	h.mu.Lock()
	h.entries[token] = entry
	h.mu.Unlock()

	url := h.baseURL + "/t/" + token + "/"
	fmt.Fprintf(ctrl, "ok %s\n", url)
	h.logger.Info().Str("token", abbrev(token)).Str("url", url).Msg("web tunnel registered")

	// Block until the control connection closes.
	br := bufio.NewReader(ctrl)
	for {
		if _, err := br.ReadString('\n'); err != nil {
			break
		}
	}

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

// ServeHTTP serves the embedded static assets and proxies /t/TOKEN/ requests
// to the matching receive-web process.
func (h *TunnelHub) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if strings.HasPrefix(req.URL.Path, "/static/") {
		http.FileServer(http.FS(staticFS)).ServeHTTP(w, req)
		return
	}

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

	req.URL.Path = "/" + token + subpath
	req.RequestURI = req.URL.RequestURI()
	req.Close = true

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

	if err := req.Write(dataConn); err != nil {
		return
	}
	if n := brw.Reader.Buffered(); n > 0 {
		buf := make([]byte, n)
		brw.Read(buf)       //nolint:errcheck
		dataConn.Write(buf) //nolint:errcheck
	}
	io.Copy(browserConn, dataConn) //nolint:errcheck
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
