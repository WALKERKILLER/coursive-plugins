#!/usr/bin/env node
// validate.mjs — check index.json and mirrors.json against the marketplace contract.
// The contract is owned by Coursive's plugin marketplace spec:
// https://github.com/WALKERKILLER/Coursive/blob/main/docs/specs/draft-plugin-marketplace.md
// Usage: node scripts/validate.mjs [--artifacts]
//   --artifacts  also download every artifact and check its size and SHA-256.
// Zero dependencies; Node >= 18.
import { createHash } from 'node:crypto';
import { readFile } from 'node:fs/promises';
import process from 'node:process';

const ID = /^[a-z][a-z0-9-]{1,31}$/;
const RESERVED = new Set(['catalog', 'catalogs', 'install', 'third-party']);
const SEMVER = /^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/;
const SHA256 = /^[a-f0-9]{64}$/;
const CATEGORIES = new Set(['notes', 'visualize', 'review', 'programming', 'connector', 'tutor', 'assignment', 'source']);
const KINDS = new Set(['agent', 'skills', 'views', 'actions', 'sources']);
const PLATFORMS = new Set(['any', 'linux-x64', 'linux-arm64', 'windows-x64', 'macos-arm64', 'macos-x64']);
const MIRROR_KINDS = new Set(['raw', 'release', 'archive']);

const errors = [];
const fail = (where, message) => errors.push(`${where}: ${message}`);

function artifactHostAllowed(url) {
  let parsed;
  try { parsed = new URL(url); } catch { return false; }
  if (parsed.protocol !== 'https:') return false;
  if (parsed.host === 'raw.githubusercontent.com') return true;
  return parsed.host === 'github.com' && /^\/[^/]+\/[^/]+\/releases\/download\//.test(parsed.pathname);
}

function checkIndex(index) {
  if (index.schemaVersion !== 1) fail('index.json', 'schemaVersion must be 1');
  if (Number.isNaN(Date.parse(index.generatedAt))) fail('index.json', 'generatedAt must be an ISO 8601 time');
  if (!Array.isArray(index.plugins)) { fail('index.json', 'plugins must be an array'); return []; }
  const ids = new Set();
  const artifacts = [];
  for (const [i, plugin] of index.plugins.entries()) {
    const where = `index.json plugins[${i}] ${plugin.id ?? ''}`.trim();
    if (!ID.test(plugin.id ?? '') || RESERVED.has(plugin.id)) fail(where, 'id must match [a-z][a-z0-9-]{1,31} and not be reserved');
    if (ids.has(plugin.id)) fail(where, 'duplicate id');
    ids.add(plugin.id);
    if (!plugin.title || plugin.title.length > 40) fail(where, 'title is required, at most 40 characters');
    if (!plugin.summary || plugin.summary.length > 120) fail(where, 'summary is required, at most 120 characters');
    if (!plugin.author) fail(where, 'author is required');
    if (!/^https:\/\//.test(plugin.homepage ?? '')) fail(where, 'homepage must be an HTTPS URL');
    for (const c of plugin.categories ?? []) if (!CATEGORIES.has(c)) fail(where, `unknown category ${c}`);
    if (!plugin.kinds?.length || plugin.kinds.some((k) => !KINDS.has(k))) fail(where, `kinds must be a non-empty subset of ${[...KINDS].join(', ')}`);
    if (typeof plugin.official !== 'boolean') fail(where, 'official must be true or false');
    if (!plugin.versions?.length) { fail(where, 'at least one version is required'); continue; }
    const versions = new Set();
    for (const v of plugin.versions) {
      const at = `${where}@${v.version}`;
      if (!SEMVER.test(v.version ?? '')) fail(at, 'version must be semver');
      if (versions.has(v.version)) fail(at, 'duplicate version');
      versions.add(v.version);
      if (!v.engine) fail(at, 'engine range is required');
      if (!/^\d{4}-\d{2}-\d{2}$/.test(v.published ?? '')) fail(at, 'published must be YYYY-MM-DD');
      if (!v.artifacts?.length) { fail(at, 'at least one artifact is required'); continue; }
      const platforms = new Set();
      for (const a of v.artifacts) {
        if (!PLATFORMS.has(a.platform)) fail(at, `unknown platform ${a.platform}`);
        if (platforms.has(a.platform)) fail(at, `duplicate artifact for ${a.platform}`);
        platforms.add(a.platform);
        if (!artifactHostAllowed(a.url)) fail(at, `artifact URL must be a GitHub release asset or raw file: ${a.url}`);
        if (!SHA256.test(a.sha256 ?? '')) fail(at, 'sha256 must be 64 lowercase hex characters');
        if (!Number.isInteger(a.size) || a.size <= 0) fail(at, 'size must be a positive integer');
        artifacts.push({ at, ...a });
      }
      if (platforms.has('any') && platforms.size > 1) fail(at, '"any" cannot be combined with platform artifacts');
    }
  }
  return artifacts;
}

function checkMirrors(list) {
  if (list.schemaVersion !== 1) fail('mirrors.json', 'schemaVersion must be 1');
  if (!Array.isArray(list.mirrors)) { fail('mirrors.json', 'mirrors must be an array'); return; }
  const ids = new Set();
  for (const m of list.mirrors) {
    const where = `mirrors.json ${m.id ?? '?'}`;
    if (!ID.test(m.id ?? '')) fail(where, 'id must match [a-z][a-z0-9-]{1,31}');
    if (ids.has(m.id)) fail(where, 'duplicate id');
    ids.add(m.id);
    if (!m.name) fail(where, 'name is required');
    if (!m.kinds?.length || m.kinds.some((k) => !MIRROR_KINDS.has(k))) fail(where, 'kinds must be a non-empty subset of raw, release, archive');
    const t = m.template ?? '';
    if (m.id === 'direct') { if (t !== '{url}') fail(where, 'direct must use the template {url}'); continue; }
    if (!t.startsWith('https://')) fail(where, 'template must be HTTPS');
    const whole = t.includes('{url}') || t.includes('{path}');
    const parts = ['{owner}', '{repo}', '{ref}', '{file}'].every((p) => t.includes(p));
    if (!whole && !parts) fail(where, 'template needs {url}, {path}, or {owner} {repo} {ref} {file}');
    if (parts && !whole && m.kinds.some((k) => k !== 'raw')) fail(where, 'an {owner}/{repo}/{ref}/{file} template serves raw files only');
  }
  if (!ids.has('direct')) fail('mirrors.json', 'the direct entry is required');
}

async function checkArtifact(a) {
  const response = await fetch(a.url, { redirect: 'follow' });
  if (!response.ok) { fail(a.at, `download failed with HTTP ${response.status}: ${a.url}`); return; }
  const body = Buffer.from(await response.arrayBuffer());
  if (body.length !== a.size) fail(a.at, `size ${body.length} does not match ${a.size}`);
  if (createHash('sha256').update(body).digest('hex') !== a.sha256) fail(a.at, 'sha256 does not match the downloaded artifact');
}

const artifacts = checkIndex(JSON.parse(await readFile('index.json', 'utf8')));
checkMirrors(JSON.parse(await readFile('mirrors.json', 'utf8')));
if (process.argv.includes('--artifacts')) for (const a of artifacts) await checkArtifact(a);
if (errors.length) {
  for (const e of errors) console.error(`validate: ${e}`);
  process.exit(1);
}
console.log(`validate: OK (${artifacts.length} artifacts${process.argv.includes('--artifacts') ? ' downloaded and verified' : ''})`);
