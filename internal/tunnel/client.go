package tunnel

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
)

// Client connects to the relay's tunnel facility, registers a session, and
// serves incoming HTTP requests by forwarding them to a local handler.
type Client struct {
	relayAddr   string
	token       string
	description string
	handler     http.Handler
	ctrl        net.Conn
	ctrlBr      *bufio.Reader
}

func NewClient(relayAddr, token, description string, handler http.Handler) *Client {
	return &Client{
		relayAddr:   relayAddr,
		token:       token,
		description: description,
		handler:     handler,
	}
}

// SetHandler sets the HTTP handler used to serve tunnel requests. Call before
// Open if you need to wire the handler after creating the client.
func (c *Client) SetHandler(h http.Handler) {
	c.handler = h
}

// Open dials the relay, registers the tunnel, and returns the public upload URL.
// Call Serve in a goroutine afterwards to process incoming requests.
func (c *Client) Open() (string, error) {
	ctrl, err := net.DialTimeout("tcp", c.relayAddr, 30*time.Second)
	if err != nil {
		return "", fmt.Errorf("connect to relay: %w", err)
	}

	if c.description != "" {
		fmt.Fprintf(ctrl, "tunnel %s %s\n", c.token, c.description)
	} else {
		fmt.Fprintf(ctrl, "tunnel %s\n", c.token)
	}

	ctrl.SetDeadline(time.Now().Add(15 * time.Second))
	br := bufio.NewReader(ctrl)
	line, err := br.ReadString('\n')
	ctrl.SetDeadline(time.Time{})
	if err != nil {
		ctrl.Close()
		return "", fmt.Errorf("relay handshake: %w", err)
	}
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "ok ") {
		ctrl.Close()
		return "", fmt.Errorf("relay error: %s", line)
	}

	c.ctrl = ctrl
	c.ctrlBr = br
	return strings.TrimPrefix(line, "ok "), nil
}

// NotifyUploaded sends the relay a file-count update over the control connection.
// Called by the upload handler after files are saved.
func (c *Client) NotifyUploaded(count int) {
	if c.ctrl != nil && count > 0 {
		fmt.Fprintf(c.ctrl, "uploaded %d\n", count)
	}
}

// Serve processes incoming tunnel requests until ctx is cancelled or the relay
// disconnects. It should be called in a goroutine after Open.
func (c *Client) Serve(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		line, err := c.ctrlBr.ReadString('\n')
		if err != nil {
			return
		}
		if strings.TrimSpace(line) == "request" {
			go c.serveOne(ctx)
		}
	}
}

// Close shuts down the control connection, which causes Serve to return.
func (c *Client) Close() {
	if c.ctrl != nil {
		c.ctrl.Close()
	}
}

// serveOne opens a data connection to the relay and handles one HTTP
// request/response cycle.
func (c *Client) serveOne(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	dataConn, err := net.DialTimeout("tcp", c.relayAddr, 30*time.Second)
	if err != nil {
		return
	}
	defer dataConn.Close()

	fmt.Fprintf(dataConn, "tunnel-data %s\n", c.token)

	dataConn.SetDeadline(time.Now().Add(10 * time.Second))
	br := bufio.NewReader(dataConn)
	resp, err := br.ReadString('\n')
	dataConn.SetDeadline(time.Time{})
	if err != nil || strings.TrimSpace(resp) != "ok" {
		return
	}

	// Parse the HTTP request forwarded by the relay.
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}

	// Serve the request using our handler and capture the response.
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)

	// Write the HTTP response back through the data connection to the relay.
	rec.Result().Write(dataConn) //nolint:errcheck
}
