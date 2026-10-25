package render

import (
	"fmt"
	"strings"

	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
)

// LinkedInLimit is the maximum length of a LinkedIn post's text.
const LinkedInLimit = 3000

// LinkedIn renders Markdown as the plain text the LinkedIn composer accepts.
// Formatting markers are dropped, links are written out as "text (url)" and
// images are left out, since they have to be attached by hand.
func LinkedIn(markdown string) string {
	src := []byte(markdown)
	var blocks []string
	for c := parse(src).FirstChild(); c != nil; c = c.NextSibling() {
		if b := liBlock(c, src, ""); b != "" {
			blocks = append(blocks, b)
		}
	}
	return strings.Join(blocks, "\n\n")
}

func liBlock(n ast.Node, src []byte, indent string) string {
	switch b := n.(type) {
	case *ast.Paragraph, *ast.TextBlock:
		if soleImage(b, src) != nil {
			return ""
		}
		return strings.TrimSpace(liInline(b, src))
	case *ast.Heading:
		return strings.TrimSpace(liInline(b, src))
	case *ast.ThematicBreak:
		return "* * *"
	case *ast.CodeBlock, *ast.FencedCodeBlock:
		return strings.TrimRight(linesOf(b, src), "\n")
	case *ast.Blockquote:
		var parts []string
		for c := b.FirstChild(); c != nil; c = c.NextSibling() {
			if s := liBlock(c, src, ""); s != "" {
				parts = append(parts, s)
			}
		}
		lines := strings.Split(strings.Join(parts, "\n\n"), "\n")
		for i, l := range lines {
			lines[i] = strings.TrimRight("> "+l, " ")
		}
		return strings.Join(lines, "\n")
	case *ast.List:
		var items []string
		num := b.Start
		for item := b.FirstChild(); item != nil; item = item.NextSibling() {
			marker := "• "
			if b.IsOrdered() {
				marker = fmt.Sprintf("%d. ", num)
				num++
			}
			var parts []string
			for c := item.FirstChild(); c != nil; c = c.NextSibling() {
				if s := liBlock(c, src, indent+"  "); s != "" {
					parts = append(parts, s)
				}
			}
			items = append(items, indent+marker+strings.Join(parts, "\n"))
		}
		return strings.Join(items, "\n")
	}
	return ""
}

func liInline(n ast.Node, src []byte) string {
	var sb strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch t := c.(type) {
		case *ast.Text:
			sb.Write(t.Segment.Value(src))
			if t.SoftLineBreak() || t.HardLineBreak() {
				sb.WriteByte('\n')
			}
		case *ast.String:
			sb.Write(t.Value)
		case *ast.CodeSpan:
			sb.WriteString(plainText(t, src))
		case *ast.Emphasis, *east.Strikethrough:
			sb.WriteString(liInline(t, src))
		case *ast.Link:
			label := liInline(t, src)
			dest := string(t.Destination)
			if label == "" || label == dest {
				sb.WriteString(dest)
			} else {
				fmt.Fprintf(&sb, "%s (%s)", label, dest)
			}
		case *ast.AutoLink:
			sb.Write(t.URL(src))
		case *ast.Image:
			// Images can't go into the composer as text.
		}
	}
	return sb.String()
}

func linesOf(n ast.Node, src []byte) string {
	var sb strings.Builder
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		sb.Write(seg.Value(src))
	}
	return sb.String()
}
