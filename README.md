<a id="readme-top"></a>

<div align="right">
<strong>English</strong> | <a href="README.zh-CN.md">简体中文</a>
</div>

<div align="center">

<h1>Coursive Plugins</h1>

<p><strong>The plugin index, mirror list and official plugins for <a href="https://github.com/WALKERKILLER/Coursive">Coursive</a></strong><br />
<sub>A signed index the app trusts, mirrors that only carry bytes, and the source plugins that sign in to course platforms</sub></p>

<p>
<a href="https://github.com/WALKERKILLER/coursive-plugins/stargazers"><img height="22" src="https://m3-markdown-badges.vercel.app/stars/3/3/WALKERKILLER/coursive-plugins" alt="Stars" /></a>
&nbsp;
<a href="https://go.dev"><img height="22" src="https://ziadoua.github.io/m3-Markdown-Badges/badges/Go/go3.svg" alt="Go" /></a>
&nbsp;
<a href="https://nodejs.org"><img height="22" src="https://ziadoua.github.io/m3-Markdown-Badges/badges/NodeJS/nodejs3.svg" alt="Node.js" /></a>
&nbsp;
<a href="index.json"><img height="22" src="https://ziadoua.github.io/m3-Markdown-Badges/badges/JSON/json3.svg" alt="JSON" /></a>
&nbsp;
<a href="LICENSE"><img height="22" src="https://ziadoua.github.io/m3-Markdown-Badges/badges/LicenceMIT/licencemit3.svg" alt="MIT" /></a>
</p>

<p>
<img height="22" src="https://ziadoua.github.io/m3-Markdown-Badges/badges/Linux/linux3.svg" alt="Linux" />
&nbsp;
<img height="22" src="https://ziadoua.github.io/m3-Markdown-Badges/badges/Windows/windows3.svg" alt="Windows" />
&nbsp;
<img height="22" src="https://ziadoua.github.io/m3-Markdown-Badges/badges/macOS/macos3.svg" alt="macOS" />
</p>

<p>
<sub><em>Read the <a href="https://github.com/WALKERKILLER/Coursive/blob/main/docs/specs/0025-plugin-marketplace.md">Marketplace contract</a> · Build on the <a href="https://github.com/WALKERKILLER/Coursive/blob/main/docs/specs/0024-source-provider-plugins.md">Source-provider contract</a> · Follow the <a href="https://github.com/WALKERKILLER/Coursive/blob/main/docs/plugin-v3-guide.md">v3 guide</a> · Check <a href="https://github.com/WALKERKILLER/coursive-plugins/actions/workflows/ci.yml">CI</a></em></sub>
</p>
<br />

</div>

> **Things to note**: the marketplace contract is approved in the Coursive repository as spec 0025. Coursive reads the signed index from the [`index`](https://github.com/WALKERKILLER/coursive-plugins/tree/index) branch, which the publish workflow writes after every merge.

## What Lives Here

- **[index.json](index.json)**: the third-party plugins submitted to the marketplace, with their versions, platform artifacts, sizes and SHA-256 hashes. The publish workflow adds the official plugins, signs the result with the Ed25519 key in [minisign.pub](minisign.pub) and publishes it to the `index` branch; Coursive checks that signature before trusting the index.
- **[mirrors.json](mirrors.json)**: the GitHub mirrors Coursive may download through. Coursive measures them with a speed test and uses the fastest; a mirror only carries bytes, and every download is checked against the signed index.
- **[plugins/](plugins/)**: the source of the official plugins, each released as `<id>-v<version>`.
- **[sdk/provider](sdk/provider)**: the Go SDK that source plugins use to speak the provider protocol.
- **[templates/](templates/)**: starting points for an agent plugin and a source plugin.
- **[scripts/validate.mjs](scripts/validate.mjs)**: the checks CI runs on every change.
- **[scripts/check-manifests.mjs](scripts/check-manifests.mjs)**: checks the manifests of the official plugins and templates against Coursive's [v3 manifest rules](https://github.com/WALKERKILLER/Coursive/blob/main/docs/plugin-v3-guide.md) and the daemon's source market.

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
node scripts/check-manifests.mjs --rules ../Coursive  # manifests, against a Coursive checkout
```

A plugin in its own repository checks its manifest with Coursive's validator: `node <Coursive>/agent/scripts/validate-plugin.mjs <plugin-dir> --files --host 3.0.0`.

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md) and [AGENTS.md](AGENTS.md) first; human and AI contributors follow the same rules.

## Community

**Feedback:**

- Report a problem with a listed plugin with the **Plugin problem** form and report a dead or misbehaving mirror with the **Mirror report** form in [Issues](https://github.com/WALKERKILLER/coursive-plugins/issues/new/choose).
- Report vulnerabilities in the index, the mirror list or an official plugin privately, as described in [SECURITY.md](SECURITY.md).

**Built With:**

- [Go](https://go.dev) for the provider SDK and the source plugins, and [Node.js](https://nodejs.org) for the index scripts.

The index, scripts and official plugins are released under the [MIT License](LICENSE). Third-party plugins listed in the index keep their own licenses.

---

<div align="right">
<a href="#readme-top">↑ Back to top</a>
</div>
