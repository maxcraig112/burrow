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
	"time"

	"github.com/maxcraig112/burrow/internal/pake"
)

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

// SendDir connects to the relay and streams the directory as a gzip-compressed
// tar archive. This eliminates per-file protocol overhead and compresses
// text-heavy directories (configs, TOMLs, JSONs) significantly.
//
// onFile is called just before each file is added to the archive; may be nil.
// progress is called with (uncompressedBytesSent, totalUncompressedBytes)
// after each file is written into the archive; may be nil.
func SendDir(keys *pake.DerivedKeys, dirPath string, onFile func(relPath string, size int64), progress func(sent, total int64)) error {
	type entry struct {
		abs     string
		rel     string
		size    int64
		modTime time.Time
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
		files = append(files, entry{
			abs:     path,
			rel:     filepath.ToSlash(rel),
			size:    info.Size(),
			modTime: info.ModTime(),
		})
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

	// Write the tar.gz stream into a pipe in a goroutine; the main thread reads
	// from the pipe and streams it as encrypted chunks. No temp file needed.
	pr, pw := io.Pipe()
	tarErr := make(chan error, 1)

	go func() {
		gz, err := gzip.NewWriterLevel(pw, gzip.BestSpeed)
		if err != nil {
			pw.CloseWithError(err)
			tarErr <- err
			return
		}
		tw := tar.NewWriter(gz)

		var sent int64
		for _, fe := range files {
			if onFile != nil {
				onFile(fe.rel, fe.size)
			}

			f, err := os.Open(fe.abs)
			if err != nil {
				pw.CloseWithError(err)
				tarErr <- fmt.Errorf("open %s: %w", fe.rel, err)
				return
			}

			if err := tw.WriteHeader(&tar.Header{
				Name:    fe.rel,
				Size:    fe.size,
				Mode:    0644,
				ModTime: fe.modTime,
			}); err != nil {
				f.Close()
				pw.CloseWithError(err)
				tarErr <- err
				return
			}

			n, err := io.Copy(tw, f)
			f.Close()
			if err != nil {
				pw.CloseWithError(err)
				tarErr <- err
				return
			}

			sent += n
			if progress != nil {
				progress(sent, totalSize)
			}
		}

		if err := tw.Close(); err != nil {
			pw.CloseWithError(err)
			tarErr <- err
			return
		}
		if err := gz.Close(); err != nil {
			pw.CloseWithError(err)
			tarErr <- err
			return
		}
		pw.Close()
		tarErr <- nil
	}()

	if err := streamReader(bw, aead, pr); err != nil {
		pr.CloseWithError(err)
		return err
	}
	if err := bw.Flush(); err != nil {
		pr.CloseWithError(err)
		return err
	}
	return <-tarErr
}

// streamFile sends the contents of f as encrypted chunks then writes the
// zero-length EOF marker. Used by SendFile for single-file transfers.
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

// streamReader reads from r in chunkSize blocks, encrypts each block, and
// writes it to w. Used by SendDir to stream the tar.gz pipe. Progress tracking
// is handled by the tar goroutine (which counts uncompressed bytes).
func streamReader(w io.Writer, aead cipher.AEAD, r io.Reader) error {
	buf := make([]byte, chunkSize)
	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			if err := writeChunk(w, aead, buf[:n]); err != nil {
				return fmt.Errorf("send chunk: %w", err)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read: %w", readErr)
		}
	}
	_, err := w.Write([]byte{0, 0, 0, 0})
	return err
}
