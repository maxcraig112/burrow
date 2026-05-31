package main

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/maxcraig112/burrow/internal/progress"
)

// cmdSendToSession uploads a file directly to a running receive-web session.
func cmdSendToSession(nameplate, filePath string) {
	base := tunnelBaseURL()
	if base == "" {
		fatalf("no server configured — run: burrow config <exchange-addr> <relay-addr>")
	}

	info, err := os.Stat(filePath)
	if err != nil {
		fatalf("file error: %v", err)
	}
	if info.IsDir() {
		fatalf("%s is a directory — only files are supported", filePath)
	}

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)

	go func() {
		var writeErr error
		defer func() {
			if writeErr != nil {
				pw.CloseWithError(writeErr)
			} else {
				mw.Close()
				pw.Close()
			}
		}()

		part, err := mw.CreateFormFile("file", filepath.Base(filePath))
		if err != nil {
			writeErr = err
			return
		}

		f, err := os.Open(filePath)
		if err != nil {
			writeErr = err
			return
		}
		defer f.Close()

		bar := progress.NewBar(info.Size())
		buf := make([]byte, 32*1024)
		var sent int64
		for {
			n, err := f.Read(buf)
			if n > 0 {
				if _, werr := part.Write(buf[:n]); werr != nil {
					writeErr = werr
					return
				}
				sent += int64(n)
				bar.Print(sent)
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				writeErr = err
				return
			}
		}
	}()

	fmt.Printf("Sending %s (%s) to session %s…\n",
		filepath.Base(filePath), progress.FormatBytes(info.Size()), nameplate)

	uploadURL := base + "/t/" + nameplate + "/upload"
	req, err := http.NewRequest(http.MethodPost, uploadURL, pr)
	if err != nil {
		fatalf("request error: %v", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println()
		fatalf("upload error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var body struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&body) //nolint:errcheck
		fmt.Println()
		if body.Error != "" {
			fatalf("upload failed: %s", body.Error)
		}
		fatalf("upload failed: HTTP %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Saved []string `json:"saved"`
	}
	json.Unmarshal(body, &result) //nolint:errcheck

	fmt.Printf("\n  Delivered %s\n", strings.Join(result.Saved, ", "))
}
