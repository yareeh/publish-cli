package substack

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type recorded struct {
	method, path, contentType, cookie string
	body                              []byte
}

func server(t *testing.T, handler func(r recorded) (int, string)) (*Client, *[]recorded) {
	t.Helper()
	var calls []recorded
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		cookie, _ := r.Cookie("substack.sid")
		rec := recorded{r.Method, r.URL.Path, r.Header.Get("Content-Type"), "", body}
		if cookie != nil {
			rec.cookie = cookie.Value
		}
		calls = append(calls, rec)
		status, resp := handler(rec)
		w.WriteHeader(status)
		io.WriteString(w, resp)
	}))
	t.Cleanup(srv.Close)
	c := New("secret-sid", srv.URL+"/")
	c.BaseURL = srv.URL + "/api/v1"
	return c, &calls
}

func TestProfilePrimaryPublication(t *testing.T) {
	c, calls := server(t, func(recorded) (int, string) {
		return 200, `{"id":42,"name":"J","publicationUsers":[
			{"is_primary":false,"publication":{"subdomain":"other"}},
			{"is_primary":true,"publication":{"subdomain":"jari","custom_domain":"blog.example","custom_domain_optional":false}}]}`
	})
	p, err := c.Profile()
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != 42 || p.Primary().URL() != "https://blog.example" {
		t.Errorf("profile = %+v, primary = %s", p, p.Primary().URL())
	}
	if (*calls)[0].path != "/api/v1/user/profile/self" || (*calls)[0].cookie != "secret-sid" {
		t.Errorf("call = %+v", (*calls)[0])
	}
	if (Publication{Subdomain: "x", CustomDomain: "y.z", CustomDomainOptional: true}).URL() != "https://x.substack.com" {
		t.Error("optional custom domain should fall back to subdomain")
	}
}

func TestCreateAndUpdateDraft(t *testing.T) {
	c, calls := server(t, func(r recorded) (int, string) {
		if r.method == "POST" {
			return 200, `{"id":777}`
		}
		return 200, `{}`
	})
	d := Draft{Title: "Otsikko", Body: map[string]any{"type": "doc", "content": []any{}}, UserID: 42}
	id, err := c.CreateDraft(d)
	if err != nil || id != 777 {
		t.Fatalf("id=%d err=%v", id, err)
	}
	if err := c.UpdateDraft(id, d); err != nil {
		t.Fatal(err)
	}
	post, put := (*calls)[0], (*calls)[1]
	if post.path != "/api/v1/drafts" || put.method != "PUT" || put.path != "/api/v1/drafts/777" {
		t.Errorf("calls = %+v", *calls)
	}
	var payload map[string]any
	if err := json.Unmarshal(post.body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["draft_title"] != "Otsikko" || payload["draft_body"] != `{"content":[],"type":"doc"}` {
		t.Errorf("payload = %v", payload)
	}
	bylines := payload["draft_bylines"].([]any)[0].(map[string]any)
	if bylines["id"] != float64(42) {
		t.Errorf("bylines = %v", bylines)
	}
	if !strings.HasSuffix(c.DraftURL(777), "/publish/post/777") {
		t.Errorf("DraftURL = %s", c.DraftURL(777))
	}
}

func TestUploadImage(t *testing.T) {
	c, calls := server(t, func(recorded) (int, string) { return 200, `{"url":"https://cdn/x.png"}` })
	p := filepath.Join(t.TempDir(), "a.png")
	os.WriteFile(p, []byte("PNGDATA"), 0644)
	u, err := c.UploadImage(p)
	if err != nil || u != "https://cdn/x.png" {
		t.Fatalf("u=%q err=%v", u, err)
	}
	form, _ := url.ParseQuery(string((*calls)[0].body))
	if form.Get("image") != "data:image/png;base64,UE5HREFUQQ==" || (*calls)[0].path != "/api/v1/image" {
		t.Errorf("upload call = %+v", (*calls)[0])
	}
}

func TestUnauthorized(t *testing.T) {
	c, _ := server(t, func(recorded) (int, string) { return 401, `{"error":"Not authorized"}` })
	if _, err := c.Profile(); err != ErrUnauthorized {
		t.Errorf("err = %v", err)
	}
}
