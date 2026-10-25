package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/yareeh/publish-cli/internal/config"
	"github.com/yareeh/publish-cli/internal/keychain"
	"github.com/yareeh/publish-cli/internal/note"
	"github.com/yareeh/publish-cli/internal/render"
	"github.com/yareeh/publish-cli/internal/substack"
)

const (
	linkedInComposer = "https://www.linkedin.com/feed/?shareActive=true"
	substackAccount  = "substack.sid"
	draftField       = "substack_draft"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}
	cmd, args := os.Args[1], os.Args[2:]

	var err error
	switch cmd {
	case "linkedin":
		err = doLinkedIn(args)
	case "substack":
		err = doSubstack(args)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`publish-cli — turn Obsidian Writing notes into LinkedIn and Substack drafts

Usage: publish-cli <command> [options]

Commands:
  linkedin <note.md>             Copy the note as LinkedIn text to the clipboard
                                 and open the LinkedIn composer
  substack login                 Save the Substack session cookie to the keychain
  substack logout                Remove the saved cookie
  substack draft <note.md>       Create a Substack draft from the note, or update
                                 the one recorded in its substack_draft field

Options:
  --print       linkedin: print the text instead of copying it and opening LinkedIn
  --dry-run     substack draft: print the draft payload without contacting Substack
  --new         substack draft: create a new draft even if the note has one
  --no-open     don't open the browser
  --publication <url>  substack login: use this publication instead of the primary one

Examples:
  publish-cli linkedin "Writing/2026Q4/10-25-01 Banned From Utopia.md"
  publish-cli substack login
  publish-cli substack draft "Writing/2026Q4/10-25-02 Luontokatsaus.md"
`)
}

func doLinkedIn(args []string) error {
	printOnly := hasFlag(&args, "--print")
	noOpen := hasFlag(&args, "--no-open")
	if len(args) != 1 {
		return errors.New("usage: publish-cli linkedin <note.md>")
	}
	n, err := note.Load(args[0])
	if err != nil {
		return err
	}
	text := render.LinkedIn(n.Body)
	// Without an H1 the post opens with the title from the file name.
	if !n.TitleFromH1 && !n.Untitled() {
		text = n.Title + "\n\n" + text
	}
	if printOnly {
		fmt.Println(text)
		return nil
	}

	if err := copyToClipboard(text, render.LinkedInHTML(text)); err != nil {
		return err
	}
	count := len([]rune(text))
	fmt.Printf("Copied %q to the clipboard (%d characters).\n", n.Title, count)
	if count > render.LinkedInLimit {
		fmt.Printf("Warning: LinkedIn posts are limited to %d characters.\n", render.LinkedInLimit)
	}
	if n, err = markPublished(n, "linkedin"); err != nil {
		return err
	}
	for _, img := range n.Images {
		fmt.Println("Attach image:", img)
	}
	if noOpen {
		return nil
	}
	if len(n.Images) > 0 {
		// Show the image in Finder so it can be dragged into the composer.
		_ = exec.Command("/usr/bin/open", "-R", n.Images[0]).Run()
	}
	return openURL(linkedInComposer)
}

func doSubstack(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: publish-cli substack <login|logout|draft>")
	}
	switch args[0] {
	case "login":
		return doSubstackLogin(args[1:])
	case "logout":
		if err := keychain.Delete(substackAccount); err != nil {
			return errors.New("no saved Substack cookie")
		}
		fmt.Println("Removed the Substack cookie from the keychain.")
		return nil
	case "draft":
		return doSubstackDraft(args[1:])
	}
	return fmt.Errorf("unknown substack command: %s", args[0])
}

func doSubstackLogin(args []string) error {
	pubFlag := flagValue(&args, "--publication")
	fmt.Print(`Log in to substack.com in your browser, then open the developer tools:
  Application (Chrome) / Storage (Firefox, Safari) → Cookies → https://substack.com
and copy the value of the "substack.sid" cookie.

`)
	sid, err := readSecret("substack.sid: ")
	if err != nil {
		return err
	}
	sid = cleanCookie(sid)
	if sid == "" {
		return errors.New("no cookie given")
	}

	client := substack.New(sid, "")
	profile, err := client.Profile()
	if err != nil {
		return err
	}
	pub := strings.TrimRight(pubFlag, "/")
	if pub == "" {
		primary := profile.Primary()
		if primary == nil {
			return errors.New("no publication found on this account; pass --publication <url>")
		}
		pub = primary.URL()
	}

	if err := keychain.Set(substackAccount, sid); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.SubstackPublication, cfg.SubstackUserID = pub, profile.ID
	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("Logged in as %s. Drafts will go to %s.\n", profile.Name, pub)
	return nil
}

func doSubstackDraft(args []string) error {
	dryRun := hasFlag(&args, "--dry-run")
	forceNew := hasFlag(&args, "--new")
	noOpen := hasFlag(&args, "--no-open")
	if len(args) != 1 {
		return errors.New("usage: publish-cli substack draft <note.md>")
	}
	n, err := note.Load(args[0])
	if err != nil {
		return err
	}

	if dryRun {
		doc, err := render.ProseMirror(n.PostBody(), func(p string) (string, error) { return "file://" + p, nil })
		if err != nil {
			return err
		}
		out, _ := json.MarshalIndent(map[string]any{"draft_title": n.Title, "draft_subtitle": n.Subtitle, "draft_body": doc}, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	sid, err := keychain.Get(substackAccount)
	if err != nil || cfg.SubstackPublication == "" {
		return errors.New("not logged in; run `publish-cli substack login` first")
	}
	client := substack.New(sid, cfg.SubstackPublication)

	doc, err := render.ProseMirror(n.PostBody(), func(p string) (string, error) {
		fmt.Println("Uploading", p)
		return client.UploadImage(p)
	})
	if err != nil {
		return err
	}
	draft := substack.Draft{Title: n.Title, Subtitle: n.Subtitle, Body: doc, UserID: cfg.SubstackUserID}

	if id, ok := existingDraft(n.Frontmatter, cfg.SubstackPublication); ok && !forceNew {
		if err := client.UpdateDraft(id, draft); err != nil {
			return fmt.Errorf("updating draft %d (use --new for a fresh one): %w", id, err)
		}
		fmt.Println("Updated draft:", client.DraftURL(id))
		if _, err := markPublished(n, "substack"); err != nil {
			return err
		}
		return maybeOpen(client.DraftURL(id), noOpen)
	}

	id, err := client.CreateDraft(draft)
	if err != nil {
		return err
	}
	url := client.DraftURL(id)
	if err := note.SetField(n.Path, draftField, url); err != nil {
		fmt.Fprintln(os.Stderr, "Warning: could not record the draft in the note:", err)
	}
	fmt.Println("Created draft:", url)
	if _, err := markPublished(n, "substack"); err != nil {
		return err
	}
	return maybeOpen(url, noOpen)
}

// markPublished tags the note published/<platform>, which on its first
// publication also renames it to today's date. It returns the reloaded note.
func markPublished(n *note.Note, platform string) (*note.Note, error) {
	newPath, err := note.MarkPublished(n.Path, platform, time.Now())
	if err != nil {
		return n, fmt.Errorf("tagging the note: %w", err)
	}
	if newPath != n.Path {
		// The Obsidian plugin reads this line to open the renamed note.
		fmt.Println("Renamed to:", newPath)
	}
	return note.Load(newPath)
}

var draftIDRe = regexp.MustCompile(`/publish/post/(\d+)`)

// existingDraft finds the draft recorded in the note, if it belongs to the
// current publication.
func existingDraft(frontmatter, publication string) (int, bool) {
	u := note.Field(frontmatter, draftField)
	if u == "" || !strings.HasPrefix(u, publication+"/") {
		return 0, false
	}
	m := draftIDRe.FindStringSubmatch(u)
	if m == nil {
		return 0, false
	}
	id, err := strconv.Atoi(m[1])
	return id, err == nil
}

// cleanCookie accepts the bare value, "substack.sid=value" or a whole Cookie header.
func cleanCookie(s string) string {
	s = strings.TrimSpace(s)
	for part := range strings.SplitSeq(s, ";") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(part), "substack.sid="); ok {
			return v
		}
	}
	if strings.Contains(s, "=") {
		return ""
	}
	return s
}

func readSecret(prompt string) (string, error) {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		fmt.Print(prompt)
		b, err := term.ReadPassword(fd)
		fmt.Println()
		return string(b), err
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return line, nil
}

// clipboardScript puts both a plain-text and an HTML version on the pasteboard.
// pbcopy can't do HTML, and without LANG (as when Obsidian runs us) it reads
// its input as MacRoman and garbles non-ASCII text.
const clipboardScript = `ObjC.import('AppKit');
const env = $.NSProcessInfo.processInfo.environment;
const pb = $.NSPasteboard.generalPasteboard;
pb.clearContents;
pb.setStringForType(env.objectForKey('PUBLISH_CLI_TEXT'), 'public.utf8-plain-text');
pb.setStringForType(env.objectForKey('PUBLISH_CLI_HTML'), 'public.html');`

func copyToClipboard(text, html string) error {
	cmd := exec.Command("/usr/bin/osascript", "-l", "JavaScript", "-e", clipboardScript)
	cmd.Env = append(os.Environ(), "PUBLISH_CLI_TEXT="+text, "PUBLISH_CLI_HTML="+html)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("copying to clipboard: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func maybeOpen(url string, noOpen bool) error {
	if noOpen {
		return nil
	}
	return openURL(url)
}

func openURL(url string) error {
	return exec.Command("/usr/bin/open", url).Run()
}

func hasFlag(args *[]string, flag string) bool {
	for i, a := range *args {
		if a == flag {
			*args = append((*args)[:i], (*args)[i+1:]...)
			return true
		}
	}
	return false
}

func flagValue(args *[]string, flag string) string {
	for i, a := range *args {
		if a == flag && i+1 < len(*args) {
			v := (*args)[i+1]
			*args = append((*args)[:i], (*args)[i+2:]...)
			return v
		}
		if v, ok := strings.CutPrefix(a, flag+"="); ok {
			*args = append((*args)[:i], (*args)[i+1:]...)
			return v
		}
	}
	return ""
}
