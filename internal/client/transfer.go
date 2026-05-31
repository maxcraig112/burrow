package client

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/maxcraig112/burrow/internal/pake"
)

const chunkSize = 64 * 1024 // 64 KB per encrypted chunk

type fileHeader struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// IncomingFile holds the metadata received from the sender before the file
// data begins. Call Save to write the file to disk.
type IncomingFile struct {
	Name string
	Size int64
	conn net.Conn
	aead cipher.AEAD
}

// SendFile connects to the relay, waits for the receiver, then streams the
// file in AES-256-GCM encrypted chunks. progress is called with (sent, total)
// after each chunk and may be nil.
func SendFile(keys *pake.DerivedKeys, filePath string, progress func(sent, total int64)) error {
	conn, err := connectRelay(RelayAddr, keys.RelayToken, "sender")
	if err != nil {
		return err
	}
	defer conn.Close()

	aead, err := newAEAD(keys.FileKey)
	if err != nil {
		return fmt.Errorf("create cipher: %w", err)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	hdr, _ := json.Marshal(fileHeader{Name: filepath.Base(filePath), Size: info.Size()})
	if err := writeChunk(conn, aead, hdr); err != nil {
		return fmt.Errorf("send header: %w", err)
	}

	buf := make([]byte, chunkSize)
	var sent int64
	for {
		n, readErr := f.Read(buf)
		if n > 0 {
			if err := writeChunk(conn, aead, buf[:n]); err != nil {
				return fmt.Errorf("send chunk: %w", err)
			}
			sent += int64(n)
			if progress != nil {
				progress(sent, info.Size())
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read file: %w", readErr)
		}
	}

	// Zero-length marker signals end of stream.
	_, err = conn.Write([]byte{0, 0, 0, 0})
	return err
}

// ReceiveHeader connects to the relay and reads the encrypted file header,
// returning an IncomingFile ready to Save.
func ReceiveHeader(keys *pake.DerivedKeys) (*IncomingFile, error) {
	conn, err := connectRelay(RelayAddr, keys.RelayToken, "receiver")
	if err != nil {
		return nil, err
	}

	aead, err := newAEAD(keys.FileKey)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	hdrBytes, err := readChunk(conn, aead)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("receive header: %w", err)
	}

	var hdr fileHeader
	if err := json.Unmarshal(hdrBytes, &hdr); err != nil {
		conn.Close()
		return nil, fmt.Errorf("parse header: %w", err)
	}

	return &IncomingFile{Name: hdr.Name, Size: hdr.Size, conn: conn, aead: aead}, nil
}

// Save writes the incoming file to destDir. progress is called with
// (received, total) after each chunk and may be nil. Returns the path saved.
func (f *IncomingFile) Save(destDir string, progress func(received, total int64)) (string, error) {
	defer f.conn.Close()

	destPath := filepath.Join(destDir, f.Name)
	out, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}

	var received int64
	for {
		chunk, err := readChunk(f.conn, f.aead)
		if err == io.EOF {
			break
		}
		if err != nil {
			out.Close()
			os.Remove(destPath)
			return "", fmt.Errorf("receive chunk: %w", err)
		}
		if _, err := out.Write(chunk); err != nil {
			out.Close()
			os.Remove(destPath)
			return "", fmt.Errorf("write chunk: %w", err)
		}
		received += int64(len(chunk))
		if progress != nil {
			progress(received, f.Size)
		}
	}

	if err := out.Close(); err != nil {
		return "", err
	}
	return destPath, nil
}

// connectRelay dials the relay server and performs the pairing handshake.
// It blocks until the peer connects (or the relay times out).
func connectRelay(addr, token, side string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", addr, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connect to relay: %w", err)
	}

	fmt.Fprintf(conn, "please relay %s for %s\n", token, side)

	// Read the relay's response byte-by-byte to avoid buffering file data.
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

// writeChunk encrypts plaintext and writes [4-byte length][nonce][ciphertext+tag].
func writeChunk(conn net.Conn, aead cipher.AEAD, plaintext []byte) error {
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	ct := aead.Seal(nonce, nonce, plaintext, nil) // nonce || ciphertext+tag

	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(ct)))
	if _, err := conn.Write(lenBuf[:]); err != nil {
		return err
	}
	_, err := conn.Write(ct)
	return err
}

// readChunk reads one encrypted chunk. Returns io.EOF when the zero-length
// end-of-stream marker is read.
func readChunk(conn net.Conn, aead cipher.AEAD) ([]byte, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
		return nil, err
	}
	chunkLen := binary.BigEndian.Uint32(lenBuf[:])
	if chunkLen == 0 {
		return nil, io.EOF
	}
	data := make([]byte, chunkLen)
	if _, err := io.ReadFull(conn, data); err != nil {
		return nil, err
	}
	ns := aead.NonceSize()
	if len(data) < ns {
		return nil, fmt.Errorf("chunk too small")
	}
	return aead.Open(nil, data[:ns], data[ns:], nil)
}
