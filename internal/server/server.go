package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mermd/internal/render"
)

// Options configures the preview HTTP server.
type Options struct {
	Title         string
	FilePath      string
	StaticContent []byte
	Port          int
	LiveReload    bool
	OnReady       func(url string)
}

// Serve starts the HTTP preview server and blocks until ctx is cancelled.
func Serve(ctx context.Context, opts Options) error {
	addr := fmt.Sprintf("127.0.0.1:%d", opts.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	mux := NewHandler(opts)
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	if opts.OnReady != nil {
		opts.OnReady("http://" + ln.Addr().String())
	}

	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// NewHandler creates the HTTP handler for the preview server.
func NewHandler(opts Options) http.Handler {
	mux := http.NewServeMux()

	readSource := func() ([]byte, error) {
		if opts.FilePath == "" {
			return opts.StaticContent, nil
		}
		return os.ReadFile(opts.FilePath)
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			serveStaticAsset(w, r, opts.FilePath)
			return
		}
		src, err := readSource()
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to read markdown: %v", err), http.StatusInternalServerError)
			return
		}
		page, err := render.RenderPage(opts.Title, src, opts.LiveReload)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to render markdown: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	})

	mux.HandleFunc("/api/content", func(w http.ResponseWriter, r *http.Request) {
		src, err := readSource()
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to read markdown: %v", err), http.StatusInternalServerError)
			return
		}
		fragment, err := render.MarkdownToHTML(src)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to render markdown: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(fragment))
	})

	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher.Flush()

		if opts.FilePath == "" || !opts.LiveReload {
			<-r.Context().Done()
			return
		}

		var lastMod time.Time
		var lastSize int64
		if info, err := os.Stat(opts.FilePath); err == nil {
			lastMod = info.ModTime()
			lastSize = info.Size()
		}

		ticker := time.NewTicker(300 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				info, err := os.Stat(opts.FilePath)
				if err != nil {
					continue
				}
				if !info.ModTime().Equal(lastMod) || info.Size() != lastSize {
					lastMod = info.ModTime()
					lastSize = info.Size()
					_, _ = fmt.Fprint(w, "event: reload\ndata: changed\n\n")
					flusher.Flush()
				}
			}
		}
	})

	return mux
}

func serveStaticAsset(w http.ResponseWriter, r *http.Request, mdFilePath string) {
	if mdFilePath == "" {
		http.NotFound(w, r)
		return
	}
	baseDir, err := filepath.Abs(filepath.Dir(mdFilePath))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	cleaned := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	target := filepath.Join(baseDir, cleaned)
	rel, err := filepath.Rel(baseDir, target)
	if err != nil || strings.HasPrefix(rel, "..") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	info, err := os.Stat(target)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	http.ServeFile(w, r, target)
}
