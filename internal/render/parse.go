// Package render converts CommonMark into the formats the platforms accept:
// plain text for the LinkedIn composer and ProseMirror JSON for Substack drafts.
package render

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

var md = goldmark.New(goldmark.WithExtensions(extension.Strikethrough))

func parse(source []byte) ast.Node {
	return md.Parser().Parse(text.NewReader(source))
}

// soleImage returns the image if a paragraph consists of nothing else.
func soleImage(p ast.Node, src []byte) *ast.Image {
	var img *ast.Image
	for c := p.FirstChild(); c != nil; c = c.NextSibling() {
		switch n := c.(type) {
		case *ast.Image:
			if img != nil {
				return nil
			}
			img = n
		case *ast.Text:
			if len(strings.TrimSpace(string(n.Segment.Value(src)))) != 0 {
				return nil
			}
		default:
			return nil
		}
	}
	return img
}

// plainText concatenates the literal text under a node, ignoring markup.
func plainText(n ast.Node, src []byte) string {
	var out []byte
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := c.(type) {
		case *ast.Text:
			out = append(out, t.Segment.Value(src)...)
		case *ast.String:
			out = append(out, t.Value...)
		}
		return ast.WalkContinue, nil
	})
	return string(out)
}
