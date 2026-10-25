package render

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"reflect"
	"strings"

	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
)

// Node is a ProseMirror node in Substack's editor schema. The schema is
// undocumented; the shapes here follow python-substack's substack/nodes.py.
type Node struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Content []Node         `json:"content,omitempty"`
	Text    string         `json:"text,omitempty"`
	Marks   []Mark         `json:"marks,omitempty"`
}

type Mark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// ImageUploader stores a local image and returns the URL to reference it by.
type ImageUploader func(path string) (string, error)

// ProseMirror renders Markdown as a Substack draft body. Local images are
// passed to upload; images with an http(s) URL are referenced as they are.
func ProseMirror(markdown string, upload ImageUploader) (Node, error) {
	src := []byte(markdown)
	doc := Node{Type: "doc", Content: []Node{}}
	for c := parse(src).FirstChild(); c != nil; c = c.NextSibling() {
		nodes, err := pmBlock(c, src, upload)
		if err != nil {
			return Node{}, err
		}
		doc.Content = append(doc.Content, nodes...)
	}
	return doc, nil
}

func pmBlock(n ast.Node, src []byte, upload ImageUploader) ([]Node, error) {
	switch b := n.(type) {
	case *ast.Paragraph, *ast.TextBlock:
		if img := soleImage(b, src); img != nil {
			node, err := captionedImage(img, src, upload)
			if err != nil {
				return nil, err
			}
			return []Node{node}, nil
		}
		content := pmInline(b, src, nil)
		if len(content) == 0 {
			return nil, nil
		}
		return []Node{{Type: "paragraph", Content: content}}, nil
	case *ast.Heading:
		return []Node{{Type: "heading", Attrs: map[string]any{"level": b.Level}, Content: pmInline(b, src, nil)}}, nil
	case *ast.ThematicBreak:
		return []Node{{Type: "horizontal_rule"}}, nil
	case *ast.CodeBlock, *ast.FencedCodeBlock:
		node := Node{Type: "codeBlock", Content: []Node{{Type: "text", Text: strings.TrimRight(linesOf(b, src), "\n")}}}
		if f, ok := b.(*ast.FencedCodeBlock); ok {
			if lang := string(f.Language(src)); lang != "" {
				node.Attrs = map[string]any{"language": lang}
			}
		}
		if node.Content[0].Text == "" {
			return nil, nil
		}
		return []Node{node}, nil
	case *ast.Blockquote:
		content, err := pmChildren(b, src, upload)
		if err != nil || len(content) == 0 {
			return nil, err
		}
		return []Node{{Type: "blockquote", Content: content}}, nil
	case *ast.List:
		typ := "bullet_list"
		if b.IsOrdered() {
			typ = "ordered_list"
		}
		list := Node{Type: typ}
		for item := b.FirstChild(); item != nil; item = item.NextSibling() {
			content, err := pmChildren(item, src, upload)
			if err != nil {
				return nil, err
			}
			if len(content) == 0 {
				content = []Node{{Type: "paragraph"}}
			}
			list.Content = append(list.Content, Node{Type: "list_item", Content: content})
		}
		return []Node{list}, nil
	}
	return nil, nil
}

func pmChildren(n ast.Node, src []byte, upload ImageUploader) ([]Node, error) {
	var out []Node
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		nodes, err := pmBlock(c, src, upload)
		if err != nil {
			return nil, err
		}
		out = append(out, nodes...)
	}
	return out, nil
}

func pmInline(n ast.Node, src []byte, marks []Mark) []Node {
	var out []Node
	add := func(s string, m []Mark) {
		if s == "" {
			return
		}
		// Adjacent text with the same marks becomes one node.
		if k := len(out) - 1; k >= 0 && out[k].Type == "text" && reflect.DeepEqual(out[k].Marks, m) {
			out[k].Text += s
			return
		}
		out = append(out, Node{Type: "text", Text: s, Marks: m})
	}
	with := func(m Mark) []Mark {
		return append(append([]Mark{}, marks...), m)
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch t := c.(type) {
		case *ast.Text:
			add(string(t.Segment.Value(src)), marks)
			if t.SoftLineBreak() || t.HardLineBreak() {
				add(" ", marks)
			}
		case *ast.String:
			add(string(t.Value), marks)
		case *ast.CodeSpan:
			add(plainText(t, src), with(Mark{Type: "code"}))
		case *ast.Emphasis:
			m := Mark{Type: "em"}
			if t.Level >= 2 {
				m = Mark{Type: "strong"}
			}
			for _, sub := range pmInline(t, src, with(m)) {
				add(sub.Text, sub.Marks)
			}
		case *east.Strikethrough:
			for _, sub := range pmInline(t, src, with(Mark{Type: "strikethrough"})) {
				add(sub.Text, sub.Marks)
			}
		case *ast.Link:
			link := Mark{Type: "link", Attrs: map[string]any{"href": string(t.Destination)}}
			for _, sub := range pmInline(t, src, with(link)) {
				add(sub.Text, sub.Marks)
			}
		case *ast.AutoLink:
			url := string(t.URL(src))
			add(url, with(Mark{Type: "link", Attrs: map[string]any{"href": url}}))
		case *ast.Image:
			// An image inside running text has no place in the schema; keep its alt text.
			add(plainText(t, src), marks)
		}
	}
	// Trailing soft-break spaces would show as stray whitespace.
	if k := len(out) - 1; k >= 0 && out[k].Type == "text" {
		out[k].Text = strings.TrimRight(out[k].Text, " ")
		if out[k].Text == "" {
			out = out[:k]
		}
	}
	return out
}

func captionedImage(img *ast.Image, src []byte, upload ImageUploader) (Node, error) {
	url := string(img.Destination)
	// Without real dimensions the editor falls back to python-substack's defaults.
	width, height, size := any(1456), any(819), any(nil)
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		if w, h, n, ok := imageInfo(url); ok {
			width, height, size = w, h, n
		}
		var err error
		if url, err = upload(url); err != nil {
			return Node{}, err
		}
	}
	var alt any
	if s := plainText(img, src); s != "" {
		alt = s
	}
	content := []Node{{
		Type: "image2",
		Attrs: map[string]any{
			"src": url, "srcNoWatermark": nil, "fullscreen": false, "imageSize": "normal",
			"height": height, "width": width, "resizeWidth": 728, "bytes": size, "alt": alt,
			"title": nil, "type": nil, "href": nil, "belowTheFold": false, "topImage": false,
			"internalRedirect": nil, "isProcessing": false, "align": nil, "offset": false,
		},
	}}
	if title := string(img.Title); title != "" {
		content = append(content, Node{Type: "caption", Content: []Node{{Type: "text", Text: title}}})
	}
	return Node{Type: "captionedImage", Content: content}, nil
}

func imageInfo(path string) (width, height int, size int64, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, 0, false
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, 0, false
	}
	info, err := f.Stat()
	if err != nil {
		return 0, 0, 0, false
	}
	return cfg.Width, cfg.Height, info.Size(), true
}
