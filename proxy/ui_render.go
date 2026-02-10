package proxy

import (
	"bytes"
	"html/template"
	"io"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	mdhtml "github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

// chromaHTMLFormatter is a shared HTML formatter for syntax highlighting
var chromaHTMLFormatter = html.New(
	html.WithClasses(true),
	html.WithLineNumbers(false),
	html.TabWidth(2),
)

// chromaStyle is the syntax highlighting style
var chromaStyle = styles.Get("github")

// RenderMarkdown converts markdown text to HTML with syntax highlighting
func RenderMarkdown(text string) template.HTML {
	if text == "" {
		return template.HTML("")
	}

	// Create parser with extensions
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.Strikethrough
	p := parser.NewWithExtensions(extensions)

	// Parse markdown
	doc := p.Parse([]byte(text))

	// Render with custom code block handler
	opts := mdhtml.RendererOptions{
		Flags:          mdhtml.CommonFlags | mdhtml.HrefTargetBlank,
		RenderNodeHook: renderCodeBlockHook,
	}
	renderer := mdhtml.NewRenderer(opts)

	html := markdown.Render(doc, renderer)
	return template.HTML(html)
}

// renderCodeBlockHook is a custom hook to render code blocks with syntax highlighting
func renderCodeBlockHook(w io.Writer, node ast.Node, entering bool) (ast.WalkStatus, bool) {
	codeBlock, ok := node.(*ast.CodeBlock)
	if !ok {
		return ast.GoToNext, false
	}

	if entering {
		lang := string(codeBlock.Info)
		code := string(codeBlock.Literal)

		// Render with syntax highlighting
		highlighted := RenderCodeBlock(code, lang)
		w.Write([]byte(highlighted))
	}

	return ast.GoToNext, true
}

// RenderCodeBlock renders a code block with syntax highlighting
func RenderCodeBlock(code, lang string) template.HTML {
	if code == "" {
		return template.HTML("")
	}

	// Clean up language identifier
	lang = strings.TrimSpace(lang)
	if lang == "" {
		lang = "text"
	}

	// Get lexer for language
	lexer := lexers.Get(lang)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	// Tokenize
	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		// Fallback to plain text on error
		return template.HTML("<pre><code>" + template.HTMLEscapeString(code) + "</code></pre>")
	}

	// Render to HTML
	var buf bytes.Buffer
	buf.WriteString("<div class=\"chroma\">")
	err = chromaHTMLFormatter.Format(&buf, chromaStyle, iterator)
	buf.WriteString("</div>")

	if err != nil {
		// Fallback to plain text on error
		return template.HTML("<pre><code>" + template.HTMLEscapeString(code) + "</code></pre>")
	}

	return template.HTML(buf.String())
}

// GenerateChromaCSS generates the CSS for syntax highlighting
func GenerateChromaCSS() (string, error) {
	var buf bytes.Buffer
	err := chromaHTMLFormatter.WriteCSS(&buf, chromaStyle)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}
