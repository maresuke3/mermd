package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mermd/internal/render"
	"mermd/internal/server"
)

func TestMarkdownToHTML_MermaidAndGFM(t *testing.T) {
	src := []byte("# Hello\n\n| Col1 | Col2 |\n| --- | --- |\n| A | B |\n\n```mermaid\ngraph TD\n  A[Start] --> B[End]\n```\n\n```go\nfmt.Println(\"hi\")\n```\n")

	out, err := render.MarkdownToHTML(src)
	if err != nil {
		t.Fatalf("MarkdownToHTML failed: %v", err)
	}

	checks := []string{
		`<h1 id="hello">Hello</h1>`,
		`<table>`,
		`<div class="mermaid-wrapper"><pre class="mermaid">graph TD`,
		`A[Start] --&gt; B[End]`,
		`<code class="language-go">`,
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q\ngot:\n%s", want, out)
		}
	}
}

func TestSplitMarkdownChunks(t *testing.T) {
	src := "# Heading\n\n```go\nfmt.Println(1)\n```\n\n```mermaid\ngraph LR\n  A --> B\n```\n\nFooter text\n"
	chunks := render.SplitMarkdownChunks(src)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d: %#v", len(chunks), chunks)
	}
	if chunks[0].IsMermaid || !strings.Contains(chunks[0].Content, "fmt.Println(1)") {
		t.Errorf("unexpected chunk 0: %#v", chunks[0])
	}
	if !chunks[1].IsMermaid || !strings.Contains(chunks[1].Content, "A --> B") {
		t.Errorf("unexpected chunk 1: %#v", chunks[1])
	}
	if chunks[2].IsMermaid || !strings.Contains(chunks[2].Content, "Footer text") {
		t.Errorf("unexpected chunk 2: %#v", chunks[2])
	}
}

func TestRenderTerminal_MarkdownFallback(t *testing.T) {
	src := []byte("# Terminal Test\n\nHello **world**\n")
	var buf bytes.Buffer
	err := render.RenderTerminal(&buf, src, render.TerminalOptions{
		Style: "notty",
		Width: 80,
	})
	if err != nil {
		t.Fatalf("RenderTerminal failed: %v", err)
	}
	if !strings.Contains(buf.String(), "Terminal Test") || !strings.Contains(buf.String(), "world") {
		t.Errorf("unexpected terminal output: %s", buf.String())
	}
}

func TestWriteKittyImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 50))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := render.WriteKittyImage(&out, pngBuf.Bytes(), 80); err != nil {
		t.Fatalf("WriteKittyImage failed: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "\x1b_Ga=T,f=32,s=100,v=50,c=80,r=20,m=1,q=2;") {
		t.Errorf("unexpected kitty header in output: %q", got[:min(len(got), 120)])
	}
	if !strings.Contains(got, "\x1b_Gm=0;") {
		t.Errorf("missing terminating kitty chunk")
	}
}

func TestRenderPage_StandaloneAndLiveReload(t *testing.T) {
	src := []byte("# Title\n\n```mermaid\nsequenceDiagram\n  Alice->>Bob: Hi\n```\n")

	standalone, err := render.RenderPage("test.md", src, false)
	if err != nil {
		t.Fatalf("RenderPage standalone failed: %v", err)
	}
	if strings.Contains(standalone, "EventSource('/events')") {
		t.Errorf("standalone HTML should not contain EventSource")
	}
	if !strings.Contains(standalone, "mermaid.esm.min.mjs") {
		t.Errorf("standalone HTML should include mermaid script")
	}

	live, err := render.RenderPage("test.md", src, true)
	if err != nil {
		t.Fatalf("RenderPage live failed: %v", err)
	}
	if !strings.Contains(live, "EventSource('/events')") {
		t.Errorf("live HTML should contain EventSource")
	}
}

func TestServerHandlers(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(mdPath, []byte("# Server Test\n\n```mermaid\ngraph LR\n  A --> B\n```\n"), 0644); err != nil {
		t.Fatal(err)
	}
	imgPath := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(imgPath, []byte("asset-ok"), 0644); err != nil {
		t.Fatal(err)
	}

	mux := server.NewHandler(server.Options{
		Title:      "doc.md",
		FilePath:   mdPath,
		LiveReload: true,
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Server Test") {
		t.Fatalf("GET / unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/content", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `<pre class="mermaid">`) {
		t.Fatalf("GET /api/content unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/sample.txt", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "asset-ok" {
		t.Fatalf("GET /sample.txt unexpected: code=%d body=%s", rec.Code, rec.Body.String())
	}
}
