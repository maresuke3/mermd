package render

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/draw"
	_ "image/png"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/glamour"
	"golang.org/x/term"
)

// Chunk represents either a regular Markdown section or a Mermaid diagram block.
type Chunk struct {
	IsMermaid bool
	Content   string
}

// TerminalOptions configures terminal rendering behavior.
type TerminalOptions struct {
	Style       string // auto, dark, light, notty
	ImageFormat string // auto, kitty, sixel, iterm, symbols
	Width       int    // 0 means auto-detect
}

// SplitMarkdownChunks splits Markdown text into alternating regular Markdown and Mermaid chunks.
func SplitMarkdownChunks(src string) []Chunk {
	lines := strings.Split(src, "\n")
	var chunks []Chunk
	var buf []string

	inFence := false
	inMermaid := false
	fenceChar := byte(0)
	fenceLen := 0

	flush := func(isMermaid bool) {
		if len(buf) == 0 {
			return
		}
		text := strings.Join(buf, "\n")
		if isMermaid || strings.TrimSpace(text) != "" {
			chunks = append(chunks, Chunk{
				IsMermaid: isMermaid,
				Content:   text,
			})
		}
		buf = nil
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if !inFence {
			if fc, fl, info, ok := parseFenceOpen(trimmed); ok {
				if strings.EqualFold(strings.TrimSpace(info), "mermaid") {
					flush(false)
					inFence = true
					inMermaid = true
					fenceChar = fc
					fenceLen = fl
					continue
				}
				inFence = true
				inMermaid = false
				fenceChar = fc
				fenceLen = fl
			}
			buf = append(buf, line)
			continue
		}

		// Inside a fenced block
		if isFenceClose(trimmed, fenceChar, fenceLen) {
			if inMermaid {
				flush(true)
			} else {
				buf = append(buf, line)
			}
			inFence = false
			inMermaid = false
			fenceChar = 0
			fenceLen = 0
			continue
		}

		buf = append(buf, line)
	}

	flush(inMermaid)
	return chunks
}

func parseFenceOpen(trimmed string) (char byte, length int, info string, ok bool) {
	if len(trimmed) < 3 {
		return 0, 0, "", false
	}
	c := trimmed[0]
	if c != '`' && c != '~' {
		return 0, 0, "", false
	}
	i := 0
	for i < len(trimmed) && trimmed[i] == c {
		i++
	}
	if i < 3 {
		return 0, 0, "", false
	}
	rest := trimmed[i:]
	if c == '`' && strings.ContainsRune(rest, '`') {
		return 0, 0, "", false
	}
	return c, i, rest, true
}

func isFenceClose(trimmed string, char byte, minLen int) bool {
	if len(trimmed) < minLen {
		return false
	}
	i := 0
	for i < len(trimmed) && trimmed[i] == char {
		i++
	}
	return i >= minLen && strings.TrimSpace(trimmed[i:]) == ""
}

// RenderTerminal renders Markdown and Mermaid diagrams directly to a terminal writer.
func RenderTerminal(w io.Writer, src []byte, opts TerminalOptions) error {
	width := opts.Width
	if width <= 0 {
		if tw, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && tw > 0 {
			width = tw
			if width > 120 {
				width = 120
			}
		} else {
			width = 100
		}
	}

	renderer, err := newGlamourRenderer(opts.Style, width)
	if err != nil {
		return err
	}

	chunks := SplitMarkdownChunks(string(src))
	for _, chunk := range chunks {
		if !chunk.IsMermaid {
			rendered, err := renderer.Render(chunk.Content)
			if err != nil {
				return err
			}
			if _, err := io.WriteString(w, rendered); err != nil {
				return err
			}
			continue
		}

		if err := renderMermaidToTerminal(w, chunk.Content, width, opts, renderer); err != nil {
			// Fallback to code block if diagram rendering fails
			fallback, _ := renderer.Render("```mermaid\n" + chunk.Content + "\n```")
			_, _ = fmt.Fprintf(w, "\n[mermaid render skipped: %v]\n%s", err, fallback)
		}
	}
	return nil
}

func newGlamourRenderer(style string, width int) (*glamour.TermRenderer, error) {
	styleOpt := glamour.WithAutoStyle()
	switch strings.ToLower(strings.TrimSpace(style)) {
	case "", "auto":
		styleOpt = glamour.WithAutoStyle()
	default:
		styleOpt = glamour.WithStandardStyle(style)
	}
	return glamour.NewTermRenderer(
		styleOpt,
		glamour.WithWordWrap(width),
	)
}

func renderMermaidToTerminal(w io.Writer, diagram string, width int, opts TerminalOptions, _ *glamour.TermRenderer) error {
	mmdcPath, err := exec.LookPath("mmdc")
	if err != nil {
		return fmt.Errorf("mmdc not found in PATH")
	}
	if err != nil {
		return fmt.Errorf("mmdc not found in PATH")
	}

	tmpDir, err := os.MkdirTemp("", "mermd-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	inFile := filepath.Join(tmpDir, "diagram.mmd")
	outFile := filepath.Join(tmpDir, "diagram.png")
	pptrFile := filepath.Join(tmpDir, "puppeteer.json")

	if err := os.WriteFile(inFile, []byte(diagram), 0600); err != nil {
		return err
	}
	if err := os.WriteFile(pptrFile, []byte(`{"args":["--no-sandbox","--disable-setuid-sandbox"]}`), 0600); err != nil {
		return err
	}

	theme := "dark"
	bg := "#0d1117"
	if strings.EqualFold(opts.Style, "light") {
		theme = "default"
		bg = "#ffffff"
	}

	mmdcArgs := []string{
		"-i", inFile,
		"-o", outFile,
		"-p", pptrFile,
		"-t", theme,
		"-b", bg,
		"-s", "2",
	}
	var mmdcErr bytes.Buffer
	mmdcCmd := exec.Command(mmdcPath, mmdcArgs...)
	mmdcCmd.Stderr = &mmdcErr
	if err := mmdcCmd.Run(); err != nil {
		msg := strings.TrimSpace(mmdcErr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("mmdc failed: %s", msg)
	}

	maxCols := width - 4
	if maxCols < 40 {
		maxCols = 40
	}

	imgFormat := resolveImageFormat(opts.ImageFormat)

	// Send lossless RGBA pixels directly for Kitty and PNG for iTerm2 so there is no downsampling blur.
	if imgFormat == "kitty" {
		pngData, err := os.ReadFile(outFile)
		if err != nil {
			return err
		}
		return writeKittyImage(w, pngData, maxCols)
	}
	if imgFormat == "iterm" {
		pngData, err := os.ReadFile(outFile)
		if err != nil {
			return err
		}
		return writeITermImage(w, pngData, maxCols)
	}

	chafaPath, err := exec.LookPath("chafa")
	if err != nil {
		return fmt.Errorf("chafa not found in PATH")
	}

	chafaArgs := []string{
		fmt.Sprintf("--size=%d", maxCols),
	}
	if imgFormat != "" && imgFormat != "auto" {
		chafaArgs = append(chafaArgs, "-f", imgFormat)
	}
	chafaArgs = append(chafaArgs, outFile)

	var chafaErr bytes.Buffer
	chafaCmd := exec.Command(chafaPath, chafaArgs...)
	// Connect directly to w (typically os.Stdout TTY) so chafa can detect
	// terminal capabilities and pixel dimensions instead of falling back to ASCII.
	chafaCmd.Stdout = w
	chafaCmd.Stderr = &chafaErr
	_, _ = fmt.Fprintln(w)
	if err := chafaCmd.Run(); err != nil {
		return fmt.Errorf("chafa failed: %v (%s)", err, strings.TrimSpace(chafaErr.String()))
	}
	_, _ = fmt.Fprintln(w)
	return nil
}

// WriteKittyImage encodes a PNG image as raw 32-bit RGBA chunks using the Kitty graphics protocol.
// Sending f=32 with explicit s, v, c, r fields ensures compatibility across Kitty, WezTerm, Ghostty, and herdr.
func WriteKittyImage(w io.Writer, pngData []byte, maxCols int) error {
	img, _, err := image.Decode(bytes.NewReader(pngData))
	if err != nil {
		return fmt.Errorf("decode png: %w", err)
	}

	bounds := img.Bounds()
	pw := bounds.Dx()
	ph := bounds.Dy()
	if pw <= 0 || ph <= 0 {
		return fmt.Errorf("invalid image dimensions: %dx%d", pw, ph)
	}

	// Cap pixel width to keep payload within terminal graphics storage limits (e.g. ~1-2MB).
	const maxPixelWidth = 1000
	if pw > maxPixelWidth {
		newW := maxPixelWidth
		newH := int(math.Round(float64(ph) * float64(newW) / float64(pw)))
		if newH < 1 {
			newH = 1
		}
		img = resizeBilinear(img, newW, newH)
		bounds = img.Bounds()
		pw = bounds.Dx()
		ph = bounds.Dy()
	}

	rgba := image.NewRGBA(image.Rect(0, 0, pw, ph))
	draw.Draw(rgba, rgba.Bounds(), img, bounds.Min, draw.Src)

	// Estimate display cols and rows assuming ~10x20 px cell aspect ratio (1:2)
	cols := maxCols
	if cols <= 0 {
		cols = 80
	}
	rows := int(math.Ceil(float64(ph) * float64(cols) / (float64(pw) * 2.0)))
	if rows < 1 {
		rows = 1
	}

	encoded := base64.StdEncoding.EncodeToString(rgba.Pix)
	const chunkSize = 4096

	if _, err := fmt.Fprintf(w, "\n\x1b_Ga=T,f=32,s=%d,v=%d,c=%d,r=%d,m=1,q=2\x1b\\", pw, ph, cols, rows); err != nil {
		return err
	}
	for i := 0; i < len(encoded); i += chunkSize {
		end := i + chunkSize
		if end > len(encoded) {
			end = len(encoded)
		}
		if _, err := fmt.Fprintf(w, "\x1b_Gm=1;%s\x1b\\", encoded[i:end]); err != nil {
			return err
		}
	}
	_, err = fmt.Fprint(w, "\x1b_Gm=0\x1b\\\n\n")
	return err
}

func resizeBilinear(src image.Image, newW, newH int) image.Image {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	for y := 0; y < newH; y++ {
		sy := float64(y) * float64(sh-1) / float64(max(1, newH-1))
		y0 := int(sy)
		y1 := min(y0+1, sh-1)
		fy := sy - float64(y0)
		for x := 0; x < newW; x++ {
			sx := float64(x) * float64(sw-1) / float64(max(1, newW-1))
			x0 := int(sx)
			x1 := min(x0+1, sw-1)
			fx := sx - float64(x0)

			c00 := src.At(sb.Min.X+x0, sb.Min.Y+y0)
			c10 := src.At(sb.Min.X+x1, sb.Min.Y+y0)
			c01 := src.At(sb.Min.X+x0, sb.Min.Y+y1)
			c11 := src.At(sb.Min.X+x1, sb.Min.Y+y1)

			r00, g00, b00, a00 := c00.RGBA()
			r10, g10, b10, a10 := c10.RGBA()
			r01, g01, b01, a01 := c01.RGBA()
			r11, g11, b11, a11 := c11.RGBA()

			interp := func(v00, v10, v01, v11 uint32) uint8 {
				top := float64(v00)*(1-fx) + float64(v10)*fx
				bot := float64(v01)*(1-fx) + float64(v11)*fx
				v := top*(1-fy) + bot*fy
				return uint8(v / 257.0)
			}

			off := dst.PixOffset(x, y)
			dst.Pix[off+0] = interp(r00, r10, r01, r11)
			dst.Pix[off+1] = interp(g00, g10, g01, g11)
			dst.Pix[off+2] = interp(b00, b10, b01, b11)
			dst.Pix[off+3] = interp(a00, a10, a01, a11)
		}
	}
	return dst
}

func writeKittyImage(w io.Writer, pngData []byte, cols int) error {
	return WriteKittyImage(w, pngData, cols)
}

func writeITermImage(w io.Writer, pngData []byte, cols int) error {
	encoded := base64.StdEncoding.EncodeToString(pngData)
	_, err := fmt.Fprintf(w, "\n\x1b]1337;File=inline=1;width=%d;preserveAspectRatio=1:%s\a\n\n", cols, encoded)
	return err
}

func resolveImageFormat(requested string) string {
	if requested != "" && requested != "auto" {
		return requested
	}
	if os.Getenv("KITTY_WINDOW_ID") != "" ||
		os.Getenv("WEZTERM_PANE") != "" ||
		os.Getenv("GHOSTTY_RESOURCES_DIR") != "" ||
		os.Getenv("HERDR_ENV") != "" ||
		strings.EqualFold(os.Getenv("TERM_PROGRAM"), "herdr") {
		return "kitty"
	}
	if strings.Contains(strings.ToLower(os.Getenv("TERM_PROGRAM")), "iterm") {
		return "iterm"
	}
	return "auto"
}
