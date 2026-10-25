package render

import (
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = `## Otsikko

Tämä on **lihavoitu**, *kursiivi* ja ~~yliviivattu~~ sekä ` + "`koodi`" + `.
Toinen rivi [linkillä](https://example.com) ja <https://auto.example>.

- yksi
- kaksi
  - sisennetty

3. kolmas
4. neljäs

> Lainaus

---

` + "```go\nfmt.Println(1)\n```\n"

func TestLinkedIn(t *testing.T) {
	got := LinkedIn(sample)
	want := `Otsikko

Tämä on lihavoitu, kursiivi ja yliviivattu sekä koodi.
Toinen rivi linkillä (https://example.com) ja https://auto.example.

• yksi
• kaksi
  • sisennetty

3. kolmas
4. neljäs

> Lainaus

* * *

fmt.Println(1)`
	if got != want {
		t.Errorf("LinkedIn output mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestLinkedInDropsImagesAndRawLinks(t *testing.T) {
	got := LinkedIn("Before\n\n![](</tmp/x y.png>)\n\n[https://a.b](https://a.b)\n")
	if got != "Before\n\nhttps://a.b" {
		t.Errorf("got %q", got)
	}
}

func TestLinkedInHTML(t *testing.T) {
	got := LinkedInHTML("Tänään <b>\nrivi\n\nToinen & kolmas")
	want := `<meta charset="utf-8"><p>Tänään &lt;b&gt;<br>rivi</p><p><br></p><p>Toinen &amp; kolmas</p>`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func toJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestProseMirrorBlocksAndMarks(t *testing.T) {
	doc, err := ProseMirror(sample, nil)
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, n := range doc.Content {
		types = append(types, n.Type)
	}
	wantTypes := "heading paragraph bullet_list ordered_list blockquote horizontal_rule codeBlock"
	if strings.Join(types, " ") != wantTypes {
		t.Errorf("block types = %v", types)
	}

	para := toJSON(t, doc.Content[1])
	for _, want := range []string{
		`{"type":"text","text":"lihavoitu","marks":[{"type":"strong"}]}`,
		`{"type":"text","text":"kursiivi","marks":[{"type":"em"}]}`,
		`{"type":"text","text":"yliviivattu","marks":[{"type":"strikethrough"}]}`,
		`{"type":"text","text":"koodi","marks":[{"type":"code"}]}`,
		`{"type":"text","text":"linkillä","marks":[{"type":"link","attrs":{"href":"https://example.com"}}]}`,
		// The soft line break becomes a space inside the same text node.
		`"text":". Toinen rivi "`,
	} {
		if !strings.Contains(para, want) {
			t.Errorf("paragraph missing %s\n%s", want, para)
		}
	}

	nested := toJSON(t, doc.Content[2])
	if !strings.Contains(nested, `"type":"list_item","content":[{"type":"paragraph","content":[{"type":"text","text":"kaksi"}]},{"type":"bullet_list"`) {
		t.Errorf("nested list not kept: %s", nested)
	}
	if got := toJSON(t, doc.Content[6]); got != `{"type":"codeBlock","attrs":{"language":"go"},"content":[{"type":"text","text":"fmt.Println(1)"}]}` {
		t.Errorf("code block = %s", got)
	}
}

func TestProseMirrorUploadsLocalImages(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "kuva 1.png")
	f, _ := os.Create(p)
	png.Encode(f, image.NewRGBA(image.Rect(0, 0, 40, 30)))
	f.Close()

	var uploaded []string
	upload := func(path string) (string, error) {
		uploaded = append(uploaded, path)
		return "https://cdn.example/img.png", nil
	}
	doc, err := ProseMirror("Teksti\n\n![Alt](<"+p+"> \"Kuvateksti\")\n\n![](https://remote/x.png)\n", upload)
	if err != nil {
		t.Fatal(err)
	}
	if len(uploaded) != 1 || uploaded[0] != p {
		t.Fatalf("uploaded = %v", uploaded)
	}
	img := doc.Content[1]
	if img.Type != "captionedImage" || img.Content[0].Type != "image2" {
		t.Fatalf("image node = %s", toJSON(t, img))
	}
	a := img.Content[0].Attrs
	if a["src"] != "https://cdn.example/img.png" || a["width"] != 40 || a["height"] != 30 || a["alt"] != "Alt" {
		t.Errorf("image attrs = %v", a)
	}
	if img.Content[1].Type != "caption" || img.Content[1].Content[0].Text != "Kuvateksti" {
		t.Errorf("caption = %s", toJSON(t, img.Content[1]))
	}
	if doc.Content[2].Content[0].Attrs["src"] != "https://remote/x.png" {
		t.Errorf("remote image = %s", toJSON(t, doc.Content[2]))
	}
}

func TestProseMirrorUploadError(t *testing.T) {
	_, err := ProseMirror("![](/tmp/a.png)\n", func(string) (string, error) { return "", errors.New("boom") })
	if err == nil || err.Error() != "boom" {
		t.Errorf("err = %v", err)
	}
}
