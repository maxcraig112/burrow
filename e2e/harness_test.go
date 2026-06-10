package e2e

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/maxcraig112/burrow/internal/client"
	"github.com/maxcraig112/burrow/internal/exchange"
	"github.com/maxcraig112/burrow/internal/relay"
	"github.com/maxcraig112/burrow/internal/transport"
	"github.com/rs/zerolog"
)

const testTTL = 30 * time.Second

// harness holds running in-process exchange and relay servers wired to the
// client package globals. Globals are restored via t.Cleanup.
type harness struct {
	t           testing.TB
	RelayAddr   string
	ExchangeURL string
}

// newHarness starts an exchange HTTP server and a relay TCP listener.
// nameplate is fixed on the exchange; pass "" to generate randomly each call.
func newHarness(t testing.TB, nameplate string) *harness {
	t.Helper()
	logger := zerolog.Nop()

	ex := exchange.New(testTTL, logger, exchange.Flags{FixedNameplate: nameplate})
	h := transport.NewHandler(ex, testTTL, logger)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /send", h.Send)
	mux.HandleFunc("POST /receive", h.Receive)
	srv := httptest.NewServer(mux)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("relay listen: %v", err)
	}
	rl := relay.New(2*time.Minute, logger, nil)
	go acceptLoop(ln, rl)

	prevEx := client.ExchangeAddr
	prevRl := client.RelayAddr
	client.ExchangeAddr = srv.Listener.Addr().String()
	client.RelayAddr = ln.Addr().String()

	t.Cleanup(func() {
		srv.Close()
		ln.Close()
		client.ExchangeAddr = prevEx
		client.RelayAddr = prevRl
	})

	return &harness{
		t:           t,
		RelayAddr:   ln.Addr().String(),
		ExchangeURL: "http://" + srv.Listener.Addr().String(),
	}
}

// newHarnessWithHub starts a harness whose relay includes a TunnelHub, also
// returning an HTTP server that serves the hub so tunnel URLs resolve.
func newHarnessWithHub(t testing.TB) (*harness, *relay.TunnelHub, *httptest.Server) {
	t.Helper()
	logger := zerolog.Nop()

	ex := exchange.New(testTTL, logger, exchange.Flags{})
	h := transport.NewHandler(ex, testTTL, logger)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /send", h.Send)
	mux.HandleFunc("POST /receive", h.Receive)
	exchSrv := httptest.NewServer(mux)

	hubLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("hub listen: %v", err)
	}
	baseURL := "http://" + hubLn.Addr().String()
	hub := relay.NewTunnelHub(baseURL, t.TempDir(), logger)
	hubSrv := &http.Server{Handler: hub}
	go hubSrv.Serve(hubLn) //nolint:errcheck

	relayLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("relay listen: %v", err)
	}
	rl := relay.New(2*time.Minute, logger, hub)
	go acceptLoop(relayLn, rl)

	prevEx := client.ExchangeAddr
	prevRl := client.RelayAddr
	client.ExchangeAddr = exchSrv.Listener.Addr().String()
	client.RelayAddr = relayLn.Addr().String()

	fakeHubSrv := httptest.NewUnstartedServer(hub)
	fakeHubSrv.Listener = hubLn

	t.Cleanup(func() {
		exchSrv.Close()
		hubSrv.Close()  //nolint:errcheck
		relayLn.Close()
		client.ExchangeAddr = prevEx
		client.RelayAddr = prevRl
	})

	return &harness{
		t:           t,
		RelayAddr:   relayLn.Addr().String(),
		ExchangeURL: "http://" + exchSrv.Listener.Addr().String(),
	}, hub, fakeHubSrv
}

func acceptLoop(ln net.Listener, rl *relay.Relay) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go rl.Handle(conn)
	}
}

// ── File helpers ──────────────────────────────────────────────────────────────

// fileSpec describes one file in a transfer test.
type fileSpec struct {
	path    string // relative path, forward slashes, e.g. "sub/file.txt"
	content []byte
}

// randContent returns n pseudo-random bytes suitable for transfer payloads.
func randContent(n int) []byte {
	if n == 0 {
		return []byte{}
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("rand.Read: %v", err))
	}
	return b
}

// makeSourceFile writes content to a temp file and returns its path.
func makeSourceFile(t testing.TB, name string, content []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, content, 0644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	return p
}

// makeSourceDir creates a temp directory tree populated with the given files.
// The returned path is the root of the tree (named after dirName).
func makeSourceDir(t testing.TB, dirName string, files []fileSpec) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), dirName)
	for _, f := range files {
		full := filepath.Join(root, filepath.FromSlash(f.path))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, f.content, 0644); err != nil {
			t.Fatalf("write %s: %v", f.path, err)
		}
	}
	return root
}

// assertFileEqual fatals if path does not exist or its content differs from want.
func assertFileEqual(t testing.TB, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("file %s: content mismatch (got %d bytes, want %d bytes)", path, len(got), len(want))
	}
}

// assertDirReceived verifies destDir/dirName contains exactly the files in specs.
func assertDirReceived(t testing.TB, destDir, dirName string, files []fileSpec) {
	t.Helper()
	for _, f := range files {
		full := filepath.Join(destDir, dirName, filepath.FromSlash(f.path))
		assertFileEqual(t, full, f.content)
	}
}

// ── Transfer orchestration ────────────────────────────────────────────────────

// doSendReceiveFile runs a complete single-file send+receive cycle.
// It creates a temp dest dir and returns the path of the received file.
func doSendReceiveFile(t testing.TB, srcPath string, content []byte) string {
	t.Helper()
	destDir := t.TempDir()

	sess, err := client.StartSend()
	if err != nil {
		t.Fatalf("StartSend: %v", err)
	}

	sendErr := make(chan error, 1)
	go func() {
		keys, err := sess.WaitForReceiver()
		if err != nil {
			sendErr <- fmt.Errorf("WaitForReceiver: %w", err)
			return
		}
		sendErr <- client.SendFile(keys, srcPath, nil)
	}()

	recvKeys, err := client.Receive(sess.Nameplate)
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	transfer, err := client.ReceiveTransfer(recvKeys)
	if err != nil {
		t.Fatalf("ReceiveTransfer: %v", err)
	}
	path, err := transfer.Save(destDir, nil, nil)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := <-sendErr; err != nil {
		t.Fatalf("sender: %v", err)
	}

	assertFileEqual(t, path, content)
	return path
}

// doSendReceiveDir runs a complete directory send+receive cycle.
// It returns the path of the received directory root.
func doSendReceiveDir(t testing.TB, srcDir string, files []fileSpec) string {
	t.Helper()
	destDir := t.TempDir()
	dirName := filepath.Base(srcDir)

	sess, err := client.StartSend()
	if err != nil {
		t.Fatalf("StartSend: %v", err)
	}

	sendErr := make(chan error, 1)
	go func() {
		keys, err := sess.WaitForReceiver()
		if err != nil {
			sendErr <- fmt.Errorf("WaitForReceiver: %w", err)
			return
		}
		sendErr <- client.SendDir(keys, srcDir, nil, nil)
	}()

	recvKeys, err := client.Receive(sess.Nameplate)
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	transfer, err := client.ReceiveTransfer(recvKeys)
	if err != nil {
		t.Fatalf("ReceiveTransfer: %v", err)
	}
	path, err := transfer.Save(destDir, nil, nil)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := <-sendErr; err != nil {
		t.Fatalf("sender: %v", err)
	}

	assertDirReceived(t, destDir, dirName, files)
	return path
}

// readFileBytes reads and returns all bytes from path. Safe to call from any
// goroutine (does not call t.Fatal).
func readFileBytes(path string) ([]byte, error) {
	return os.ReadFile(path)
}
