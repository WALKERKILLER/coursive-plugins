# AGENTS.md

This repository publishes the Coursive plugin index (`index.json`), the mirror list (`mirrors.json`), the official plugins (`plugins/`) and plugin templates (`templates/`). The contracts it implements are owned by the Coursive repository: the [marketplace](https://github.com/WALKERKILLER/Coursive/blob/main/docs/specs/0025-plugin-marketplace.md) and [source-provider](https://github.com/WALKERKILLER/Coursive/blob/main/docs/specs/0024-source-provider-plugins.md) specs. When a spec changes status or file name, update every link to it here.

## Commands

- Validate: `node scripts/validate.mjs` (add `--artifacts` to download and verify every artifact).
- Go code (`sdk/`, `internal/`, source plugins): `go vet ./...` and `go test ./...`; build a provider with `CGO_ENABLED=0 go build -trimpath -o plugins/<id>/bin/provider-<platform> ./plugins/<id>`.
- Pack source plugins: `node scripts/pack-sources.mjs --out <dir>` writes one `.tgz` per platform and an `index.json` (`--all` for every platform, `--release` for GitHub release URLs).

Run validation before every commit; CI runs it on every push and pull request, together with `gofmt`, `go vet`, `go test -race` and a pack of every source plugin for every platform.

## Rules

- Never edit a published version's entry or hash; add a new version instead.
- Only plugins released from this repository may be marked `"official": true`.
- Templates, the SDK, `internal/` packages and repository docs stay institution-neutral. An institution or course platform is named only inside its own source plugin directory (`plugins/<id>/`) and that plugin's index entry and README.
- Institution source plugins never ship credentials, test accounts or captured sessions; sign-in state lives only in the daemon's vault.
- Mirrors receive GET requests only; never put credentials, tokens or private URLs into anything fetched through a mirror.
- Do not copy code from GPL/AGPL projects into official plugins or templates.
- Never `git commit` or `git push` unless the user explicitly asks; never read `.env` files or other secrets.
- Follow [CONTRIBUTING.md](CONTRIBUTING.md) for commits, submissions and reviews.
