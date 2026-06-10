package client

import (
	"bufio"
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
	"strings"
	"time"

	"github.com/maxcraig112/burrow/internal/pake"
)

const chunkSize = 64 * 1024 // 64 KB per encrypted chunk

// IOBufferSize is the write-buffer size used by SendDir and the read-buffer
// size used by ReceiveTransfer. Larger values coalesce more small-file frames
// into fewer TCP segments. Override in tests to compare different sizes.
var IOBufferSize = 256 * 1024

// fileHeader is sent as the first encrypted chunk of every transfer.
// Type "dir" precedes a directory transfer; omitted/"file" is a single file.
type fileHeader struct {
	Type  string `json:"type,omitempty"`  // "file" (default) or "dir"
	Name  string `json:"name"`
	Size  int64  `json:"size,omitempty"`  // bytes; total for dirs
	Count int    `json:"count,omitempty"` // number of files (dirs only)
	Path  string `json:"path,omitempty"`  // slash-separated relative path (files in a dir)
}

// IncomingTransfer holds the metadata from the sender before data begins.
// Call Save to write to disk.
type IncomingTransfer struct {
	IsDir bool
	Name  string // file name or directory name
	Size  int64  // total bytes
	Count int    // number of files (1 for a single-file transfer)
	conn  net.Conn
	br    *bufio.Reader // buffered reader wrapping conn; reduces read syscall count
	aead  cipher.AEAD
}

// SendFile connects to the relay and streams a single file encrypted.
// progress is called with (sent, total) after each chunk; may be nil.
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

	return streamFile(conn, aead, f, info.Size(), 0, progress)
}

// SendDir connects to the relay and streams every file under dirPath.
// onFile is called just before each file begins sending; may be nil.
// progress is called with (totalSent, totalSize) after each chunk; may be nil.
//
// Writes are buffered (256 KB) so that per-file protocol frames for small
// files are coalesced into large TCP segments instead of many tiny ones.
func SendDir(keys *pake.DerivedKeys, dirPath string, onFile func(relPath string, size int64), progress func(sent, total int64)) error {
	type entry struct {
		abs  string
		rel  string // slash-separated relative path
		size int64
	}

	var files []entry
	var totalSize int64
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dirPath, path)
		if err != nil {
			return err
		}
		files = append(files, entry{abs: path, rel: filepath.ToSlash(rel), size: info.Size()})
		totalSize += info.Size()
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk directory: %w", err)
	}

	conn, err := connectRelay(RelayAddr, keys.RelayToken, "sender")
	if err != nil {
		return err
	}
	defer conn.Close()

	aead, err := newAEAD(keys.FileKey)
	if err != nil {
		return fmt.Errorf("create cipher: %w", err)
	}

	// Wrap the connection in a write buffer. Small-file directories produce
	// many tiny encrypted frames; buffering coalesces them into large TCP
	// segments and cuts syscall count from O(files) to O(bytes/bufferSize).
	bw := bufio.NewWriterSize(conn, IOBufferSize)

	dirHdr, _ := json.Marshal(fileHeader{
		Type:  "dir",
		Name:  filepath.Base(dirPath),
		Size:  totalSize,
		Count: len(files),
	})
	if err := writeChunk(bw, aead, dirHdr); err != nil {
		return fmt.Errorf("send dir header: %w", err)
	}

	var totalSent int64
	for _, fe := range files {
		if onFile != nil {
			onFile(fe.rel, fe.size)
		}

		fhdr, _ := json.Marshal(fileHeader{
			Type: "file",
			Name: filepath.Base(fe.abs),
			Path: fe.rel,
			Size: fe.size,
		})
		if err := writeChunk(bw, aead, fhdr); err != nil {
			return fmt.Errorf("send file header: %w", err)
		}

		f, err := os.Open(fe.abs)
		if err != nil {
			return fmt.Errorf("open %s: %w", fe.rel, err)
		}

		var wrap func(sent, _ int64)
		if progress != nil {
			wrap = func(sent, _ int64) { progress(totalSent+sent, totalSize) }
		}
		if err := streamFile(bw, aead, f, fe.size, 0, wrap); err != nil {
			f.Close()
			return err
		}
		totalSent += fe.size
		f.Close()
	}

	return bw.Flush()
}

// ReceiveTransfer connects to the relay, reads the initial header, and returns
// an IncomingTransfer. Call Save to write to disk.
func ReceiveTransfer(keys *pake.DerivedKeys) (*IncomingTransfer, error) {
	conn, err := connectRelay(RelayAddr, keys.RelayToken, "receiver")
	if err != nil {
		return nil, err
	}

	aead, err := newAEAD(keys.FileKey)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	// Buffer incoming data to amortise read syscalls over the many small
	// chunk frames that arrive during a directory transfer.
	br := bufio.NewReaderSize(conn, IOBufferSize)

	hdrBytes, err := readChunk(br, aead)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("receive header: %w", err)
	}

	var hdr fileHeader
	if err := json.Unmarshal(hdrBytes, &hdr); err != nil {
		conn.Close()
		return nil, fmt.Errorf("parse header: %w", err)
	}

	t := &IncomingTransfer{conn: conn, br: br, aead: aead, Name: hdr.Name, Size: hdr.Size}
	if hdr.Type == "dir" {
		t.IsDir = true
		t.Count = hdr.Count
	} else {
		t.Count = 1
	}
	return t, nil
}

// Save writes the transfer to destDir.
//   - Single file: saved as destDir/name; returns that path.
//   - Directory:   saved as destDir/dirname/; returns that path.
//
// onFile is called before each file begins writing (relPath, size); may be nil.
// progress is called with (totalReceived, totalExpected) after each chunk; may be nil.
func (t *IncomingTransfer) Save(destDir string, onFile func(relPath string, size int64), progress func(received, total int64)) (string, error) {
	defer t.conn.Close()

	if !t.IsDir {
		if onFile != nil {
			onFile(t.Name, t.Size)
		}
		destPath := filepath.Join(destDir, t.Name)
		if err := receiveFileData(t.br, t.aead, destPath, t.Size, func(n, _ int64) {
			if progress != nil {
				progress(n, t.Size)
			}
		}); err != nil {
			return "", err
		}
		return destPath, nil
	}

	// Directory transfer.
	dirPath := filepath.Join(destDir, t.Name)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}

	var totalReceived int64
	for i := 0; i < t.Count; i++ {
		hdrBytes, err := readChunk(t.br, t.aead)
		if err != nil {
			return "", fmt.Errorf("receive file header: %w", err)
		}
		var hdr fileHeader
		if err := json.Unmarshal(hdrBytes, &hdr); err != nil {
			return "", fmt.Errorf("parse file header: %w", err)
		}

		// Guard against path traversal.
		cleanRel := filepath.Clean(filepath.FromSlash(hdr.Path))
		if filepath.IsAbs(cleanRel) || strings.HasPrefix(cleanRel, "..") {
			return "", fmt.Errorf("rejected unsafe path in transfer: %s", hdr.Path)
		}
		destPath := filepath.Join(dirPath, cleanRel)

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return "", fmt.Errorf("create subdirectory: %w", err)
		}

		if onFile != nil {
			onFile(hdr.Path, hdr.Size)
		}

		localReceived := int64(0)
		if err := receiveFileData(t.br, t.aead, destPath, hdr.Size, func(n, _ int64) {
			delta := n - localReceived
			localReceived = n
			totalReceived += delta
			if progress != nil {
				progress(totalReceived, t.Size)
			}
		}); err != nil {
			return "", err
		}
	}

	return dirPath, nil
}

// receiveFileData reads encrypted chunks until the zero-length EOF marker and
// writes them to destPath.
func receiveFileData(r io.Reader, aead cipher.AEAD, destPath string, _ int64, progress func(received, total int64)) error {
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}

	var received int64
	for {
		chunk, err := readChunk(r, aead)
		if err == io.EOF {
			break
		}
		if err != nil {
			out.Close()
			os.Remove(destPath)
			return fmt.Errorf("receive chunk: %w", err)
		}
		if _, err := out.Write(chunk); err != nil {
			out.Close()
			os.Remove(destPath)
			return fmt.Errorf("write chunk: %w", err)
		}
		received += int64(len(chunk))
		if progress != nil {
			progress(received, 0)
		}
	}

	return out.Close()
}

// streamFile sends the contents of f as encrypted chunks then writes the
// zero-length EOF marker.
func streamFile(w io.Writer, aead cipher.AEAD, f *os.File, size, offset int64, progress func(sent, total int64)) error {
	buf := make([]byte, chunkSize)
	var sent int64
	for {
		n, readErr := f.Read(buf)
		if n > 0 {
			if err := writeChunk(w, aead, buf[:n]); err != nil {
				return fmt.Errorf("send chunk: %w", err)
			}
			sent += int64(n)
			if progress != nil {
				progress(offset+sent, size)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read file: %w", readErr)
		}
	}
	_, err := w.Write([]byte{0, 0, 0, 0})
	return err
}

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

	// Pre-allocate one frame buffer: 4-byte length prefix + ciphertext.
	// A single Write avoids two syscalls (or two bufio copies) per chunk.
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
