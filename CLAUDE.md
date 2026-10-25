# publish-cli

Go CLI that turns Obsidian Writing notes into LinkedIn composer text and Substack drafts. macOS only.

## Build

```
go build -o publish-cli .
```

## Project Structure

```
main.go                    # CLI entry point, command routing, clipboard/browser
internal/
  note/note.go             # Load a note: frontmatter, title, Obsidian syntax → CommonMark
  render/
    parse.go               # goldmark parser and shared AST helpers
    linkedin.go            # CommonMark → plain text for the LinkedIn composer
    prosemirror.go         # CommonMark → Substack ProseMirror document
  substack/client.go       # Substack private API: profile, image upload, drafts
  keychain/keychain.go     # security(1) wrapper; service "publish-cli"
  config/config.go         # ~/.config/publish-cli/config.json (no secrets)
```

## Platform facts

- LinkedIn Posts API only accepts `lifecycleState: PUBLISHED` on create, so no API
  drafts. The CLI copies text to the clipboard and opens the composer instead.
- Substack has no official API. Endpoints and the ProseMirror node shapes follow
  python-substack (`substack/api.py`, `substack/nodes.py`). Auth is the
  `substack.sid` cookie. `draft_body` is sent as a JSON *string*.
- Draft endpoints live on the publication host (`https://<pub>/api/v1/drafts`);
  the profile lives on `https://substack.com/api/v1/user/profile/self`.

## Development workflow (TDD)

For every new feature or bug fix:
1. **Write tests first**
2. **Implement** the feature
3. **Lint and format** (`gofmt`, `go vet`)
4. **Run tests** (`go test ./...`)
5. Fix any issues and repeat until clean

## Releasing

Tag-triggered: pushing a `v*` tag runs goreleaser, which publishes the release and
updates `yareeh/homebrew-tap`. Needs the `HOMEBREW_TAP_TOKEN` repo secret.

```bash
git tag v0.1.0
git push origin v0.1.0
```
