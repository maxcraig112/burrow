package relay

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/maxcraig112/burrow/internal/nameplate"
	"github.com/maxcraig112/burrow/internal/webupload"
	"github.com/rs/zerolog"
	"rsc.io/qr"
)

//go:embed active.html
var activeHTML string

//go:embed home.html
var homeHTML string

//go:embed create.html
var createHTML string

//go:embed static
var staticFS embed.FS

// TunnelHub manages web-upload tunnels and browser-created local sessions.
type TunnelHub struct {
	mu      sync.Mutex
	entries map[string]*tunnelEntry
	baseURL string
	logger  zerolog.Logger

	localMu   sync.Mutex
	locals    map[string]*localSession
	uploadDir string // default directory for browser-created sessions
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

// localSession is a session created via the browser UI and served directly
// by the relay — no tunnel or external receive-web process needed.
type localSession struct {
	description string
	directory   string
	startedAt   time.Time
	handler     http.Handler

	mu             sync.Mutex
	filesRecv      int
	lastUploaderIP string
}

// SessionInfo is the JSON shape returned by /api/sessions.
type SessionInfo struct {
	Nameplate      string    `json:"nameplate"`
	Description    string    `json:"description,omitempty"`
	URL            string    `json:"url"`
	StartedAt      time.Time `json:"started_at"`
	FilesReceived  int       `json:"files_received"`
	ReceiverIP     string    `json:"receiver_ip,omitempty"`
	LastUploaderIP string    `json:"last_uploader_ip,omitempty"`
	Source         string    `json:"source"`              // "tunnel" or "local"
	Directory      string    `json:"directory,omitempty"` // local sessions only
}

func NewTunnelHub(baseURL, uploadDir string, logger zerolog.Logger) *TunnelHub {
	return &TunnelHub{
		entries:   make(map[string]*tunnelEntry),
		locals:    make(map[string]*localSession),
		baseURL:   baseURL,
		uploadDir: uploadDir,
		logger:    logger,
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

// ServeHTTP routes browser requests to the appropriate handler.
func (h *TunnelHub) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Static pages
	switch req.URL.Path {
	case "/":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, homeHTML)
		return
	case "/active":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, activeHTML)
		return
	case "/create":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, createHTML)
		return
	}

	// Static assets
	if strings.HasPrefix(req.URL.Path, "/static/") {
		http.FileServer(http.FS(staticFS)).ServeHTTP(w, req)
		return
	}

	// API routes
	switch req.URL.Path {
	case "/api/sessions":
		h.serveSessionsAPI(w)
		return
	case "/api/config":
		h.serveConfig(w)
		return
	case "/api/create-session":
		h.handleCreateSession(w, req)
		return
	case "/api/browse":
		h.serveBrowse(w, req)
		return
	case "/api/qr":
		h.serveQR(w, req)
		return
	}
	if strings.HasPrefix(req.URL.Path, "/api/close-session/") {
		h.handleCloseSession(w, req)
		return
	}

	// Session routes: /t/TOKEN/subpath
	token, subpath, ok := parseTunnelPath(req.URL.Path)
	if !ok {
		http.NotFound(w, req)
		return
	}

	// Local session — serve directly without tunneling.
	h.localMu.Lock()
	local, isLocal := h.locals[token]
	h.localMu.Unlock()
	if isLocal {
		if req.Method == http.MethodPost && subpath == "/upload" {
			ip, _, _ := net.SplitHostPort(req.RemoteAddr)
			local.mu.Lock()
			local.lastUploaderIP = ip
			local.mu.Unlock()
		}
		req.URL.Path = "/" + token + subpath
		req.RequestURI = req.URL.RequestURI()
		local.handler.ServeHTTP(w, req)
		return
	}

	// Tunnel session — proxy through the receiver's control connection.
	h.mu.Lock()
	entry, ok := h.entries[token]
	h.mu.Unlock()
	if !ok {
		http.NotFound(w, req)
		return
	}

	if req.Method == http.MethodPost && subpath == "/upload" {
		ip, _, _ := net.SplitHostPort(req.RemoteAddr)
		entry.mu.Lock()
		entry.lastUploaderIP = ip
		entry.mu.Unlock()
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
		brw.Reader.Read(buf) //nolint:errcheck
		dataConn.Write(buf)  //nolint:errcheck
	}
	io.Copy(browserConn, dataConn)
}

// ── API handlers ─────────────────────────────────────────────────────────────

func (h *TunnelHub) serveConfig(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"upload_dir": h.uploadDir})
}

func (h *TunnelHub) serveSessionsAPI(w http.ResponseWriter) {
	var sessions []SessionInfo

	h.mu.Lock()
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
			Source:         "tunnel",
		})
		e.mu.Unlock()
	}
	h.mu.Unlock()

	h.localMu.Lock()
	for token, ls := range h.locals {
		ls.mu.Lock()
		sessions = append(sessions, SessionInfo{
			Nameplate:      token,
			Description:    ls.description,
			URL:            h.baseURL + "/t/" + token + "/",
			StartedAt:      ls.startedAt,
			FilesReceived:  ls.filesRecv,
			LastUploaderIP: ls.lastUploaderIP,
			Source:         "local",
			Directory:      ls.directory,
		})
		ls.mu.Unlock()
	}
	h.localMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(map[string]any{"sessions": sessions})
}

func (h *TunnelHub) handleCreateSession(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Directory   string `json:"directory"`
		Description string `json:"description"`
	}
	json.NewDecoder(req.Body).Decode(&body) //nolint:errcheck
	if body.Directory == "" {
		body.Directory = h.uploadDir
	}

	if err := os.MkdirAll(body.Directory, 0755); err != nil {
		http.Error(w, `{"error":"could not create directory: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	np := h.uniqueNameplate()
	ls := &localSession{
		description: body.Description,
		directory:   body.Directory,
		startedAt:   time.Now().UTC(),
	}
	ls.handler = webupload.NewHandler(body.Directory, np, body.Description, func(n int) {
		ls.mu.Lock()
		ls.filesRecv += n
		ls.mu.Unlock()
	})

	h.localMu.Lock()
	h.locals[np] = ls
	h.localMu.Unlock()

	sessionURL := h.baseURL + "/t/" + np + "/"
	h.logger.Info().Str("token", abbrev(np)).Str("dir", body.Directory).Msg("local session created")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"nameplate": np,
		"url":       sessionURL,
	})
}

func (h *TunnelHub) handleCloseSession(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	np := strings.TrimPrefix(req.URL.Path, "/api/close-session/")
	if np == "" {
		http.Error(w, "missing nameplate", http.StatusBadRequest)
		return
	}

	// Try tunnel session first.
	h.mu.Lock()
	entry, ok := h.entries[np]
	if ok {
		delete(h.entries, np)
	}
	h.mu.Unlock()
	if ok {
		entry.ctrl.Close()
		h.logger.Info().Str("token", abbrev(np)).Msg("session closed via browser")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "closed"})
		return
	}

	// Try local session.
	h.localMu.Lock()
	_, ok = h.locals[np]
	if ok {
		delete(h.locals, np)
	}
	h.localMu.Unlock()
	if ok {
		h.logger.Info().Str("token", abbrev(np)).Msg("local session closed via browser")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "closed"})
		return
	}

	http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
}

func (h *TunnelHub) serveBrowse(w http.ResponseWriter, req *http.Request) {
	path := req.URL.Query().Get("path")
	if path == "" {
		path = h.uploadDir
	}
	path = filepath.Clean(path)

	entries, err := os.ReadDir(path)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}

	parent := filepath.Dir(path)
	if parent == path {
		parent = ""
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"path":   path,
		"parent": parent,
		"dirs":   dirs,
	})
}

func (h *TunnelHub) serveQR(w http.ResponseWriter, req *http.Request) {
	url := req.URL.Query().Get("url")
	if url == "" {
		http.Error(w, "missing url", http.StatusBadRequest)
		return
	}
	code, err := qr.Encode(url, qr.M)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(code.PNG()) //nolint:errcheck
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// uniqueNameplate generates a nameplate not currently used by any session.
func (h *TunnelHub) uniqueNameplate() string {
	for {
		np := nameplate.Generate()
		h.mu.Lock()
		_, tunnel := h.entries[np]
		h.mu.Unlock()
		if tunnel {
			continue
		}
		h.localMu.Lock()
		_, local := h.locals[np]
		h.localMu.Unlock()
		if !local {
			return np
		}
	}
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
