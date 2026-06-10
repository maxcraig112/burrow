package client

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/cipher"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/maxcraig112/burrow/internal/pake"
)

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
// progress is called with (totalReceived, totalExpected) after each file; may be nil.
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

	// Directory transfer: decrypt chunks into a pipe, decompress with gzip,
	// then extract each tar entry to disk.
	dirPath := filepath.Join(destDir, t.Name)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}

	pr, pw := io.Pipe()
	decryptErr := make(chan error, 1)

	// Goroutine: decrypt encrypted chunks and feed the plaintext into the pipe.
	go func() {
		for {
			chunk, err := readChunk(t.br, t.aead)
			if err == io.EOF {
				pw.Close()
				decryptErr <- nil
				return
			}
			if err != nil {
				pw.CloseWithError(err)
				decryptErr <- err
				return
			}
			if _, err := pw.Write(chunk); err != nil {
				// Pipe reader closed; tar extraction is done or errored.
				decryptErr <- nil
				return
			}
		}
	}()

	gz, err := gzip.NewReader(pr)
	if err != nil {
		pr.CloseWithError(err)
		<-decryptErr
		return "", fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var totalReceived int64

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			pr.CloseWithError(err)
			<-decryptErr
			return "", fmt.Errorf("read tar entry: %w", err)
		}

		// Guard against path traversal.
		cleanRel := filepath.Clean(filepath.FromSlash(hdr.Name))
		if filepath.IsAbs(cleanRel) || strings.HasPrefix(cleanRel, "..") {
			pr.CloseWithError(fmt.Errorf("path traversal"))
			<-decryptErr
			return "", fmt.Errorf("rejected unsafe path in transfer: %s", hdr.Name)
		}
		destPath := filepath.Join(dirPath, cleanRel)

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			pr.CloseWithError(err)
			<-decryptErr
			return "", fmt.Errorf("create subdirectory: %w", err)
		}

		if onFile != nil {
			onFile(hdr.Name, hdr.Size)
		}

		out, err := os.Create(destPath)
		if err != nil {
			pr.CloseWithError(err)
			<-decryptErr
			return "", fmt.Errorf("create %s: %w", hdr.Name, err)
		}

		n, err := io.Copy(out, tr)
		out.Close()
		if err != nil {
			os.Remove(destPath)
			pr.CloseWithError(err)
			<-decryptErr
			return "", fmt.Errorf("write %s: %w", hdr.Name, err)
		}

		totalReceived += n
		if progress != nil {
			progress(totalReceived, t.Size)
		}
	}

	if err := <-decryptErr; err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return dirPath, nil
}

// receiveFileData reads encrypted chunks until the zero-length EOF marker and
// writes them to destPath. Used for single-file transfers only.
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
