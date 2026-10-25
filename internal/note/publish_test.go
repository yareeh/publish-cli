package note

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

var oct25 = time.Date(2026, 10, 25, 15, 0, 0, 0, time.Local)

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestTags(t *testing.T) {
	cases := map[string][]string{
		"tags:\n  - a\n  - published/linkedin\nother: x": {"a", "published/linkedin"},
		"tags: [a, \"b\"]": {"a", "b"},
		"tags: a":          {"a"},
		"title: x":         nil,
	}
	for fm, want := range cases {
		if got := Tags(fm); !reflect.DeepEqual(got, want) {
			t.Errorf("Tags(%q) = %v, want %v", fm, got, want)
		}
	}
}

func TestAddTag(t *testing.T) {
	cases := []struct{ fm, want string }{
		{"tags:\n  - a\nx: 1", "tags:\n  - a\n  - published/substack\nx: 1"},
		{"tags: [a]\nx: 1", "x: 1"}, // checked separately below
		{"", "tags:\n  - published/substack"},
		{"x: 1", "x: 1\ntags:\n  - published/substack"},
	}
	for i, c := range cases {
		got := addTag(c.fm, Tags(c.fm), "published/substack")
		if i == 1 {
			if !reflect.DeepEqual(Tags(got), []string{"a", "published/substack"}) {
				t.Errorf("inline list: %q", got)
			}
			continue
		}
		if got != c.want {
			t.Errorf("addTag(%q) = %q, want %q", c.fm, got, c.want)
		}
	}
}

func TestMarkPublishedFirstTimeRenamesToToday(t *testing.T) {
	root := writeVault(t, map[string]string{
		"Writing/2026Q3/09-30-02 Post.md":  "---\ntags:\n  - draft\n---\nBody ![[09-30-02 Post.png]]\n",
		"Writing/2026Q3/09-30-02 Post.png": "png",
		"Writing/2026Q4/10-25-01 Other.md": "x",
		"Writing/2026Q4/10-25-03.md":       "x",
		"Writing/2026Q4/10-25-02 Old.jpeg": "x",
		"Diary/2026-10-25.md":              "See [[09-30-02 Post]] and [[Writing/2026Q3/09-30-02 Post|alias]] and [[09-30-02 Post 2]].",
	})
	old := filepath.Join(root, "Writing/2026Q3/09-30-02 Post.md")
	newPath, err := MarkPublished(old, "linkedin", oct25)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "Writing/2026Q4/10-25-04 Post.md")
	if newPath != want {
		t.Fatalf("newPath = %s, want %s", newPath, want)
	}
	if got := read(t, newPath); got != "---\ntags:\n  - draft\n  - published/linkedin\n---\nBody ![[10-25-04 Post.png]]\n" {
		t.Errorf("note = %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, "Writing/2026Q4/10-25-04 Post.png")); err != nil {
		t.Error("attachment not moved:", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("old note still exists")
	}
	diary := read(t, filepath.Join(root, "Diary/2026-10-25.md"))
	if diary != "See [[10-25-04 Post]] and [[10-25-04 Post|alias]] and [[09-30-02 Post 2]]." {
		t.Errorf("links = %q", diary)
	}
}

func TestMarkPublishedKeepsTodaysDate(t *testing.T) {
	root := writeVault(t, map[string]string{
		"Writing/2026Q4/10-25-01 Post.md": "Body\n",
		"Writing/2026Q4/10-25-05 X.md":    "x",
	})
	p := filepath.Join(root, "Writing/2026Q4/10-25-01 Post.md")
	got, err := MarkPublished(p, "substack", oct25)
	if err != nil || got != p {
		t.Fatalf("got %s, err %v", got, err)
	}
	if read(t, p) != "---\ntags:\n  - published/substack\n---\nBody\n" {
		t.Errorf("note = %q", read(t, p))
	}
}

func TestMarkPublishedSecondPlatformDoesNotRename(t *testing.T) {
	root := writeVault(t, map[string]string{
		"Writing/2026Q3/09-30-01 Post.md": "---\ntags:\n  - published/linkedin\n---\nBody\n",
	})
	p := filepath.Join(root, "Writing/2026Q3/09-30-01 Post.md")
	got, err := MarkPublished(p, "substack", oct25)
	if err != nil || got != p {
		t.Fatalf("got %s, err %v", got, err)
	}
	if !reflect.DeepEqual(Tags(mustFrontmatter(t, p)), []string{"published/linkedin", "published/substack"}) {
		t.Errorf("tags = %v", Tags(mustFrontmatter(t, p)))
	}
	// Already tagged: nothing changes.
	before := read(t, p)
	if got, _ := MarkPublished(p, "substack", oct25); got != p || read(t, p) != before {
		t.Error("re-marking changed the note")
	}
}

func mustFrontmatter(t *testing.T, p string) string {
	fm, _ := SplitFrontmatter(read(t, p))
	return fm
}
