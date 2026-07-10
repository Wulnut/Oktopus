#!/usr/bin/env node
/**
 * Writes public/version.json from repo VERSION + git commit.
 * Used at container start (dev) and before production builds.
 */
const fs = require('fs');
const path = require('path');
const { execSync } = require('child_process');

const frontendRoot = path.resolve(__dirname, '..');
const repoRoot = path.resolve(frontendRoot, '..');
const outPath = path.join(frontendRoot, 'public/version.json');

function readVersion() {
  if (process.env.OKTOPUS_VERSION) {
    return process.env.OKTOPUS_VERSION.trim();
  }
  for (const candidate of [
    path.join(repoRoot, 'VERSION'),
    path.join(frontendRoot, 'VERSION'),
  ]) {
    if (fs.existsSync(candidate)) {
      return fs.readFileSync(candidate, 'utf8').trim();
    }
  }
  try {
    const pkg = JSON.parse(fs.readFileSync(path.join(frontendRoot, 'package.json'), 'utf8'));
    return pkg.version || '0.0.0-dev';
  } catch {
    return '0.0.0-dev';
  }
}

function gitShort() {
  for (const cwd of [repoRoot, frontendRoot]) {
    try {
      return execSync('git rev-parse --short HEAD', { cwd, stdio: ['ignore', 'pipe', 'ignore'] })
        .toString()
        .trim();
    } catch {
      // try next
    }
  }
  return 'unknown';
}

const version = readVersion();
// Prefer explicit env (CI / Docker build-arg), then git, then any pre-stamped file
// from ci-pack-source.sh. Do not invent a commit — "unknown" is honest when none exist.
let commit = (process.env.OKTOPUS_GIT_COMMIT || gitShort() || '').trim();
let builtAt = process.env.OKTOPUS_BUILT_AT || '';
if (fs.existsSync(outPath)) {
  try {
    const prev = JSON.parse(fs.readFileSync(outPath, 'utf8'));
    if (!commit && prev.commit && prev.commit !== 'unknown') {
      commit = prev.commit;
    }
    if (!builtAt && prev.built_at) {
      builtAt = prev.built_at;
    }
  } catch {
    // ignore
  }
}
if (!commit) {
  commit = 'unknown';
}
if (!builtAt) {
  builtAt = new Date().toISOString().replace(/\.\d{3}Z$/, 'Z');
}
const data = {
  version,
  commit,
  built_at: builtAt,
  label: `v${version} (${commit})`,
};

fs.mkdirSync(path.dirname(outPath), { recursive: true });
fs.writeFileSync(outPath, `${JSON.stringify(data, null, 2)}\n`);
process.stdout.write(`wrote ${outPath}: ${data.label}\n`);
