package relay

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

//go:embed active.html
var activeHTML string

// TunnelHub manages web-upload tunnels. A receiver registers a persistent
// control connection; the hub's HTTP server proxies browser requests through
// that connection to the receiver.
type TunnelHub struct {
	mu      sync.Mutex
	entries map[string]*tunnelEntry
	baseURL string
	logger  zerolog.Logger
}

type tunnelEntry struct {
	ctrl        net.Conn
	data        chan net.Conn
	description string
	startedAt   time.Time
	receiverIP  string

	mu             sync.Mutex
	filesRecv      int
	lastUploaderIP string
}

// SessionInfo is the JSON representation of an active tunnel session.
type SessionInfo struct {
	Nameplate      string    `json:"nameplate"`
	Description    string    `json:"description,omitempty"`
	URL            string    `json:"url"`
	StartedAt      time.Time `json:"started_at"`
	FilesReceived  int       `json:"files_received"`
	ReceiverIP     string    `json:"receiver_ip"`
	LastUploaderIP string    `json:"last_uploader_ip,omitempty"`
}

func NewTunnelHub(baseURL string, logger zerolog.Logger) *TunnelHub {
	return &TunnelHub{
		entries: make(map[string]*tunnelEntry),
		baseURL: baseURL,
		logger:  logger,
	}
}

// register handles a "tunnel TOKEN [DESCRIPTION]" control connection.
// Blocks until the receiver disconnects.
func (h *TunnelHub) register(ctrl net.Conn, token, description string) {
	ip, _, _ := net.SplitHostPort(ctrl.RemoteAddr().String())
	entry := &tunnelEntry{
		ctrl:        ctrl,
		data:        make(chan net.Conn, 4),
		description: description,
		startedAt:   time.Now().UTC(),
		receiverIP:  ip,
	}
	h.mu.Lock()
	h.entries[token] = entry
	h.mu.Unlock()

	url := h.baseURL + "/t/" + token + "/"
	fmt.Fprintf(ctrl, "ok %s\n", url)
	h.logger.Info().Str("token", abbrev(token)).Str("url", url).Msg("web tunnel registered")

	// Read upload-count notifications from the receiver ("uploaded N\n").
	// ServeHTTP writes "request\n" concurrently — net.Conn is safe for that.
	br := bufio.NewReader(ctrl)
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "uploaded ") {
			n, err := strconv.Atoi(strings.TrimPrefix(line, "uploaded "))
			if err == nil && n > 0 {
				entry.mu.Lock()
				entry.filesRecv += n
				entry.mu.Unlock()
			}
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

// ServeHTTP proxies an incoming browser request through the tunnel to the
// receiver. Path format: /t/TOKEN/rest
func (h *TunnelHub) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.URL.Path {
	case "/active":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, activeHTML)
		return
	case "/api/sessions":
		h.serveSessionsAPI(w)
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

	// Record the uploader IP on upload requests before hijacking.
	if req.Method == http.MethodPost && subpath == "/upload" {
		ip, _, _ := net.SplitHostPort(req.RemoteAddr)
		entry.mu.Lock()
		entry.lastUploaderIP = ip
		entry.mu.Unlock()
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

func (h *TunnelHub) serveSessionsAPI(w http.ResponseWriter) {
	h.mu.Lock()
	sessions := make([]SessionInfo, 0, len(h.entries))
	for token, e := range h.entries {
		e.mu.Lock()
		sessions = append(sessions, SessionInfo{
			Nameplate:      token,
			Description:    e.description,
			URL:            h.baseURL + "/t/" + token + "/",
			StartedAt:      e.startedAt,
			FilesReceived:  e.filesRecv,
			ReceiverIP:     e.receiverIP,
			LastUploaderIP: e.lastUploaderIP,
		})
		e.mu.Unlock()
	}
	h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(map[string]any{"sessions": sessions})
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
