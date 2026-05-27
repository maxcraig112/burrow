package relay

import (
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// Relay pairs two TCP connections that present the same token and splices
// them together. It never inspects the data flowing between them.
type Relay struct {
	mu      sync.Mutex
	pending map[string]*slot
	timeout time.Duration
	logger  zerolog.Logger
	hub     *TunnelHub
}

type slot struct {
	conn net.Conn
	side string
	pair chan net.Conn // second connection sends itself here
}

func New(timeout time.Duration, logger zerolog.Logger, hub *TunnelHub) *Relay {
	return &Relay{
		pending: make(map[string]*slot),
		timeout: timeout,
		logger:  logger,
		hub:     hub,
	}
}

// Handle manages a single incoming TCP connection. Call in a goroutine.
func (r *Relay) Handle(conn net.Conn) {
	conn.SetDeadline(time.Now().Add(30 * time.Second))
	line, err := readLine(conn)
	if err != nil {
		conn.Close()
		return
	}
	conn.SetDeadline(time.Time{})

	switch {
	case strings.HasPrefix(line, "please relay "):
		r.handleRelay(conn, line)
	case strings.HasPrefix(line, "tunnel-data "):
		token := strings.TrimPrefix(line, "tunnel-data ")
		if r.hub != nil {
			r.hub.acceptData(conn, token)
		} else {
			fmt.Fprintf(conn, "error: tunnels not enabled\n")
			conn.Close()
		}
	case strings.HasPrefix(line, "tunnel "):
		token := strings.TrimPrefix(line, "tunnel ")
		if r.hub != nil {
			r.hub.register(conn, token)
		} else {
			fmt.Fprintf(conn, "error: tunnels not enabled\n")
			conn.Close()
		}
	default:
		fmt.Fprintf(conn, "error: unknown command\n")
		conn.Close()
	}
}

func (r *Relay) handleRelay(conn net.Conn, line string) {
	token, side, err := parseRelayHandshake(line)
	if err != nil {
		fmt.Fprintf(conn, "error: invalid handshake\n")
		conn.Close()
		return
	}

	logToken := abbrev(token)
	r.logger.Debug().Str("token", logToken).Str("side", side).Msg("relay connection received")

	r.mu.Lock()
	existing, ok := r.pending[token]
	if !ok {
		s := &slot{conn: conn, side: side, pair: make(chan net.Conn, 1)}
		r.pending[token] = s
		r.mu.Unlock()

		select {
		case peer := <-s.pair:
			r.splice(conn, peer, logToken)
		case <-time.After(r.timeout):
			r.mu.Lock()
			delete(r.pending, token)
			r.mu.Unlock()
			r.logger.Warn().Str("token", logToken).Msg("relay slot timed out with no peer")
			fmt.Fprintf(conn, "error: timed out waiting for peer\n")
			conn.Close()
		}
		return
	}

	delete(r.pending, token)
	r.mu.Unlock()

	existing.pair <- conn
	fmt.Fprintf(conn, "ok\n")
}

func (r *Relay) splice(a, b net.Conn, logToken string) {
	fmt.Fprintf(a, "ok\n")
	r.logger.Info().Str("token", logToken).Msg("relay pair connected")

	done := make(chan int64, 2)
	pipe := func(dst, src net.Conn) {
		n, _ := io.Copy(dst, src)
		dst.Close()
		src.Close()
		done <- n
	}
	go pipe(a, b)
	go pipe(b, a)

	ab := <-done
	ba := <-done
	r.logger.Info().Str("token", logToken).
		Int64("a_to_b_bytes", ab).
		Int64("b_to_a_bytes", ba).
		Msg("relay pair disconnected")
}

func parseRelayHandshake(line string) (token, side string, err error) {
	var p0, p1, p2, p3, p4 string
	n, _ := fmt.Sscanf(line, "%s %s %s %s %s", &p0, &p1, &p2, &p3, &p4)
	if n != 5 || p0 != "please" || p1 != "relay" || p3 != "for" {
		return "", "", fmt.Errorf("unexpected handshake: %q", line)
	}
	return p2, p4, nil
}

func readLine(conn net.Conn) (string, error) {
	var line []byte
	buf := make([]byte, 1)
	for {
		_, err := conn.Read(buf)
		if err != nil {
			return "", err
		}
		if buf[0] == '\n' {
			return string(line), nil
		}
		line = append(line, buf[0])
	}
}
