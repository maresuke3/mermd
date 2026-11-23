package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"mermd/internal/render"
	"mermd/internal/server"
)

const version = "0.2.0"

func main() {
	var (
		webMode     bool
		port        int
		noOpen      bool
		outputHTML  bool
		style       string
		imageFormat string
		width       int
		showVersion bool
	)

	flag.BoolVar(&webMode, "web", false, "open preview in browser via local HTTP server")
	flag.BoolVar(&webMode, "w", false, "open preview in browser (shorthand)")
	flag.IntVar(&port, "port", 0, "port for --web server (default: random available port)")
	flag.IntVar(&port, "p", 0, "port for --web server (shorthand)")
	flag.BoolVar(&noOpen, "no-open", false, "do not open browser automatically in --web mode")
	flag.BoolVar(&noOpen, "n", false, "do not open browser automatically in --web mode (shorthand)")
	flag.BoolVar(&outputHTML, "html", false, "output rendered HTML to stdout instead of terminal")
	flag.StringVar(&style, "style", "auto", "terminal style: auto, dark, light, notty")
	flag.StringVar(&style, "s", "auto", "terminal style (shorthand)")
	flag.StringVar(&imageFormat, "image", "auto", "terminal diagram format: auto, kitty, sixel, iterm, symbols")
	flag.IntVar(&width, "width", 0, "terminal word-wrap width (default: auto)")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.BoolVar(&showVersion, "v", false, "print version and exit (shorthand)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "mermd - Markdown & Mermaid CLI previewer\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n  mermd [options] [file.md | -]\n\n")
		fmt.Fprintf(os.Stderr, "Default mode renders Markdown and Mermaid diagrams directly in the terminal.\n")
		fmt.Fprintf(os.Stderr, "Use -w / --web to open a live-reloading browser preview.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if showVersion {
		fmt.Printf("mermd v%s\n", version)
		return
	}

	opts, err := resolveInput(flag.Args(), port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	src, err := loadSource(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if outputHTML {
		htmlDoc, err := render.RenderPage(opts.Title, src, false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(htmlDoc)
		return
	}

	if !webMode {
		err := render.RenderTerminal(os.Stdout, src, render.TerminalOptions{
			Style:       style,
			ImageFormat: imageFormat,
			Width:       width,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	opts.OnReady = func(url string) {
		fmt.Fprintf(os.Stderr, "mermd serving %s at %s (Ctrl+C to stop)\n", opts.Title, url)
		if !noOpen {
			if err := openBrowser(url); err != nil {
				fmt.Fprintf(os.Stderr, "Could not open browser automatically: %v\n", err)
			}
		}
	}

	if err := server.Serve(ctx, opts); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

func loadSource(opts server.Options) ([]byte, error) {
	if opts.FilePath == "" {
		return opts.StaticContent, nil
	}
	return os.ReadFile(opts.FilePath)
}

func resolveInput(args []string, port int) (server.Options, error) {
	opts := server.Options{
		Port:       port,
		LiveReload: true,
	}

	if len(args) == 0 {
		if hasStdinPipe() {
			return readStdinOptions(opts)
		}
		if _, err := os.Stat("README.md"); err == nil {
			abs, err := filepath.Abs("README.md")
			if err != nil {
				return opts, err
			}
			opts.FilePath = abs
			opts.Title = "README.md"
			return opts, nil
		}
		return opts, errors.New("no input file provided (and no README.md found in current directory)")
	}

	target := args[0]
	if target == "-" {
		return readStdinOptions(opts)
	}

	abs, err := filepath.Abs(target)
	if err != nil {
		return opts, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return opts, err
	}
	if info.IsDir() {
		return opts, fmt.Errorf("%s is a directory", target)
	}

	opts.FilePath = abs
	opts.Title = filepath.Base(abs)
	return opts, nil
}

func readStdinOptions(opts server.Options) (server.Options, error) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return opts, fmt.Errorf("read stdin: %w", err)
	}
	opts.StaticContent = data
	opts.Title = "stdin"
	opts.LiveReload = false
	return opts, nil
}

func hasStdinPipe() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) == 0
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	return cmd.Start()
}
