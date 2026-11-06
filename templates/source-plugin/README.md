# Source plugin template

A source plugin connects Coursive to an external course platform. It ships a native provider executable per platform that speaks the protocol in the [source-provider contract](https://github.com/WALKERKILLER/Coursive/blob/main/docs/specs/draft-source-provider-plugins.md): JSON-RPC 2.0, one message per line, over stdin and stdout.

## Layout

```text
package.json            # coursive manifest with contributes.sources and the "source" permission
bin/
├── provider-linux-x64
├── provider-linux-arm64
├── provider-windows-x64.exe
├── provider-macos-arm64
└── provider-macos-x64
```

`manifest.example.json` shows the `coursive` manifest of a source plugin.

## Rules for providers

- Read requests from stdin and write responses to stdout only; diagnostics go to stderr and never contain secrets.
- Store anything that must survive a restart through `vault.put`; keep nothing secret in files.
- Return media and courseware as URLs with the headers needed to fetch them; never download them yourself.
- Answer `initialize` within 10 seconds and announce only the capabilities you implement.
- Report a needed sign-in with the `auth_required` error kind and rate limits with `rate_limited` and `retryAfterSec`.

Build every executable in a public CI workflow from the tagged source, and attach them to the release one asset per platform.
