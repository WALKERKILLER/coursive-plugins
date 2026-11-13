# Agent plugin template

A minimal plugin with a composer toggle and a hook that extends the assistant's system prompt while the toggle is on. Copy this directory into a new repository, then change the `name`, the `coursive.id` and the behavior.

- `package.json`: the `coursive` manifest declares the plugin's id, the manifest API range (`api`), its agent module as a `pi` runtime, its settings form, the action behind the toggle and the permissions it needs (none here).
- `agent.mjs`: `activate(coursive)` runs once per load and registers the action; the default export runs for every conversation and registers hooks or tools with Pi's extension API.

The manifest is v3, defined by the [plugin fabric contract](https://github.com/WALKERKILLER/Coursive/blob/main/docs/specs/0031-plugin-fabric-core.md) and the [v3 guide](https://github.com/WALKERKILLER/Coursive/blob/main/docs/plugin-v3-guide.md) in the Coursive repository; publishing follows the [marketplace contract](https://github.com/WALKERKILLER/Coursive/blob/main/docs/specs/0025-plugin-marketplace.md). It installs in an assistant host that reads v3 manifests (API Version 3.0.0, [decision 0035](https://github.com/WALKERKILLER/Coursive/blob/main/docs/decisions/0035-plugin-api-versions-and-deprecation.md)). Check it with `node <Coursive>/agent/scripts/validate-plugin.mjs . --files --host 3.0.0`.

To publish, pack the plugin (`npm pack`), attach the `.tgz` to a GitHub release tagged with the version, and follow [CONTRIBUTING.md](../../CONTRIBUTING.md).
