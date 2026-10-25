// Package note reads an Obsidian Writing note and turns its Obsidian-specific
// syntax into plain CommonMark that the renderers can parse.
package note

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/yareeh/publish-cli/internal/render"
)

type Note struct {
	Path        string
	Title       string
	Frontmatter string   // raw YAML between the --- fences, without them
	TitleFromH1 bool     // Title is the body's leading H1, not the file name
	Subtitle    string   // an H2 opening the body (after any H1), as plain text
	Body        string   // CommonMark with Obsidian syntax resolved, headings included
	Images      []string // absolute paths of embedded images, in order
	postBody    string
}

// PostBody is the body without the leading H1 and the subtitle, for
// platforms that show the title and subtitle separately.
func (n *Note) PostBody() string {
	return n.postBody
}

// Untitled is true for a note still named only "MM-DD-NN.md".
func (n *Note) Untitled() bool {
	return untitledRe.MatchString(n.Title)
}

var untitledRe = regexp.MustCompile(`^\d{2}-\d{2}-\d{2}$`)

// Writing notes are named "MM-DD-NN Title.md"; the title is what follows the prefix.
var datePrefix = regexp.MustCompile(`^\d{2}-\d{2}-\d{2} `)

var (
	commentRe   = regexp.MustCompile(`(?s)%%.*?%%`)
	highlightRe = regexp.MustCompile(`==([^=\n]+)==`)
	embedRe     = regexp.MustCompile(`!\[\[([^\]|]+)(?:\|[^\]]*)?\]\]`)
	wikiLinkRe  = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]*))?\]\]`)
)

var imageExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true}

func Load(path string) (*Note, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	fm, body := SplitFrontmatter(string(data))
	n := &Note{Path: abs, Title: TitleFromPath(abs), Frontmatter: fm}
	n.Body, n.Images, err = resolve(body, filepath.Dir(abs), VaultRoot(abs))
	if err != nil {
		return nil, err
	}
	n.postBody = n.Body
	// A note that starts with its own H1 uses that as the title instead, and
	// an H2 that opens the body, after the H1 if there is one, is the
	// subtitle. Both are kept as plain text.
	if h1, rest, ok := leadingHeading(n.Body, "# "); ok {
		n.Title, n.TitleFromH1, n.postBody = render.LinkedIn(h1), true, rest
	}
	if h2, rest, ok := leadingHeading(n.postBody, "## "); ok {
		n.Subtitle, n.postBody = render.LinkedIn(h2), rest
	}
	return n, nil
}

func TitleFromPath(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return datePrefix.ReplaceAllString(name, "")
}

// SplitFrontmatter separates a leading YAML block from the body.
func SplitFrontmatter(s string) (frontmatter, body string) {
	if !strings.HasPrefix(s, "---\n") {
		return "", s
	}
	end := strings.Index(s[4:], "\n---")
	if end < 0 {
		return "", s
	}
	fm := s[4 : 4+end]
	rest := s[4+end+4:]
	rest = strings.TrimPrefix(rest, "\n")
	return fm, rest
}

// VaultRoot is the nearest ancestor directory that holds .obsidian, or the
// note's own directory when there is none.
func VaultRoot(path string) string {
	dir := filepath.Dir(path)
	for d := dir; ; d = filepath.Dir(d) {
		if info, err := os.Stat(filepath.Join(d, ".obsidian")); err == nil && info.IsDir() {
			return d
		}
		if filepath.Dir(d) == d {
			return dir
		}
	}
}

func leadingHeading(body, marker string) (heading, rest string, ok bool) {
	trimmed := strings.TrimLeft(body, "\n")
	line, after, _ := strings.Cut(trimmed, "\n")
	if !strings.HasPrefix(line, marker) {
		return "", body, false
	}
	return strings.TrimSpace(line[len(marker):]), strings.TrimLeft(after, "\n"), true
}

func resolve(body, noteDir, vault string) (string, []string, error) {
	body = commentRe.ReplaceAllString(body, "")
	body = highlightRe.ReplaceAllString(body, "$1")

	var images []string
	var resolveErr error
	body = embedRe.ReplaceAllStringFunc(body, func(m string) string {
		name := strings.TrimSpace(embedRe.FindStringSubmatch(m)[1])
		if !imageExts[strings.ToLower(filepath.Ext(name))] {
			return "" // embedded notes, PDFs etc. have no counterpart in a post
		}
		p, err := findFile(name, noteDir, vault)
		if err != nil {
			resolveErr = err
			return m
		}
		images = append(images, p)
		// The image goes in its own paragraph so renderers treat it as a block.
		return "\n\n![](<" + p + ">)\n\n"
	})
	if resolveErr != nil {
		return "", nil, resolveErr
	}

	body = wikiLinkRe.ReplaceAllStringFunc(body, func(m string) string {
		g := wikiLinkRe.FindStringSubmatch(m)
		if g[2] != "" {
			return g[2]
		}
		target, _, _ := strings.Cut(g[1], "#")
		return filepath.Base(target)
	})

	// Standard Markdown images pointing at local files are collected too.
	for _, g := range mdImageRe.FindAllStringSubmatch(body, -1) {
		src := strings.Trim(g[1], "<>")
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
			continue
		}
		if !slices.Contains(images, src) {
			images = append(images, src)
		}
	}
	return body, images, nil
}

var mdImageRe = regexp.MustCompile(`!\[[^\]]*\]\((<[^>]+>|[^)\s]+)`)

func findFile(name, noteDir, vault string) (string, error) {
	if p := filepath.Join(noteDir, name); exists(p) {
		return p, nil
	}
	var found string
	_ = filepath.WalkDir(vault, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && p != vault {
			return filepath.SkipDir
		}
		if !d.IsDir() && (d.Name() == filepath.Base(name)) && strings.HasSuffix(p, name) {
			found = p
			return fs.SkipAll
		}
		return nil
	})
	if found == "" {
		return "", fmt.Errorf("embedded image not found in vault: %s", name)
	}
	return found, nil
}

func exists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// SetField sets a top-level scalar field in the note's frontmatter, creating
// the frontmatter block if the file has none. Other lines are left as they are.
func SetField(path, key, value string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	fm, body := SplitFrontmatter(string(data))
	line := key + ": " + value
	var lines []string
	if fm != "" {
		lines = strings.Split(fm, "\n")
	}
	replaced := false
	for i, l := range lines {
		if strings.HasPrefix(l, key+":") {
			lines[i] = line
			replaced = true
		}
	}
	if !replaced {
		lines = append(lines, line)
	}
	out := "---\n" + strings.Join(lines, "\n") + "\n---\n" + body
	return os.WriteFile(path, []byte(out), 0644)
}

// Field returns a top-level scalar field from raw frontmatter.
func Field(frontmatter, key string) string {
	for l := range strings.SplitSeq(frontmatter, "\n") {
		if v, ok := strings.CutPrefix(l, key+":"); ok {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}
