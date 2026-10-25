# publish-cli

Turn an Obsidian Writing note into a LinkedIn post or a Substack draft. macOS only.

```
publish-cli linkedin "Writing/2026Q4/10-25-01 Banned From Utopia.md"
publish-cli substack draft "Writing/2026Q4/10-25-02 Luontokatsaus.md"
```

## LinkedIn

LinkedIn's Posts API only accepts `lifecycleState: PUBLISHED` when creating a post,
so it can't make drafts. Instead `publish-cli linkedin`:

1. converts the note to plain text (formatting markers dropped, links written as
   `text (url)`, images left out),
2. copies it to the clipboard,
3. reveals the note's first image in Finder, if it has one, and
4. opens the LinkedIn composer, where you paste, attach the image and post.

It warns when the text is over LinkedIn's 3000-character limit.
`--print` prints the text instead.

## Substack

Substack has no public API. `publish-cli substack` uses the private API its editor
uses (endpoints follow [python-substack](https://github.com/ma2za/python-substack)),
so it can break without notice.

```
publish-cli substack login     # once, and again when the cookie expires
publish-cli substack draft <note.md>
```

`login` asks for the `substack.sid` cookie. Log in to substack.com in the browser
(MFA happens there), open the developer tools → Application/Storage → Cookies →
`https://substack.com` and copy the value. The cookie is checked against your
profile and stored in the login keychain (service `publish-cli`). The
publication URL and user ID go to `~/.config/publish-cli/config.json`.
`--publication <url>` picks a publication other than your primary one.

`draft` converts the note to Substack's editor format, uploads local images, creates
the draft and opens it in the editor. The draft's URL is written into the note's
frontmatter as `substack_draft`; running `draft` again updates that draft instead of
making a new one (`--new` forces a new one). `--dry-run` prints the payload without
contacting Substack.

## Notes in, posts out

- The title is the note's leading H1, or the file name without its `MM-DD-NN ` prefix
  when the note doesn't start with one. An H2 straight after that H1 is the subtitle.
  Both become plain text. Substack gets them as the draft's title and subtitle and
  leaves them out of the body; LinkedIn keeps them as the first lines of the text.
  Without an H1, the LinkedIn text starts with the title from the file name, unless
  the note is still named only `MM-DD-NN.md`.
- Frontmatter, `%%comments%%` and `==highlight==` markers are removed.
- `![[image.png]]` embeds are found next to the note or anywhere in the vault;
  other embeds are dropped.
- `[[Note|alias]]` and `[[Note]]` become plain text.
- Single line breaks inside a paragraph are kept for LinkedIn and become spaces on Substack.

## Publication tags and renaming

`linkedin` and `substack draft` add `published/linkedin` or `published/substack` to the
note's frontmatter tags (`--print` and `--dry-run` don't). When that is the note's first
`published/` tag, the note is renamed to today's date: `MM-DD-NN` with NN one higher
than the highest ordinal already used today, in the current `YYYYQn` folder.
Attachments with the same name move along, and `[[…]]` links to them anywhere in the
vault are updated. A note already dated today, or one whose name doesn't start with a
date, keeps its name. The new path is printed as `Renamed to: <path>`.

## Running it from Obsidian

With the [Shell commands](https://github.com/Taitava/obsidian-shellcommands) plugin,
add commands such as

```
publish-cli linkedin {{file_path:absolute}}
publish-cli substack draft {{file_path:absolute}}
```

and give them hotkeys. Shell commands doesn't run your login shell, so either use the
full path (`/opt/homebrew/bin/publish-cli`) or add `/opt/homebrew/bin` to the plugin's
PATH setting.

## Install

```
brew install yareeh/tap/publish-cli
```
