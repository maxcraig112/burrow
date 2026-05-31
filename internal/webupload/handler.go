package webupload

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/maxcraig112/burrow/internal/progress"
)

//go:embed ui.html
var uiHTMLRaw string

var uiTmpl = template.Must(template.New("ui").Parse(uiHTMLRaw))

// NewHandler returns an http.Handler that serves the upload UI and saves
// incoming files to destDir. The nameplate is used as the URL path prefix.
// onUpload is called with the number of files saved after each upload batch;
// it may be nil.
func NewHandler(destDir, nameplate, description string, onUpload func(int)) http.Handler {
	h := &uploadHandler{
		destDir:     destDir,
		nameplate:   nameplate,
		description: description,
		onUpload:    onUpload,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /"+nameplate+"/", h.serveUI)
	mux.HandleFunc("POST /"+nameplate+"/upload", h.handleUpload)
	return mux
}

type uploadHandler struct {
	destDir     string
	nameplate   string
	description string
	onUpload    func(int)
	mu          sync.Mutex // serialises terminal progress output
}

func (h *uploadHandler) serveUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	uiTmpl.Execute(w, map[string]string{ //nolint:errcheck
		"Nameplate":   h.nameplate,
		"Description": h.description,
		"Folder":      filepath.Base(h.destDir),
	})
}

func (h *uploadHandler) handleUpload(w http.ResponseWriter, r *http.Request) {
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		http.Error(w, "invalid content-type", http.StatusBadRequest)
		return
	}
	boundary, ok := params["boundary"]
	if !ok {
		http.Error(w, "missing boundary", http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	mr := multipart.NewReader(r.Body, boundary)
	var saved []string
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			http.Error(w, "read: "+err.Error(), http.StatusInternalServerError)
			return
		}

		name := filepath.Base(part.FileName())
		if name == "" || name == "." || name == ".." {
			part.Close()
			continue
		}

		destPath := uniquePath(h.destDir, name)
		if err := saveFile(part, destPath, r.ContentLength); err != nil {
			part.Close()
			http.Error(w, "save: "+err.Error(), http.StatusInternalServerError)
			return
		}
		part.Close()
		saved = append(saved, filepath.Base(destPath))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "ok", "saved": saved})

	if h.onUpload != nil && len(saved) > 0 {
		go h.onUpload(len(saved))
	}
}

func saveFile(r io.Reader, destPath string, contentLength int64) (retErr error) {
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer func() {
		f.Close()
		if retErr != nil {
			os.Remove(destPath)
		}
	}()

	name := filepath.Base(destPath)
	fmt.Printf("  Receiving %s...\n", name)

	var bar *progress.Bar
	if contentLength > 0 {
		bar = progress.NewBar(contentLength)
	}

	cr := &countingReader{r: r, bar: bar}
	n, err := io.Copy(f, cr)
	if err != nil {
		return err
	}

	// Replace progress line with the saved confirmation.
	fmt.Printf("\r  Saved %s (%s)\x1b[K\n", name, progress.FormatBytes(n))
	return nil
}

type countingReader struct {
	r   io.Reader
	bar *progress.Bar
	n   int64
}

func (cr *countingReader) Read(p []byte) (int, error) {
	n, err := cr.r.Read(p)
	cr.n += int64(n)
	if cr.bar != nil {
		cr.bar.Print(cr.n)
	}
	return n, err
}

// uniquePath returns dir/name, appending " (N)" if the file already exists.
func uniquePath(dir, name string) string {
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(name)
	base := name[:len(name)-len(ext)]
	for i := 1; ; i++ {
		path = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return path
		}
	}
}
