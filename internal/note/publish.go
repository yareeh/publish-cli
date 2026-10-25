package note

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const publishedPrefix = "published/"

// Tags lists the tags in raw frontmatter, in block (`- tag`), inline
// (`[a, b]`) or single-value form.
func Tags(frontmatter string) []string {
	lines := strings.Split(frontmatter, "\n")
	for i, l := range lines {
		v, ok := strings.CutPrefix(l, "tags:")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if v == "" {
			var tags []string
			for _, item := range lines[i+1:] {
				t, ok := strings.CutPrefix(strings.TrimSpace(item), "- ")
				if !ok {
					break
				}
				tags = append(tags, strings.Trim(strings.TrimSpace(t), `"'`))
			}
			return tags
		}
		var tags []string
		for t := range strings.SplitSeq(strings.Trim(v, "[]"), ",") {
			if t = strings.Trim(strings.TrimSpace(t), `"'`); t != "" {
				tags = append(tags, t)
			}
		}
		return tags
	}
	return nil
}

// MarkPublished adds published/<platform> to the note's tags. The first time a
// note gets any published/ tag it is renamed to today's date (see
// renameForToday). It returns the note's path afterwards.
func MarkPublished(path, platform string, now time.Time) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return path, err
	}
	fm, body := SplitFrontmatter(string(data))
	tags := Tags(fm)
	tag := publishedPrefix + platform
	firstPublication := true
	for _, t := range tags {
		if t == tag {
			return path, nil
		}
		if strings.HasPrefix(t, publishedPrefix) {
			firstPublication = false
		}
	}
	out := "---\n" + addTag(fm, tags, tag) + "\n---\n" + body
	if err := os.WriteFile(path, []byte(out), 0644); err != nil {
		return path, err
	}
	if !firstPublication {
		return path, nil
	}
	return renameForToday(path, now)
}

func addTag(fm string, tags []string, tag string) string {
	lines := []string{}
	if fm != "" {
		lines = strings.Split(fm, "\n")
	}
	for i, l := range lines {
		if !strings.HasPrefix(l, "tags:") {
			continue
		}
		if strings.TrimSpace(strings.TrimPrefix(l, "tags:")) == "" {
			end := i + 1
			for end < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[end]), "- ") {
				end++
			}
			rest := append([]string{"  - " + tag}, lines[end:]...)
			return strings.Join(append(lines[:end:end], rest...), "\n")
		}
		// Rewrite an inline or single-value list as a block list.
		block := []string{"tags:"}
		for _, t := range append(tags, tag) {
			block = append(block, "  - "+t)
		}
		return strings.Join(append(append(lines[:i:i], block...), lines[i+1:]...), "\n")
	}
	return strings.Join(append(lines, "tags:", "  - "+tag), "\n")
}

var (
	notePrefixRe = regexp.MustCompile(`^(\d{2})-(\d{2})-(\d{2,})(.*)$`)
	quarterRe    = regexp.MustCompile(`^\d{4}Q[1-4]$`)
)

// renameForToday gives a Writing note today's date: MM-DD-NN where NN is one
// more than the highest ordinal already used today. A note in a YYYYQn folder
// moves to the current quarter's folder. Attachments sharing the note's name
// are renamed with it and wiki-links to them across the vault are updated.
// A note that already has today's date is left alone, as is one whose name
// doesn't start with a date.
func renameForToday(path string, now time.Time) (string, error) {
	dir, file := filepath.Split(path)
	dir = filepath.Clean(dir)
	oldBase := strings.TrimSuffix(file, filepath.Ext(file))
	m := notePrefixRe.FindStringSubmatch(oldBase)
	today := now.Format("01-02")
	if m == nil || m[1]+"-"+m[2] == today {
		return path, nil
	}

	newDir := dir
	if quarterRe.MatchString(filepath.Base(dir)) {
		newDir = filepath.Join(filepath.Dir(dir), fmt.Sprintf("%dQ%d", now.Year(), (int(now.Month())+2)/3))
		if err := os.MkdirAll(newDir, 0755); err != nil {
			return path, err
		}
	}
	ordinal, err := nextOrdinal(newDir, today)
	if err != nil {
		return path, err
	}
	newBase := fmt.Sprintf("%s-%02d%s", today, ordinal, m[4])

	// The note and its attachments: "MM-DD-NN Title.md", "MM-DD-NN Title.png"…
	entries, err := os.ReadDir(dir)
	if err != nil {
		return path, err
	}
	var newPath string
	for _, e := range entries {
		name := e.Name()
		ext := filepath.Ext(name)
		if e.IsDir() || strings.TrimSuffix(name, ext) != oldBase {
			continue
		}
		target := filepath.Join(newDir, newBase+ext)
		if _, err := os.Stat(target); err == nil {
			return path, fmt.Errorf("can't rename %s: %s already exists", name, target)
		}
		if err := os.Rename(filepath.Join(dir, name), target); err != nil {
			return path, err
		}
		if filepath.Join(dir, name) == path {
			newPath = target
		}
	}
	if err := updateLinks(VaultRoot(newPath), oldBase, newBase); err != nil {
		return newPath, fmt.Errorf("renamed, but updating links failed: %w", err)
	}
	return newPath, nil
}

func nextOrdinal(dir, monthDay string) (int, error) {
	re := regexp.MustCompile(`^` + monthDay + `-(\d{2,})(?:[ .].*)?$`)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	highest := 0
	for _, e := range entries {
		if m := re.FindStringSubmatch(e.Name()); m != nil {
			if n, _ := strconv.Atoi(m[1]); n > highest {
				highest = n
			}
		}
	}
	return highest + 1, nil
}

// updateLinks rewrites [[oldBase…]] wiki-links and embeds in the vault's notes.
func updateLinks(vault, oldBase, newBase string) error {
	re := regexp.MustCompile(`(\[\[)(?:[^\]|#\n]*/)?` + regexp.QuoteMeta(oldBase) + `((?:\.[A-Za-z0-9]+)?[\]|#])`)
	return filepath.WalkDir(vault, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && p != vault {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(p) != ".md" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		updated := re.ReplaceAllString(string(data), "${1}"+strings.ReplaceAll(newBase, "$", "$$")+"${2}")
		if updated == string(data) {
			return nil
		}
		return os.WriteFile(p, []byte(updated), 0644)
	})
}
