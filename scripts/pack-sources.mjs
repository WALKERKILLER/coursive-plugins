#!/usr/bin/env node
// pack-sources.mjs — build every source plugin in plugins/, pack one .tgz
// per platform and write an index.json listing them, in the marketplace
// contract's format:
// https://github.com/WALKERKILLER/Coursive/blob/main/docs/specs/0025-plugin-marketplace.md
// Usage: node scripts/pack-sources.mjs --out <dir> [--all] [--release]
//   --all      build every platform, not only this device's
//   --release  artifact URLs point at this repository's GitHub releases;
//              otherwise they are file names next to the index, for a local
//              index (Coursive's COURSIVE_PLUGIN_INDEX)
// Needs Go and Node >= 18; zero npm dependencies.
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdir, readdir, readFile, stat, writeFile } from 'node:fs/promises';
import { extname, join } from 'node:path';
import process from 'node:process';
import { gzipSync } from 'node:zlib';

const REPO = 'WALKERKILLER/coursive-plugins';
const ICON_TYPES = { '.png': 'image/png', '.webp': 'image/webp', '.svg': 'image/svg+xml' };
const TARGETS = {
  'linux-x64': ['linux', 'amd64'],
  'linux-arm64': ['linux', 'arm64'],
  'windows-x64': ['windows', 'amd64'],
  'macos-arm64': ['darwin', 'arm64'],
  'macos-x64': ['darwin', 'amd64'],
};

const args = process.argv.slice(2);
const out = args[args.indexOf('--out') + 1];
if (!args.includes('--out') || !out) {
  console.error('usage: node scripts/pack-sources.mjs --out <dir> [--all] [--release]');
  process.exit(2);
}
const here = `${{ linux: 'linux', darwin: 'macos', win32: 'windows' }[process.platform]}-${{ x64: 'x64', arm64: 'arm64' }[process.arch]}`;
const platforms = args.includes('--all') ? Object.keys(TARGETS) : [here];

// A ustar archive of [name, data, mode] entries with fixed owner and time,
// so the same input packs to the same bytes.
function tar(entries) {
  const blocks = [];
  for (const [name, data, mode] of entries) {
    const h = Buffer.alloc(512);
    const field = (off, len, value) => h.write(value, off, len, 'ascii');
    const octal = (off, len, n) => field(off, len, `${n.toString(8).padStart(len - 1, '0')}\0`);
    field(0, 100, name);
    octal(100, 8, mode);
    octal(108, 8, 0);
    octal(116, 8, 0);
    octal(124, 12, data.length);
    octal(136, 12, 0);
    h.fill(' ', 148, 156);
    field(156, 1, '0');
    field(257, 6, 'ustar\0');
    field(263, 2, '00');
    let sum = 0;
    for (const b of h) sum += b;
    field(148, 8, `${sum.toString(8).padStart(6, '0')}\0 `);
    blocks.push(h, data, Buffer.alloc((512 - (data.length % 512)) % 512));
  }
  blocks.push(Buffer.alloc(1024));
  return Buffer.concat(blocks);
}

await mkdir(out, { recursive: true });
const plugins = [];
for (const dir of (await readdir('plugins', { withFileTypes: true })).filter((d) => d.isDirectory()).map((d) => d.name).sort()) {
  let pkg;
  try {
    pkg = JSON.parse(await readFile(join('plugins', dir, 'package.json'), 'utf8'));
  } catch {
    continue;
  }
  const m = pkg.coursive;
  const sources = m?.contributes?.sources ?? [];
  if (!sources.length) continue;
  // An icon is a Material Symbols name or an image in the package; the index
  // carries the image itself as a data: URL, covered by the index signature.
  const iconFile = ICON_TYPES[extname(m.icon ?? '').toLowerCase()];
  const icon = iconFile ? `data:${iconFile};base64,${(await readFile(join('plugins', dir, m.icon))).toString('base64')}` : m.icon;
  const artifacts = [];
  for (const platform of platforms) {
    const files = [['package.json', await readFile(join('plugins', dir, 'package.json')), 0o644]];
    for (const name of ['README.md', 'LICENSE', 'NOTICE', ...(iconFile ? [m.icon] : [])]) {
      try { files.push([name, await readFile(join('plugins', dir, name)), 0o644]); } catch {}
    }
    const exes = new Set(sources.map((s) => s.executable?.[platform]).filter(Boolean));
    for (const rel of exes) {
      const [goos, goarch] = TARGETS[platform];
      const target = join('plugins', dir, rel);
      execFileSync('go', ['build', '-trimpath', '-ldflags=-s -w', '-o', target, `./plugins/${dir}`], {
        stdio: 'inherit', env: { ...process.env, CGO_ENABLED: '0', GOOS: goos, GOARCH: goarch },
      });
      files.push([rel, await readFile(target), 0o755]);
    }
    const name = `${m.id}-${pkg.version}-${platform}.tgz`;
    const body = gzipSync(tar(files), { level: 9 });
    await writeFile(join(out, name), body);
    artifacts.push({
      platform,
      url: args.includes('--release') ? `https://github.com/${REPO}/releases/download/${m.id}-v${pkg.version}/${name}` : name,
      sha256: createHash('sha256').update(body).digest('hex'),
      size: (await stat(join(out, name))).size,
    });
  }
  plugins.push({
    id: m.id,
    title: m.title,
    summary: m.summary,
    ...(icon ? { icon } : {}),
    author: 'WALKERKILLER',
    homepage: `https://github.com/${REPO}/tree/main/plugins/${dir}`,
    categories: m.categories ?? ['source'],
    kinds: ['sources'],
    official: true,
    versions: [{ version: pkg.version, engine: m.engine, published: new Date().toISOString().slice(0, 10), artifacts }],
  });
}
await writeFile(join(out, 'index.json'), `${JSON.stringify({ schemaVersion: 1, generatedAt: new Date().toISOString(), plugins }, null, 2)}\n`);
console.log(`pack-sources: ${plugins.length} plugins, ${platforms.join(', ')} → ${out}`);
