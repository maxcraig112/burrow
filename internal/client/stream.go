package client

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

// connectRelay dials the relay server and performs the pairing handshake.
func connectRelay(addr, token, side string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", addr, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connect to relay: %w", err)
	}

	fmt.Fprintf(conn, "please relay %s for %s\n", token, side)

	conn.SetDeadline(time.Now().Add(3 * time.Minute))
	resp, err := readRelayLine(conn)
	conn.SetDeadline(time.Time{})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("relay handshake: %w", err)
	}
	if resp != "ok" {
		conn.Close()
		return nil, fmt.Errorf("relay refused: %s", resp)
	}

	return conn, nil
}

func readRelayLine(conn net.Conn) (string, error) {
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

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// writeChunk encrypts plaintext and writes [4-byte length][nonce][ciphertext+tag]
// as a single write to minimise syscalls.
func writeChunk(w io.Writer, aead cipher.AEAD, plaintext []byte) error {
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	ct := aead.Seal(nonce, nonce, plaintext, nil)

	frame := make([]byte, 4+len(ct))
	binary.BigEndian.PutUint32(frame, uint32(len(ct)))
	copy(frame[4:], ct)
	_, err := w.Write(frame)
	return err
}

// readChunk reads one encrypted chunk. Returns io.EOF on the zero-length marker.
func readChunk(r io.Reader, aead cipher.AEAD) ([]byte, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, err
	}
	chunkLen := binary.BigEndian.Uint32(lenBuf[:])
	if chunkLen == 0 {
		return nil, io.EOF
	}
	data := make([]byte, chunkLen)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}
	ns := aead.NonceSize()
	if len(data) < ns {
		return nil, fmt.Errorf("chunk too small")
	}
	return aead.Open(nil, data[:ns], data[ns:], nil)
}
