// Package substack talks to Substack's private web API, the same one its
// editor uses. There is no official API; endpoints and payloads follow
// python-substack (github.com/ma2za/python-substack) and may change without notice.
package substack

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const DefaultBaseURL = "https://substack.com/api/v1"

type Client struct {
	BaseURL     string // account-level API, DefaultBaseURL in production
	Publication string // publication root, e.g. https://name.substack.com
	SID         string // value of the substack.sid session cookie
	HTTP        *http.Client
}

func New(sid, publication string) *Client {
	return &Client{
		BaseURL:     DefaultBaseURL,
		Publication: strings.TrimRight(publication, "/"),
		SID:         sid,
		HTTP:        &http.Client{Timeout: 60 * time.Second},
	}
}

type Publication struct {
	Subdomain            string `json:"subdomain"`
	CustomDomain         string `json:"custom_domain"`
	CustomDomainOptional bool   `json:"custom_domain_optional"`
	Name                 string `json:"name"`
}

// URL is the address the publication is served at.
func (p Publication) URL() string {
	if p.CustomDomain != "" && !p.CustomDomainOptional {
		return "https://" + p.CustomDomain
	}
	return "https://" + p.Subdomain + ".substack.com"
}

type Profile struct {
	ID                 int          `json:"id"`
	Name               string       `json:"name"`
	PrimaryPublication *Publication `json:"primaryPublication"`
	PublicationUsers   []struct {
		IsPrimary   bool         `json:"is_primary"`
		Publication *Publication `json:"publication"`
	} `json:"publicationUsers"`
}

// Primary returns the user's primary publication, or the first one they belong to.
func (p Profile) Primary() *Publication {
	if p.PrimaryPublication != nil {
		return p.PrimaryPublication
	}
	for _, pu := range p.PublicationUsers {
		if pu.IsPrimary && pu.Publication != nil {
			return pu.Publication
		}
	}
	if len(p.PublicationUsers) > 0 {
		return p.PublicationUsers[0].Publication
	}
	return nil
}

func (c *Client) Profile() (*Profile, error) {
	var p Profile
	if err := c.do("GET", c.BaseURL+"/user/profile/self", nil, "", &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// UploadImage stores a local image on Substack's CDN and returns its URL.
func (c *Client) UploadImage(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	mt := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if mt == "" {
		mt = "application/octet-stream"
	}
	form := url.Values{"image": {"data:" + mt + ";base64," + base64.StdEncoding.EncodeToString(data)}}
	var resp struct {
		URL string `json:"url"`
	}
	if err := c.do("POST", c.Publication+"/api/v1/image", strings.NewReader(form.Encode()),
		"application/x-www-form-urlencoded", &resp); err != nil {
		return "", fmt.Errorf("uploading %s: %w", filepath.Base(path), err)
	}
	if !strings.HasPrefix(resp.URL, "http") {
		return "", fmt.Errorf("uploading %s: no image URL in response", filepath.Base(path))
	}
	return resp.URL, nil
}

type Draft struct {
	Title    string
	Subtitle string
	Body     any // ProseMirror document
	UserID   int
}

func (d Draft) payload() (map[string]any, error) {
	body, err := json.Marshal(d.Body)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"draft_title":               d.Title,
		"draft_subtitle":            d.Subtitle,
		"draft_body":                string(body), // the API wants the document as a JSON string
		"draft_bylines":             []map[string]any{{"id": d.UserID, "is_guest": false}},
		"audience":                  "everyone",
		"write_comment_permissions": "everyone",
		"section_chosen":            true,
		"draft_section_id":          nil,
	}, nil
}

// CreateDraft creates a new draft and returns its ID.
func (c *Client) CreateDraft(d Draft) (int, error) {
	p, err := d.payload()
	if err != nil {
		return 0, err
	}
	var resp struct {
		ID int `json:"id"`
	}
	if err := c.doJSON("POST", c.Publication+"/api/v1/drafts", p, &resp); err != nil {
		return 0, err
	}
	if resp.ID == 0 {
		return 0, fmt.Errorf("draft created but no id in response")
	}
	return resp.ID, nil
}

// UpdateDraft replaces the title and body of an existing draft.
func (c *Client) UpdateDraft(id int, d Draft) error {
	p, err := d.payload()
	if err != nil {
		return err
	}
	return c.doJSON("PUT", fmt.Sprintf("%s/api/v1/drafts/%d", c.Publication, id), p, nil)
}

// DraftURL is where the draft opens in Substack's editor.
func (c *Client) DraftURL(id int) string {
	return fmt.Sprintf("%s/publish/post/%d", c.Publication, id)
}

func (c *Client) doJSON(method, u string, in, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return c.do(method, u, bytes.NewReader(b), "application/json", out)
}

// ErrUnauthorized means the session cookie has expired or is wrong.
var ErrUnauthorized = fmt.Errorf("substack rejected the session cookie; run `publish-cli substack login` again")

func (c *Client) do(method, u string, body io.Reader, contentType string, out any) error {
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return err
	}
	req.AddCookie(&http.Cookie{Name: "substack.sid", Value: c.SID})
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s: %s", method, u, resp.Status, truncate(string(data), 300))
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s %s: unexpected response: %s", method, u, truncate(string(data), 300))
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
