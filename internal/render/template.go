package render

import (
	"bytes"
	_ "embed"
	"html/template"
)

//go:embed template.html
var rawTemplate string

var pageTmpl = template.Must(template.New("page").Parse(rawTemplate))

// PageData holds data passed into the full HTML document template.
type PageData struct {
	Title      string
	Content    template.HTML
	LiveReload bool
}

// RenderPage renders a full standalone HTML document from a Markdown byte slice.
func RenderPage(title string, markdown []byte, liveReload bool) (string, error) {
	bodyHTML, err := MarkdownToHTML(markdown)
	if err != nil {
		return "", err
	}

	var out bytes.Buffer
	err = pageTmpl.Execute(&out, PageData{
		Title:      title,
		Content:    template.HTML(bodyHTML),
		LiveReload: liveReload,
	})
	if err != nil {
		return "", err
	}
	return out.String(), nil
}
