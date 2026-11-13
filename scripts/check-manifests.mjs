#!/usr/bin/env node
// check-manifests.mjs — check every plugin and template manifest in this
// repository against Coursive's v3 manifest rules, and check that every source
// plugin still installs through the daemon's source market.
// The v3 rules are Coursive's agent/scripts/validate-plugin.mjs and
// agent/src/plugin-manifest.mjs (spec 0031, decision 0035), imported from a
// Coursive checkout instead of copied. The source market is
// daemon/internal/sources (spec 0025); its versions are read from that code.
// Usage: node scripts/check-manifests.mjs --rules <coursive> [--files] [--index <file>]
//   --rules  a Coursive checkout; CI pins its commit in .github/workflows/ci.yml
//   --files  also require the entries and executables of plugins/ to exist,
//            after pack-sources.mjs has built them
//   --index  an index written by pack-sources.mjs: every source plugin version
//            must carry an engine the daemon's source market accepts
// A manifest without coursive.api is v1: deprecated but still installable, so
// it is reported as a warning and not held to the v3 rules.
// Zero dependencies; Node >= 22.
import { readdir, readFile } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import process from 'node:process';
import { pathToFileURL } from 'node:url';

const args = process.argv.slice(2);
const option = (name) => (args.includes(name) ? args[args.indexOf(name) + 1] : undefined);
const rules = option('--rules');
const indexFile = option('--index');
if (!rules || (args.includes('--index') && !indexFile)) {
  console.error('usage: node scripts/check-manifests.mjs --rules <coursive> [--files] [--index <file>]');
  process.exit(2);
}
const load = (file) => import(pathToFileURL(resolve(rules, file)).href);
const { validate } = await load('agent/scripts/validate-plugin.mjs');
const { satisfiesApi } = await load('agent/src/plugin-manifest.mjs');
const constant = async (file, name) => {
  const m = new RegExp(`const ${name} = "([^"]+)"`).exec(await readFile(resolve(rules, file), 'utf8'));
  if (!m) throw new Error(`${file} has no ${name}`);
  return m[1];
};
const HOST_API = await constant('agent/src/plugins.mjs', 'HOST_API_VERSION');
const SOURCE_API = await constant('daemon/internal/sources/market.go', 'PluginAPI');
const PROTOCOL = await constant('daemon/internal/sources/protocol.go', 'ProtocolVersion');
const SOURCE_ID = /^[a-z][a-z0-9-]{1,31}$/;
const PLATFORMS = new Set(['linux-x64', 'linux-arm64', 'windows-x64', 'macos-arm64', 'macos-x64']);
const MANIFESTS = ['package.json', 'manifest.example.json'];

const errors = [];
const warnings = [];
const list = (v) => (Array.isArray(v) ? v : []);
const pairs = (map) => Object.entries(map && typeof map === 'object' ? map : {}).map(([k, v]) => `${k}=${v}`);

// What the daemon's source registry (daemon/internal/sources/registry.go)
// refuses beyond the v3 rules, and the process runtime that runs the
// provider executables the sources name.
function checkSources(where, c) {
  const fail = (message) => errors.push(`${where}: ${message}`);
  const sources = list(c.contributes?.sources);
  if (!list(c.permissions).includes('source')) fail('a source plugin declares the "source" permission');
  const executables = new Set();
  for (const [i, s] of sources.entries()) {
    const at = `coursive.contributes.sources[${i}]`;
    if (!SOURCE_ID.test(s?.id ?? '')) fail(`${at}.id ${JSON.stringify(s?.id)} must match [a-z][a-z0-9-]{1,31}`);
    if (s?.protocol !== PROTOCOL) fail(`${at}.protocol is ${JSON.stringify(s?.protocol)}; the daemon speaks protocol ${PROTOCOL}`);
    for (const platform of Object.keys(s?.executable ?? {})) if (!PLATFORMS.has(platform)) fail(`${at}.executable has unknown platform ${platform}`);
    for (const p of pairs(s?.executable)) executables.add(p);
  }
  const runtimes = list(c.runtime).filter((r) => r?.kind === 'process');
  const entries = new Set(runtimes.flatMap((r) => pairs(r.entry)));
  if (runtimes.length !== 1 || entries.size !== executables.size || [...entries].some((p) => !executables.has(p))) {
    fail('coursive.runtime needs one "process" component whose entry is the executable map of contributes.sources');
  }
}

for (const base of ['plugins', 'templates']) {
  for (const dir of (await readdir(base, { withFileTypes: true })).filter((d) => d.isDirectory()).map((d) => d.name).sort()) {
    const names = await readdir(join(base, dir));
    for (const name of MANIFESTS.filter((n) => names.includes(n))) {
      const where = join(base, dir, name);
      const pkg = JSON.parse(await readFile(where, 'utf8'));
      if (!pkg.coursive?.api) {
        warnings.push(`${where}: v1 manifest; it still installs, but v1 is deprecated: replace coursive.engine with coursive.api (decision 0035)`);
        continue;
      }
      const report = await validate(where, { files: args.includes('--files') && base === 'plugins', host: HOST_API });
      for (const p of report.problems) errors.push(`${where}: ${p.path} [${p.code}] ${p.message}${p.hint ? ` (${p.hint})` : ''}`);
      if (list(pkg.coursive.contributes?.sources).length) checkSources(where, pkg.coursive);
    }
  }
}

if (indexFile) {
  const index = JSON.parse(await readFile(indexFile, 'utf8'));
  for (const p of list(index.plugins).filter((e) => list(e.kinds).includes('sources'))) {
    for (const v of list(p.versions)) {
      if (!satisfiesApi(v.engine, SOURCE_API)) errors.push(`${indexFile}: ${p.id}@${v.version} engine ${JSON.stringify(v.engine)} does not match the source market's plugin API ${SOURCE_API}`);
    }
  }
}

for (const w of warnings) console.warn(`check-manifests: warning: ${w}`);
if (errors.length) {
  for (const e of errors) console.error(`check-manifests: ${e}`);
  process.exit(1);
}
console.log(`check-manifests: OK (manifest API ${HOST_API}, source market ${SOURCE_API}, protocol ${PROTOCOL}${indexFile ? `, ${indexFile}` : ''})`);
