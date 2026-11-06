#!/usr/bin/env node
// publish.mjs — release every source-plugin version the published index
// lacks, and write the signed index Coursive reads.
// Usage: node scripts/publish.mjs --into <dir> [--dry-run]
//   --into     the published index (the `index` branch): <dir>/index.json is
//              read if present and rewritten with <dir>/index.json.sig
//   --dry-run  pack and write the index without creating releases or signing
// The published index holds the official plugins in plugins/, each version
// released as <id>-v<version>, plus exactly the third-party entries of this
// branch's index.json. A published version is never changed: to publish new
// code, bump the plugin's version in its package.json.
// Needs Go, Node >= 18 and, unless --dry-run, the GitHub CLI and
// INDEX_SIGNING_KEY (a base64 PKCS#8 Ed25519 key; its public key is
// minisign.pub). Run by .github/workflows/publish.yml.
import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { mkdtemp, readFile, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import process from 'node:process';
import { signMinisign, verifyMinisign } from './minisign.mjs';

const args = process.argv.slice(2);
const into = args[args.indexOf('--into') + 1];
if (!args.includes('--into') || !into) {
  console.error('usage: node scripts/publish.mjs --into <dir> [--dry-run]');
  process.exit(2);
}
const dry = args.includes('--dry-run');
const read = async (p, fallback) => { try { return await readFile(p, 'utf8'); } catch { return fallback; } };

const out = await mkdtemp(join(tmpdir(), 'coursive-publish-'));
execFileSync('node', ['scripts/pack-sources.mjs', '--out', out, '--all', '--release'], { stdio: 'inherit' });
const packed = JSON.parse(await readFile(join(out, 'index.json'), 'utf8'));
const before = await read(join(into, 'index.json'), '');
const published = before ? JSON.parse(before) : { schemaVersion: 1, generatedAt: '', plugins: [] };
const submitted = JSON.parse(await readFile('index.json', 'utf8'));
const gh = (...a) => execFileSync('gh', a, { stdio: ['ignore', 'pipe', 'inherit'] }).toString();

const official = new Set(packed.plugins.map((p) => p.id));
for (const e of submitted.plugins) {
  if (official.has(e.id) || e.official) throw new Error(`index.json: ${e.id} is official; official plugins are published from plugins/`);
}
const plugins = [...published.plugins.filter((e) => official.has(e.id)), ...submitted.plugins];

const added = [];
for (const p of packed.plugins) {
  const [version] = p.versions;
  let entry = plugins.find((e) => e.id === p.id);
  if (entry?.versions.some((v) => v.version === version.version)) continue;
  const tag = `${p.id}-v${version.version}`;
  if (!dry) {
    const files = version.artifacts.map((a) => join(out, a.url.split('/').pop()));
    let exists = true;
    try { gh('release', 'view', tag, '--json', 'tagName'); } catch { exists = false; }
    if (exists) gh('release', 'upload', tag, ...files, '--clobber');
    else gh('release', 'create', tag, ...files, '--title', `${p.title} ${version.version}`, '--notes', `${p.title} ${version.version}, listed in the signed plugin index.`);
  }
  const { versions, ...meta } = p;
  if (!entry) {
    entry = { ...meta, versions: [] };
    plugins.push(entry);
  }
  Object.assign(entry, meta);
  entry.versions = [version, ...entry.versions];
  added.push(tag);
}
plugins.sort((a, b) => a.id.localeCompare(b.id));

const same = JSON.stringify(plugins) === JSON.stringify(published.plugins);
const signed = before !== '' && (() => {
  try { return verifyMinisign(Buffer.from(before), readFileSync(join(into, 'index.json.sig'), 'utf8'), readFileSync('minisign.pub', 'utf8')); } catch { return false; }
})();
if (same && signed) {
  console.log('publish: the published index is current and signed');
  process.exit(0);
}
const body = `${JSON.stringify({ schemaVersion: 1, generatedAt: new Date().toISOString(), plugins }, null, 2)}\n`;
await writeFile(join(into, 'index.json'), body);
if (!dry) {
  const key = process.env.INDEX_SIGNING_KEY;
  if (!key) throw new Error('INDEX_SIGNING_KEY is not set');
  const sig = signMinisign(Buffer.from(body), key, `timestamp:${Math.floor(Date.now() / 1000)}\tfile:index.json`);
  if (!verifyMinisign(Buffer.from(body), sig, await readFile('minisign.pub', 'utf8'))) throw new Error('INDEX_SIGNING_KEY does not match minisign.pub');
  await writeFile(join(into, 'index.json.sig'), sig);
}
console.log(`publish: ${added.length ? added.join(', ') : 'no new versions'}; index ${dry ? 'written, not signed' : 'written and signed'}`);
