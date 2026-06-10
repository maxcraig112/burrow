package client

import (
	"bufio"
	"crypto/cipher"
	"net"
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
	Size  int64  `json:"size,omitempty"`  // uncompressed bytes; total for dirs
	Count int    `json:"count,omitempty"` // number of files (dirs only)
	Path  string `json:"path,omitempty"`  // slash-separated relative path (files in a dir)
}

// IncomingTransfer holds the metadata from the sender before data begins.
// Call Save to write to disk.
type IncomingTransfer struct {
	IsDir bool
	Name  string // file name or directory name
	Size  int64  // total uncompressed bytes
	Count int    // number of files (1 for a single-file transfer)
	conn  net.Conn
	br    *bufio.Reader // buffered reader wrapping conn; reduces read syscall count
	aead  cipher.AEAD
}
