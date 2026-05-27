package relay

import (
	"fmt"
	"io"
	"net"
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
}

type slot struct {
	conn net.Conn
	side string
	pair chan net.Conn // second connection sends itself here
}

func New(timeout time.Duration, logger zerolog.Logger) *Relay {
	return &Relay{
		pending: make(map[string]*slot),
		timeout: timeout,
		logger:  logger,
	}
}

// Handle manages a single incoming TCP connection. Call in a goroutine.
func (r *Relay) Handle(conn net.Conn) {
	conn.SetDeadline(time.Now().Add(30 * time.Second))
	token, side, err := readHandshake(conn)
	if err != nil {
		fmt.Fprintf(conn, "error: invalid handshake\n")
		conn.Close()
		return
	}
	conn.SetDeadline(time.Time{})

	logToken := token
	if len(logToken) > 8 {
		logToken = logToken[:8] + "..."
	}
	r.logger.Debug().Str("token", logToken).Str("side", side).Msg("relay connection received")

	r.mu.Lock()
	existing, ok := r.pending[token]
	if !ok {
		s := &slot{conn: conn, side: side, pair: make(chan net.Conn, 1)}
		r.pending[token] = s
		r.mu.Unlock()

		// First connection: wait for the second.
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

	// Second connection: pair with the first.
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

// readHandshake reads "please relay TOKEN for SIDE\n" byte-by-byte.
func readHandshake(conn net.Conn) (token, side string, err error) {
	line, err := readLine(conn)
	if err != nil {
		return "", "", err
	}
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
