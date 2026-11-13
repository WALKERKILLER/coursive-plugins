# AGENTS.md

This repository publishes the Coursive plugin index (`index.json`), the mirror list (`mirrors.json`), the official plugins (`plugins/`) and plugin templates (`templates/`). The contracts it implements are owned by the Coursive repository: the [marketplace](https://github.com/WALKERKILLER/Coursive/blob/main/docs/specs/0025-plugin-marketplace.md) and [source-provider](https://github.com/WALKERKILLER/Coursive/blob/main/docs/specs/0024-source-provider-plugins.md) specs. When a spec changes status or file name, update every link to it here.

## Commands

- Validate: `node scripts/validate.mjs` (add `--artifacts` to download and verify every artifact, `--signed <dir>` to check a published index and its signature).
- Manifests: `node scripts/check-manifests.mjs --rules <Coursive checkout>` checks every manifest in `plugins/` and `templates/` against Coursive's v3 rules and checks that each source plugin still installs through the daemon's source market (add `--files` after a pack to require the built executables, `--index <dir>/index.json` to check a packed index's `engine` ranges). CI pins the Coursive commit in `ci.yml`.
- Go code (`sdk/`, `internal/`, source plugins): `go vet ./...` and `go test ./...`; build a provider with `CGO_ENABLED=0 go build -trimpath -o plugins/<id>/bin/provider-<platform> ./plugins/<id>`.
- Pack source plugins: `node scripts/pack-sources.mjs --out <dir>` writes one `.tgz` per platform and an `index.json` (`--all` for every platform, `--release` for GitHub release URLs).
- Publish: `.github/workflows/publish.yml` runs `node scripts/publish.mjs --into <dir>` after every merge to `main`. It releases each official plugin version not yet published, merges in the third-party entries of `main`'s `index.json`, signs the result with the key in `minisign.pub` and pushes `index.json` and `index.json.sig` to the `index` branch. Locally, use only `--dry-run`.

Run validation before every commit; CI runs it on every push and pull request, together with `gofmt`, `go vet`, `go test -race`, a pack of every source plugin for every platform and the manifest check.

## Rules

- Never edit a published version's entry or hash; add a new version instead.
- Manifests here use v3 (`coursive.api`, `runtime`, `contributes`; Coursive [spec 0031](https://github.com/WALKERKILLER/Coursive/blob/main/docs/specs/0031-plugin-fabric-core.md) and [decision 0035](https://github.com/WALKERKILLER/Coursive/blob/main/docs/decisions/0035-plugin-api-versions-and-deprecation.md)). A source plugin's index versions keep `engine: "^2.0.0"`, the source market API every daemon matches; `pack-sources.mjs` writes it.
- Only plugins released from this repository may be marked `"official": true`. Official plugins are never listed in `main`'s `index.json`: the publish workflow adds them from `plugins/`. To publish new plugin code, bump its version in `package.json`.
- Only the publish workflow writes the `index` branch. Never read, print or copy `INDEX_SIGNING_KEY`; only that workflow uses it.
- Templates, the SDK, `internal/` packages and repository docs stay institution-neutral. An institution or course platform is named only inside its own source plugin directory (`plugins/<id>/`) and that plugin's index entry and README.
- Institution source plugins never ship credentials, test accounts or captured sessions; sign-in state lives only in the daemon's vault.
- Mirrors receive GET requests only; never put credentials, tokens or private URLs into anything fetched through a mirror.
- Do not copy code from GPL/AGPL projects into official plugins or templates.
- Never `git commit` or `git push` unless the user explicitly asks; never read `.env` files or other secrets.
- Follow [CONTRIBUTING.md](CONTRIBUTING.md) for commits, submissions and reviews.
