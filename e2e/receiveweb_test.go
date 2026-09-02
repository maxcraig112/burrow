package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/maxcraig112/burrow/internal/client"
	"github.com/maxcraig112/burrow/internal/nameplate"
	"github.com/maxcraig112/burrow/internal/tunnel"
)

// newTunnelClient builds a tunnel.Client using the relay address currently set
// in the client package globals (configured by newHarnessWithHub).
func newTunnelClient(token string, handler http.Handler) *tunnel.Client {
	return tunnel.NewClient(client.RelayAddr, token, handler)
}

// TestReceiveWebTunnelRegistration verifies that a tunnel client can register
// with the relay and that the hub returns a valid upload URL.
func TestReceiveWebTunnelRegistration(t *testing.T) {
	_, _, hubSrv := newHarnessWithHub(t)

	token := nameplate.Generate()
	tc := newTunnelClient(token, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "ok")
	}))

	uploadURL, err := tc.Open()
	if err != nil {
		t.Fatalf("tunnel Open: %v", err)
	}
	if !strings.Contains(uploadURL, token) {
		t.Fatalf("upload URL %q does not contain token %q", uploadURL, token)
	}
	if !strings.HasPrefix(uploadURL, "http://"+hubSrv.Listener.Addr().String()) {
		t.Fatalf("upload URL %q does not point at hub %s", uploadURL, hubSrv.Listener.Addr())
	}

	tc.Close()
}

// TestReceiveWebTunnelHTTP verifies the full tunnel round-trip: an HTTP client
// sends a request through the TunnelHub, which proxies it to the tunnel
// client's handler and streams the response back.
func TestReceiveWebTunnelHTTP(t *testing.T) {
	_, _, hubSrv := newHarnessWithHub(t)

	token := nameplate.Generate()
	want := "tunnel-ok:" + token

	tc := newTunnelClient(token, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, want)
	}))

	if _, err := tc.Open(); err != nil {
		t.Fatalf("tunnel Open: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); tc.Close() })
	go tc.Serve(ctx)

	time.Sleep(20 * time.Millisecond) // let Serve goroutine reach ReadString

	reqURL := "http://" + hubSrv.Listener.Addr().String() + "/t/" + token + "/"
	resp, err := http.Get(reqURL) //nolint:noctx
	if err != nil {
		t.Fatalf("GET %s: %v", reqURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if got := strings.TrimSpace(string(body)); got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

// TestReceiveWebLongLivedTunnel verifies that a tunnel session survives multiple
// sequential HTTP requests — simulating a real receive-web session that stays
// open while several uploads arrive over time.
func TestReceiveWebLongLivedTunnel(t *testing.T) {
	tests := []struct {
		name        string
		numRequests int
		gapMs       int
	}{
		{"5 requests no gap", 5, 0},
		{"5 requests with gap", 5, 10},
		{"10 requests no gap", 10, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, hubSrv := newHarnessWithHub(t)

			token := nameplate.Generate()
			var count int
			tun := newTunnelClient(token, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				count++
				fmt.Fprintf(w, "req%d", count)
			}))

			if _, err := tun.Open(); err != nil {
				t.Fatalf("tunnel Open: %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(func() { cancel(); tun.Close() })
			go tun.Serve(ctx)

			time.Sleep(20 * time.Millisecond)

			reqURL := "http://" + hubSrv.Listener.Addr().String() + "/t/" + token + "/"

			for i := range tc.numRequests {
				if tc.gapMs > 0 {
					time.Sleep(time.Duration(tc.gapMs) * time.Millisecond)
				}
				resp, err := http.Get(reqURL) //nolint:noctx
				if err != nil {
					t.Fatalf("request %d: %v", i+1, err)
				}
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()

				want := fmt.Sprintf("req%d", i+1)
				if got := strings.TrimSpace(string(body)); got != want {
					t.Errorf("request %d: got %q, want %q", i+1, got, want)
				}
			}
		})
	}
}
