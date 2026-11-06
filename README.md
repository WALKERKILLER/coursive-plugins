<div align="center">

# Coursive Plugins

**The plugin index, the download mirror list and the official plugins for [Coursive](https://github.com/WALKERKILLER/Coursive).**

[![CI](https://github.com/WALKERKILLER/coursive-plugins/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/WALKERKILLER/coursive-plugins/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-2f7cf6.svg)](LICENSE)
[![Index schema](https://img.shields.io/badge/index%20schema-v1-44adfe.svg)](index.json)

*[简体中文](README.zh-CN.md) · [Marketplace contract](https://github.com/WALKERKILLER/Coursive/blob/main/docs/specs/0025-plugin-marketplace.md) · [Source-provider contract](https://github.com/WALKERKILLER/Coursive/blob/main/docs/specs/0024-source-provider-plugins.md) · [Contributing](CONTRIBUTING.md)*

</div>

> [!NOTE]
> The marketplace contract is approved in the Coursive repository as spec 0025. The published index lists no plugins yet; the official plugins in `plugins/` build into a local index with `scripts/pack-sources.mjs`.

## What Lives Here

- **[index.json](index.json)**: every plugin the Coursive marketplace shows, with its versions, platform artifacts, sizes and SHA-256 hashes. Release builds publish it with an Ed25519 signature that Coursive checks before trusting it.
- **[mirrors.json](mirrors.json)**: the GitHub mirrors Coursive may download through. Coursive measures them with a speed test and uses the fastest; a mirror only carries bytes, and every download is checked against the signed index.
- **[plugins/](plugins/)**: the source of the official plugins, each released as `<id>-v<version>`.
- **[sdk/provider](sdk/provider)**: the Go SDK that source plugins use to speak the provider protocol.
- **[templates/](templates/)**: starting points for an agent plugin and a source plugin.
- **[scripts/validate.mjs](scripts/validate.mjs)**: the checks CI runs on every change.

## Plugin Kinds

A plugin is a package whose `package.json` carries a `coursive` manifest. One plugin can contribute several kinds:

- **Agent tools, skills and hooks**: run in Coursive's assistant host as Pi extensions.
- **Views and actions**: interactive payloads the app renders, and buttons in the app that call the plugin.
- **Sources**: a native provider executable that signs in to an external course platform, lists courses and lectures and resolves media. Coursive's daemon stores its secrets and performs the downloads.

## Mirrors

| Mirror | Raw files | Releases | Archives |
| --- | --- | --- | --- |
| GitHub (direct) | ✓ | ✓ | ✓ |
| gh-proxy.com | ✓ | ✓ | ✓ |
| ghfast.top | ✓ | ✓ | ✓ |
| jsDelivr | ✓ | | |
| gh.llkk.cc | ✓ | ✓ | ✓ |
| ghproxy.net | ✓ | | ✓ |

Public mirrors come and go. The list was last checked on the date in `mirrors.json`; report a dead or misbehaving mirror with the **Mirror report** issue form. Never send a token or a private URL through a mirror.

## Publishing a Plugin

1. Start from a template in [templates/](templates/) and develop it in your own repository.
2. Publish a GitHub release whose tag is the plugin version and attach the artifacts (one `.tgz`, or one per platform for a source plugin).
3. Open a pull request that adds your entry to `index.json`; the full checklist is in [CONTRIBUTING.md](CONTRIBUTING.md).

CI downloads every new artifact and checks its size and hash. A maintainer reviews the code and the requested permissions before merging.

## Validating Locally

```sh
node scripts/validate.mjs              # formats and rules
node scripts/validate.mjs --artifacts  # also download and verify every artifact
```

## Security

Report vulnerabilities in the index, the mirror list or an official plugin privately, as described in [SECURITY.md](SECURITY.md).

## License

The index, scripts and official plugins are released under the [MIT License](LICENSE). Third-party plugins listed in the index keep their own licenses.

---

<div align="center">

[↑ Back to top](#coursive-plugins)

</div>
