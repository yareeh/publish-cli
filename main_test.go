package main

import "testing"

func TestCleanCookie(t *testing.T) {
	cases := map[string]string{
		"abc123\n":                          "abc123",
		"substack.sid=abc123":               "abc123",
		"ajs_id=x; substack.sid=abc; foo=1": "abc",
		"other=1":                           "",
	}
	for in, want := range cases {
		if got := cleanCookie(in); got != want {
			t.Errorf("cleanCookie(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExistingDraft(t *testing.T) {
	pub := "https://jari.substack.com"
	fm := "tags:\n  - x\nsubstack_draft: https://jari.substack.com/publish/post/123"
	if id, ok := existingDraft(fm, pub); !ok || id != 123 {
		t.Errorf("id=%d ok=%v", id, ok)
	}
	if _, ok := existingDraft(fm, "https://other.substack.com"); ok {
		t.Error("draft from another publication should be ignored")
	}
	if _, ok := existingDraft("tags: []", pub); ok {
		t.Error("no field should mean no draft")
	}
}

func TestFlags(t *testing.T) {
	args := []string{"login", "--publication", "https://x", "--dry-run"}
	if !hasFlag(&args, "--dry-run") || flagValue(&args, "--publication") != "https://x" {
		t.Fatal("flags not parsed")
	}
	if len(args) != 1 || args[0] != "login" {
		t.Errorf("remaining args = %v", args)
	}
}
