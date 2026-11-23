package render

import (
	"bytes"
	"html"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

type mermaidNodeRenderer struct{}

func (r *mermaidNodeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, r.renderFencedCodeBlock)
}

func (r *mermaidNodeRenderer) renderFencedCodeBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n, ok := node.(*ast.FencedCodeBlock)
	if !ok {
		return ast.WalkContinue, nil
	}
	if !entering {
		return ast.WalkSkipChildren, nil
	}

	var codeBuf bytes.Buffer
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		codeBuf.Write(line.Value(source))
	}

	rawLang := strings.TrimSpace(string(n.Language(source)))
	if strings.EqualFold(rawLang, "mermaid") {
		_, _ = w.WriteString(`<div class="mermaid-wrapper"><pre class="mermaid">`)
		_, _ = w.WriteString(html.EscapeString(codeBuf.String()))
		_, _ = w.WriteString(`</pre></div>` + "\n")
		return ast.WalkSkipChildren, nil
	}

	if rawLang != "" {
		_, _ = w.WriteString(`<pre><code class="language-`)
		_, _ = w.WriteString(html.EscapeString(rawLang))
		_, _ = w.WriteString(`">`)
	} else {
		_, _ = w.WriteString(`<pre><code>`)
	}
	_, _ = w.WriteString(html.EscapeString(codeBuf.String()))
	_, _ = w.WriteString("</code></pre>\n")
	return ast.WalkSkipChildren, nil
}

// MarkdownToHTML converts Markdown bytes to an HTML fragment, preserving mermaid code blocks
// inside .mermaid-wrapper elements for client-side diagram rendering.
func MarkdownToHTML(src []byte) (string, error) {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Footnote,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			gmhtml.WithUnsafe(),
			renderer.WithNodeRenderers(
				util.Prioritized(&mermaidNodeRenderer{}, 100),
			),
		),
	)

	var out bytes.Buffer
	if err := md.Convert(src, &out); err != nil {
		return "", err
	}
	return out.String(), nil
}
