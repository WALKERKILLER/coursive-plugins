# Agent plugin template

A minimal plugin with a composer toggle and a hook that extends the assistant's system prompt while the toggle is on. Copy this directory into a new repository, then change the `name`, the `coursive.id` and the behavior.

- `package.json`: the `coursive` manifest declares the plugin's id, the plugin API range (`engine`), its agent module, its settings form, the action behind the toggle and the permissions it needs (none here).
- `agent.mjs`: `activate(coursive)` runs once per load and registers the action; the default export runs for every conversation and registers hooks or tools with Pi's extension API.

The manifest fields, permissions and the `coursive` API are defined by the plugin platform contract in the Coursive repository; this template follows the Draft [marketplace contract](https://github.com/WALKERKILLER/Coursive/blob/main/docs/specs/draft-plugin-marketplace.md).

To publish, pack the plugin (`npm pack`), attach the `.tgz` to a GitHub release tagged with the version, and follow [CONTRIBUTING.md](../../CONTRIBUTING.md).
