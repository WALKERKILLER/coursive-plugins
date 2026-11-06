# AGENTS.md

This repository publishes the Coursive plugin index (`index.json`), the mirror list (`mirrors.json`), the official plugins (`plugins/`) and plugin templates (`templates/`). The contracts it implements are owned by the Coursive repository: the [marketplace](https://github.com/WALKERKILLER/Coursive/blob/main/docs/specs/draft-plugin-marketplace.md) and [source-provider](https://github.com/WALKERKILLER/Coursive/blob/main/docs/specs/draft-source-provider-plugins.md) specs. When a spec changes status or file name, update every link to it here.

## Commands

- Validate: `node scripts/validate.mjs` (add `--artifacts` to download and verify every artifact).

Run validation before every commit; CI runs it on every push and pull request.

## Rules

- Never edit a published version's entry or hash; add a new version instead.
- Only plugins released from this repository may be marked `"official": true`.
- No index entry, plugin, template or doc names a specific institution or course platform; source plugins for such platforms are listed through user-added catalogs.
- Mirrors receive GET requests only; never put credentials, tokens or private URLs into anything fetched through a mirror.
- Do not copy code from GPL/AGPL projects into official plugins or templates.
- Never `git commit` or `git push` unless the user explicitly asks; never read `.env` files or other secrets.
- Follow [CONTRIBUTING.md](CONTRIBUTING.md) for commits, submissions and reviews.
