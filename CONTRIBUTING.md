# Contributing

This repository holds the Coursive plugin index, the mirror list and the official plugins. The formats are defined by the [marketplace contract](https://github.com/WALKERKILLER/Coursive/blob/main/docs/specs/draft-plugin-marketplace.md) and the [source-provider contract](https://github.com/WALKERKILLER/Coursive/blob/main/docs/specs/draft-source-provider-plugins.md) in the Coursive repository; a format change starts there, not here.

## Submitting a plugin

1. Develop the plugin in your own public repository, starting from [templates/](templates/).
2. Publish a GitHub release whose tag is the plugin version. Attach one `.tgz` for a platform-independent plugin, or one per platform key (`linux-x64`, `linux-arm64`, `windows-x64`, `macos-arm64`, `macos-x64`) for a source plugin.
3. Add your entry to [index.json](index.json) in a pull request, keeping `plugins` sorted by `id`. Fill in each artifact's `size` and `sha256` (`sha256sum <file>`).
4. Run `node scripts/validate.mjs --artifacts` and paste its output into the pull request.

A submission is accepted when:

- the plugin's source is public and its license allows redistribution of the release artifacts;
- the requested permissions and hosts are the minimum its features need, and its README explains each one;
- it does not collect data beyond what its features need and sends nothing to hosts it does not declare;
- a source plugin stores secrets only through the daemon vault and never logs them;
- the artifacts were built from the tagged source, ideally by a public CI workflow.

Updates add a new entry to the plugin's `versions`; existing versions and their hashes are never edited. A maintainer may remove a plugin that breaks these rules.

## Official plugins

Official plugins live under [plugins/](plugins/), one directory per plugin, and are released from this repository as `<id>-v<version>`. Only entries released from this repository may set `"official": true`.

## Mirrors

Propose a new mirror or report a broken one with the issue forms. A mirror is listed when it serves byte-identical GitHub content over HTTPS without requiring an account; it never receives credentials.

## Commits and pull requests

- Commit messages and pull request titles use `type(scope): summary`, where type is `feat`, `fix`, `docs`, `ci` or `chore`, and scope is a plugin id, `index`, `mirrors`, or omitted. Example: `feat(index): add mindmap 1.0.0`.
- CI must pass; `main` accepts changes only through pull requests reviewed by the code owner @WALKERKILLER.

## Code of conduct

Be respectful and constructive. Assume good faith.
