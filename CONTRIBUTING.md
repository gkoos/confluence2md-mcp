# Contributing

Thanks for contributing.

## Development workflow

```bash
go build ./...
go vet ./...
go test ./...          # add -race where a C toolchain is available; CI runs it
gofmt -l .             # should print nothing
golangci-lint run      # optional locally; CI pins v2.12.2
```

The offline end-to-end tests install the pinned indexer CLI (`go install ...@v0.9.0`) on first run, build an index from a temporary corpus and drive the server over stdio. They need network access the first time and skip, rather than fail, when the install is impossible. No API key or running service is involved.

Please do not mock the indexer. Those tests exist to catch a schema or payload change in the release this module pins — a mock would hide exactly what they are for.

## Commit messages drive the release

This repository uses [release-please](https://github.com/googleapis/release-please) with Conventional Commits. The version and `CHANGELOG.md` are generated from the squash-merged commit subject, so neither is edited by hand.

| Prefix | Effect |
|---|---|
| `feat:` | minor release |
| `fix:`, `perf:`, `revert:` | patch release |
| `docs:`, `test:`, `ci:`, `chore:`, `refactor:`, `build:` | no release; the change still appears in the next changelog |
| `feat!:` / `fix!:` / a `BREAKING CHANGE:` footer | breaking change — deliberate and rare |

A `Release-As:` footer forces a version; add one only when that is the intent. The tool names and argument names are the public contract, so renaming one is a breaking change.

## When the indexer dependency moves

Four things move together, and CI will not catch a forgotten one:

1. `go.mod` — the version this module builds against.
2. `indexerModuleVersion` in `main_offline_test.go` — the CLI that builds the fixture index.
3. `README.md` — the requirement floor and the "Upgrading an Index" note.
4. `docs/platform.md` — only when the schema condition in that note changes.

Then read the new release's `docs/output-reference.md`. If a payload field or a default moved, the README's Response reference and its samples need the same edit, and a sample payload is worth regenerating from a real index.

## When a tool or an argument changes

- `arguments.go` is the single definition of the shared search surface; an argument added there reaches both `confluence.search` and `confluence.facets`.
- Update the README argument table, and the Response reference when the payload shape changes.
- Update `docs/platform.md` when the tool list or a tool's one-line role changes.
- Extend the offline tests when the argument changes what the indexer returns; the fixture corpus in `main_offline_test.go` is where new cases go.

## Releases

Merging a release-please PR tags the commit, and `release.yml` runs GoReleaser: linux, darwin and windows on amd64 and arm64, plus `checksums.txt`. `main.version` is stamped through `-X main.version` in `.goreleaser.yaml`, so that variable must keep its name — an unstamped build reports `dev`.

## Pull requests

- Explain what changed and why.
- Include test evidence.
- Mention follow-up items explicitly.
