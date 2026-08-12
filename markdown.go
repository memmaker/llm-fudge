package main

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

// markdown parses with GFM on, so tables, strikethrough and task lists in model
// output are structure rather than stray punctuation to be copied through.
var markdown = goldmark.New(goldmark.WithExtensions(extension.GFM))

// plainText strips markdown from prose. The DM writes emphasis and the odd
// heading into its narration; narration.txt is read by things that show it as
// it stands — a plain file, not a source file — so the marks come off here.
// Blocks are kept apart by a blank line and never wrapped: the reader owns the
// line width, and a break mid-sentence is how a phrase stops matching.
func plainText(source string) string {
	src := []byte(source)
	var blocks []string
	_ = ast.Walk(markdown.Parser().Parse(text.NewReader(src)), func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		// Containers hold blocks, not text: their children are the blocks, and
		// each one gets its own paragraph. A list becomes one line per item.
		if !entering || node.Type() != ast.TypeBlock {
			return ast.WalkContinue, nil
		}
		switch node.Kind() {
		case ast.KindDocument, ast.KindList, ast.KindListItem, ast.KindBlockquote:
			return ast.WalkContinue, nil
		}
		// A leaf block is either inline content (paragraph, heading) or raw lines
		// (code, HTML). Both end up as their own text and nothing below them is
		// walked again.
		body := plainInline(node, src)
		if strings.TrimSpace(body) == "" {
			body = rawBlockLines(node, src)
		}
		if body = strings.TrimSpace(body); body != "" {
			blocks = append(blocks, body)
		}
		return ast.WalkSkipChildren, nil
	})
	return strings.Join(blocks, "\n\n")
}

// plainInline is the text of a block's inline children with every mark dropped:
// emphasis, code spans and link syntax go, the words they wrapped stay.
func plainInline(node ast.Node, src []byte) string {
	var out strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch c := child.(type) {
		case *ast.Text:
			out.Write(c.Segment.Value(src))
			// A line break inside a paragraph is markdown's, not the prose's.
			if c.SoftLineBreak() || c.HardLineBreak() {
				out.WriteString(" ")
			}
		case *ast.String:
			out.Write(c.Value)
		case *ast.AutoLink:
			out.Write(c.URL(src))
		case *ast.RawHTML:
			// A stray tag is not prose; the text around it already came through.
		default:
			out.WriteString(plainInline(child, src))
		}
	}
	return out.String()
}

// rawBlockLines is the source of a block that carries no inline children, which
// is how code and HTML blocks keep their content.
func rawBlockLines(node ast.Node, src []byte) string {
	var out strings.Builder
	lines := node.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		out.Write(seg.Value(src))
	}
	return out.String()
}
