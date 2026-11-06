# Official plugins

Each official plugin lives in its own directory here, named by its plugin id, and is released from this repository with the tag `<id>-v<version>`. The plugins bundled with the Coursive assistant move here as the Coursive code migrates.

| Plugin | Kind | What it connects |
| --- | --- | --- |
| [tongji](tongji/) | Source | Tongji University: recorded lectures and Canvas, through the university's identity provider |
| [fudan](fudan/) | Source | Fudan University: iCourse recorded lectures through WebVPN, and Canvas, through UIS |

Source plugins are written in Go on the shared provider SDK in [`sdk/provider`](../sdk/provider). Build one for this device with:

```sh
CGO_ENABLED=0 go build -trimpath -o plugins/<id>/bin/provider-linux-x64 ./plugins/<id>
```

The `bin/` directories are build output and are not committed; release builds attach one executable per platform.

To try the plugins in Coursive before they are released, pack them into a local index and point the daemon at it; they then appear under 插件 → 发现 → 来源插件:

```sh
node scripts/pack-sources.mjs --out /tmp/coursive-market
COURSIVE_PLUGIN_INDEX=/tmp/coursive-market/index.json coursive
```
