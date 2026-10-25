package note

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeVault(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".obsidian"), 0755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestTitleFromPath(t *testing.T) {
	cases := map[string]string{
		"Writing/2026Q4/10-25-01 Banned From Utopia.md": "Banned From Utopia",
		"10-16-02.md":       "10-16-02",
		"Plain title.md":    "Plain title",
		"10-08-02 Korot.md": "Korot",
	}
	for in, want := range cases {
		if got := TitleFromPath(in); got != want {
			t.Errorf("TitleFromPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitFrontmatter(t *testing.T) {
	fm, body := SplitFrontmatter("---\ntags:\n  - published/substack\n---\nHello\n")
	if fm != "tags:\n  - published/substack" {
		t.Errorf("frontmatter = %q", fm)
	}
	if body != "Hello\n" {
		t.Errorf("body = %q", body)
	}
	fm, body = SplitFrontmatter("No frontmatter\n")
	if fm != "" || body != "No frontmatter\n" {
		t.Errorf("got %q / %q", fm, body)
	}
}

func TestLoadResolvesObsidianSyntax(t *testing.T) {
	root := writeVault(t, map[string]string{
		"Writing/2026Q4/10-25-01 Post.md": "---\ntags:\n  - x\n---\n" +
			"Intro with [[Some Note|alias]] and [[Other Note#Heading]] and ==marked==.\n" +
			"%%private remark%%\n" +
			"![[10-25-01 Post.png]]\n" +
			"![[shared.jpg|300]]\n" +
			"![[Embedded note]]\n",
		"Writing/2026Q4/10-25-01 Post.png": "png",
		"Attachments/shared.jpg":           "jpg",
	})
	n, err := Load(filepath.Join(root, "Writing/2026Q4/10-25-01 Post.md"))
	if err != nil {
		t.Fatal(err)
	}
	if n.Title != "Post" {
		t.Errorf("Title = %q", n.Title)
	}
	for _, want := range []string{"alias", "Other Note", "marked."} {
		if !strings.Contains(n.Body, want) {
			t.Errorf("body missing %q:\n%s", want, n.Body)
		}
	}
	for _, unwanted := range []string{"[[", "==", "private remark", "Embedded note"} {
		if strings.Contains(n.Body, unwanted) {
			t.Errorf("body still contains %q:\n%s", unwanted, n.Body)
		}
	}
	if len(n.Images) != 2 {
		t.Fatalf("Images = %v", n.Images)
	}
	if !strings.HasSuffix(n.Images[0], "Writing/2026Q4/10-25-01 Post.png") ||
		!strings.HasSuffix(n.Images[1], "Attachments/shared.jpg") {
		t.Errorf("Images = %v", n.Images)
	}
	if !strings.Contains(n.Body, "![](<"+n.Images[0]+">)") {
		t.Errorf("image not rewritten as Markdown image:\n%s", n.Body)
	}
}

func TestLoadMissingImageFails(t *testing.T) {
	root := writeVault(t, map[string]string{"a.md": "![[missing.png]]\n"})
	if _, err := Load(filepath.Join(root, "a.md")); err == nil {
		t.Fatal("expected error for missing image")
	}
}

func TestLoadUsesLeadingH1AsTitle(t *testing.T) {
	root := writeVault(t, map[string]string{"10-01-01 File name.md": "# Real title\n\nBody\n"})
	n, err := Load(filepath.Join(root, "10-01-01 File name.md"))
	if err != nil {
		t.Fatal(err)
	}
	if n.Title != "Real title" || strings.Contains(n.Body, "# Real title") {
		t.Errorf("Title = %q, Body = %q", n.Title, n.Body)
	}
}

func TestSetField(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "n.md")
	os.WriteFile(p, []byte("---\ntags:\n  - a\n---\nBody\n"), 0644)

	if err := SetField(p, "substack_draft", "https://x/1"); err != nil {
		t.Fatal(err)
	}
	if err := SetField(p, "substack_draft", "https://x/2"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	want := "---\ntags:\n  - a\nsubstack_draft: https://x/2\n---\nBody\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
	fm, _ := SplitFrontmatter(string(got))
	if Field(fm, "substack_draft") != "https://x/2" {
		t.Errorf("Field = %q", Field(fm, "substack_draft"))
	}

	p2 := filepath.Join(dir, "plain.md")
	os.WriteFile(p2, []byte("Body\n"), 0644)
	SetField(p2, "k", "v")
	got, _ = os.ReadFile(p2)
	if string(got) != "---\nk: v\n---\nBody\n" {
		t.Errorf("got %q", got)
	}
}
