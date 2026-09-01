#!/usr/bin/env node
/**
 * Scripted Stage 23 backup/restore verification (docs/backup-restore.md).
 *
 * The exhaustive malicious-package/decompression-bound/secret-exclusion
 * matrix is already covered directly at the Go level
 * (apps/server/internal/domain/backup/archive_test.go,
 * archive_gap_test.go, security_integration_test.go - 43 tests, several
 * against a real SQLite database and a real SecretStore). This script
 * instead exercises the real, wired-together HTTP surface end to end:
 * seed real configuration (including a real stream key, so its
 * exclusion is meaningful) -> export -> a representative malformed-
 * package rejection -> restore preview -> cancel-then-re-preview (never
 * trusts a prior preview) -> commit -> fresh local ids -> the restored
 * platform's stream key genuinely reads as not configured -> restart
 * persistence, plus a final byte-level scan of every captured HTTP body
 * and the backend's own log output for the seeded secret, per
 * docs/visual-template-packages.md's own "representative subset here"
 * precedent (verify-visual-template-packages.mjs).
 *
 * Starts the real backend against a TEMPORARY database - the real user
 * database is never opened, read, or modified.
 *
 * Usage: node scripts/verify-backup-restore.mjs
 * Exits non-zero on the first failed expectation.
 */

import { spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { deflateRawSync } from 'node:zlib';

const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const SERVER_DIR = join(REPO_ROOT, 'apps', 'server');

/** A port unlikely to collide with the dev server, a real backend, or another verify-*.mjs script. */
const PORT = 8211;
const BASE_URL = `http://127.0.0.1:${PORT}`;

const READINESS_TIMEOUT_MS = 30_000;
const SHUTDOWN_TIMEOUT_MS = 10_000;

let stepCount = 0;
const secretScanChunks = [];

function step(message) {
  stepCount += 1;
  console.log(`\n[${String(stepCount).padStart(2, '0')}] ${message}`);
}

function pass(message) {
  console.log(`     ok  ${message}`);
}

function expect(condition, message, detail) {
  if (condition) {
    pass(message);
    return;
  }
  console.error(`     FAIL ${message}`);
  if (detail !== undefined) {
    console.error(`          ${typeof detail === 'string' ? detail : JSON.stringify(detail)}`);
  }
  throw new Error(message);
}

function record(text) {
  if (typeof text === 'string' && text.length > 0) secretScanChunks.push(text);
}

async function request(method, path, body) {
  const options = { method, headers: { Accept: 'application/json' } };
  if (body !== undefined) {
    options.headers['Content-Type'] = 'application/json';
    options.body = JSON.stringify(body);
  }
  const response = await fetch(`${BASE_URL}${path}`, options);
  const text = await response.text();
  record(text);
  let payload = null;
  if (text !== '') {
    try {
      payload = JSON.parse(text);
    } catch {
      payload = text;
    }
  }
  return { status: response.status, headers: response.headers, body: payload };
}

/** Sends/receives a raw binary body - a backup archive, never JSON. */
async function requestRaw(method, path, { body, headers = {} } = {}) {
  const response = await fetch(`${BASE_URL}${path}`, { method, headers, body });
  const buf = Buffer.from(await response.arrayBuffer());
  let payload = null;
  const contentType = response.headers.get('content-type') ?? '';
  if (contentType.includes('application/json')) {
    const text = buf.toString('utf8');
    record(text);
    try {
      payload = JSON.parse(text);
    } catch {
      payload = text;
    }
  }
  return { status: response.status, body: payload, raw: buf, headers: response.headers };
}

async function startBackend(databasePath) {
  const child = spawn('go', ['run', './cmd/server'], {
    cwd: SERVER_DIR,
    env: {
      ...process.env,
      STREAMING_TREE_DB_PATH: databasePath,
      STREAMING_TREE_PORT: String(PORT),
      STREAMING_TREE_HOST: '127.0.0.1',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  });

  let output = '';
  const cap = (chunk) => {
    const text = chunk.toString();
    output += text;
    record(text);
  };
  child.stdout.on('data', cap);
  child.stderr.on('data', cap);

  let exited = false;
  child.on('exit', () => { exited = true; });

  const deadline = Date.now() + READINESS_TIMEOUT_MS;
  while (Date.now() < deadline) {
    if (exited) throw new Error(`backend exited during startup:\n${output}`);
    try {
      const health = await fetch(`${BASE_URL}/api/health`);
      if (health.ok) {
        child.getOutput = () => output;
        return child;
      }
    } catch {
      // Not listening yet.
    }
    // eslint-disable-next-line no-await-in-loop
    await new Promise((r) => setTimeout(r, 300));
  }
  child.kill();
  throw new Error(`backend did not become ready within ${READINESS_TIMEOUT_MS} ms:\n${output}`);
}

/**
 * `go run` spawns the compiled binary as a child, and on Windows a
 * signal to the wrapper does not always reach it, so the whole process
 * tree is killed and the port is then confirmed closed - the same
 * pattern verify-metadata-presets.mjs already established.
 */
async function stopBackend(child) {
  await new Promise((resolvePromise) => {
    const timer = setTimeout(() => resolvePromise(), SHUTDOWN_TIMEOUT_MS);
    child.on('exit', () => { clearTimeout(timer); resolvePromise(); });
    if (process.platform === 'win32') {
      spawn('taskkill', ['/pid', String(child.pid), '/T', '/F'], { stdio: 'ignore' });
    } else {
      child.kill('SIGTERM');
    }
  });

  const deadline = Date.now() + SHUTDOWN_TIMEOUT_MS;
  while (Date.now() < deadline) {
    try {
      await fetch(`${BASE_URL}/api/health`);
    } catch {
      return;
    }
    // eslint-disable-next-line no-await-in-loop
    await new Promise((r) => setTimeout(r, 200));
  }
  throw new Error('the backend is still answering after the shutdown request');
}

// --- tiny ZIP writer (no dependency), copied from
// verify-visual-template-packages.mjs's own established pattern -----------

const CRC_TABLE = (() => {
  const table = new Uint32Array(256);
  for (let n = 0; n < 256; n++) {
    let c = n;
    for (let k = 0; k < 8; k++) {
      c = (c & 1) !== 0 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    }
    table[n] = c >>> 0;
  }
  return table;
})();

function crc32(buf) {
  let crc = 0xffffffff;
  for (const byte of buf) {
    crc = CRC_TABLE[(crc ^ byte) & 0xff] ^ (crc >>> 8);
  }
  return (crc ^ 0xffffffff) >>> 0;
}

function buildZip(entries) {
  const localParts = [];
  const centralParts = [];
  let offset = 0;

  for (const { name, data } of entries) {
    const nameBuf = Buffer.from(name, 'utf8');
    const compressed = deflateRawSync(data);
    const useStore = compressed.length >= data.length;
    const method = useStore ? 0 : 8;
    const payload = useStore ? data : compressed;
    const crc = crc32(data);

    const localHeader = Buffer.alloc(30);
    localHeader.writeUInt32LE(0x04034b50, 0);
    localHeader.writeUInt16LE(20, 4);
    localHeader.writeUInt16LE(0, 6);
    localHeader.writeUInt16LE(method, 8);
    localHeader.writeUInt16LE(0, 10);
    localHeader.writeUInt16LE(0, 12);
    localHeader.writeUInt32LE(crc, 14);
    localHeader.writeUInt32LE(payload.length, 18);
    localHeader.writeUInt32LE(data.length, 22);
    localHeader.writeUInt16LE(nameBuf.length, 26);
    localHeader.writeUInt16LE(0, 28);
    localParts.push(localHeader, nameBuf, payload);

    const centralHeader = Buffer.alloc(46);
    centralHeader.writeUInt32LE(0x02014b50, 0);
    centralHeader.writeUInt16LE(20, 4);
    centralHeader.writeUInt16LE(20, 6);
    centralHeader.writeUInt16LE(0, 8);
    centralHeader.writeUInt16LE(method, 10);
    centralHeader.writeUInt16LE(0, 12);
    centralHeader.writeUInt16LE(0, 14);
    centralHeader.writeUInt32LE(crc, 16);
    centralHeader.writeUInt32LE(payload.length, 20);
    centralHeader.writeUInt32LE(data.length, 24);
    centralHeader.writeUInt16LE(nameBuf.length, 28);
    centralHeader.writeUInt16LE(0, 30);
    centralHeader.writeUInt16LE(0, 32);
    centralHeader.writeUInt16LE(0, 34);
    centralHeader.writeUInt16LE(0, 36);
    centralHeader.writeUInt32LE(0, 38);
    centralHeader.writeUInt32LE(offset, 42);
    centralParts.push(centralHeader, nameBuf);

    offset += localHeader.length + nameBuf.length + payload.length;
  }

  const centralStart = offset;
  const centralBuf = Buffer.concat(centralParts);
  const eocd = Buffer.alloc(22);
  eocd.writeUInt32LE(0x06054b50, 0);
  eocd.writeUInt16LE(0, 4);
  eocd.writeUInt16LE(0, 6);
  eocd.writeUInt16LE(entries.length, 8);
  eocd.writeUInt16LE(entries.length, 10);
  eocd.writeUInt32LE(centralBuf.length, 12);
  eocd.writeUInt32LE(centralStart, 16);
  eocd.writeUInt16LE(0, 20);

  return Buffer.concat([...localParts, centralBuf, eocd]);
}

function sha256Hex(buf) {
  return createHash('sha256').update(buf).digest('hex');
}

/** A well-formed but wrong-product backup package - rejected before
 * config.json is ever parsed, a representative malformed-package case
 * at the HTTP level (the exhaustive matrix lives in the Go tests). */
function buildWrongProductPackage() {
  const config = Buffer.from(JSON.stringify({ formatVersion: 1 }));
  const manifest = Buffer.from(JSON.stringify({
    formatVersion: 1,
    product: 'some-other-application-backup',
    createdAt: new Date().toISOString(),
    sourceAppVersion: '0.0.0',
    sourcePlatform: 'linux',
    configSha256: sha256Hex(config),
    configSizeBytes: config.length,
  }));
  return buildZip([
    { name: 'manifest.json', data: manifest },
    { name: 'config.json', data: config },
  ]);
}

const SEEDED_STREAM_KEY = `verify-backup-restore-sentinel-${Date.now()}`;

async function main() {
  const tempDir = mkdtempSync(join(tmpdir(), 'streaming-tree-verify-backup-'));
  const databasePath = join(tempDir, 'verify.db');

  console.log('Scripted Stage 23 backup/restore verification');
  console.log(`Temporary database: ${databasePath}`);
  console.log('The real user database is never touched.');

  let backend = null;
  let originalOverlayId = null;
  let originalPresetId = null;
  let restoredOverlayId = null;
  let restoredPresetId = null;
  let restoredPlatformId = null;

  try {
    step('Start the backend against the temporary database');
    backend = await startBackend(databasePath);
    pass('backend is ready');

    step('Seed a real stream key on the demo Twitch destination');
    const setKey = await request('PUT', '/api/platforms/pf_seed_twitch/credentials/stream-key', {
      streamKey: SEEDED_STREAM_KEY,
    });
    expect(setKey.status === 200, 'stream key set', setKey.body);

    step('Give the demo Twitch destination some real metadata');
    const meta = await request('PUT', '/api/platforms/pf_seed_twitch/metadata', {
      title: 'Backup/restore verification stream', description: '', category: 'Just Chatting', categoryId: '509658',
      tags: ['verify'], language: 'en', visibility: '', matureContent: false, dvr: false, latencyMode: '',
    });
    expect(meta.status === 200, 'metadata set', meta.body);

    step('Create a real chat overlay and a real metadata preset');
    const overlay = await request('POST', '/api/chat-overlays', { name: 'Verify overlay' });
    expect(overlay.status === 200, 'chat overlay created', overlay.body);
    originalOverlayId = overlay.body.id;

    const preset = await request('POST', '/api/metadata-presets', {
      name: 'Verify preset', note: '', title: 'Preset title', description: '', tags: [], language: 'en',
      visibility: '', matureContent: false, dvr: false, latencyMode: '',
      providers: { twitch: { category: 'Just Chatting', categoryId: '509658' } },
    });
    expect(preset.status === 201, 'metadata preset created', preset.body);
    originalPresetId = preset.body.id;

    // --- export --------------------------------------------------------

    step('Export a real backup package');
    const exported = await requestRaw('POST', '/api/backup/export');
    expect(exported.status === 200, 'export succeeds', exported.status);
    expect(exported.headers.get('content-type') === 'application/zip', 'Content-Type is application/zip', exported.headers.get('content-type'));
    const disposition = exported.headers.get('content-disposition') ?? '';
    expect(disposition.includes('attachment'), 'Content-Disposition is an attachment', disposition);
    expect(disposition.includes('.streaming-tree-backup'), 'filename uses the backup extension', disposition);
    expect(exported.raw.length > 100, 'the exported package has real content', exported.raw.length);
    expect(exported.raw.subarray(0, 2).toString('latin1') === 'PK', 'the exported package is a real ZIP archive', exported.raw.subarray(0, 4));

    step('The exported package never contains the real stream key');
    expect(!exported.raw.includes(SEEDED_STREAM_KEY), 'stream key sentinel is absent from the exported bytes', undefined);

    // --- a representative malformed-package rejection -------------------

    step('A well-formed but wrong-product package is rejected');
    const wrongProduct = await requestRaw('POST', '/api/backup/restore/preview', {
      body: buildWrongProductPackage(),
      headers: { 'Content-Type': 'application/octet-stream' },
    });
    expect(wrongProduct.status === 422, 'wrong-product package is rejected with a 4xx', wrongProduct.status);

    // --- preview, cancel, re-preview (never trusts a prior preview) ----

    step('Preview restoring the real exported package');
    const preview = await requestRaw('POST', '/api/backup/restore/preview', {
      body: exported.raw,
      headers: { 'Content-Type': 'application/octet-stream' },
    });
    expect(preview.status === 200, 'restore preview succeeds', preview.body);
    expect(typeof preview.body.token === 'string' && preview.body.token.length > 0, 'preview carries a token', preview.body.token);
    expect(preview.body.counts.platforms >= 1, 'preview counts at least one platform', preview.body.counts);
    expect(preview.body.counts.chatOverlays >= 1, 'preview counts at least one chat overlay', preview.body.counts);
    expect(preview.body.counts.metadataPresets >= 1, 'preview counts at least one metadata preset', preview.body.counts);
    expect(preview.body.destinationsNeedStreamKey >= 1, 'preview reports at least one destination will need its stream key re-entered', preview.body);

    step('Cancel the preview, then a fresh preview of the SAME bytes still works (re-validates from scratch)');
    const cancel = await request('DELETE', `/api/backup/restore/preview/${preview.body.token}`);
    expect(cancel.status === 204, 'cancel succeeds', cancel.status);
    const secondPreview = await requestRaw('POST', '/api/backup/restore/preview', {
      body: exported.raw,
      headers: { 'Content-Type': 'application/octet-stream' },
    });
    expect(secondPreview.status === 200, 'a fresh preview after cancelling the first still succeeds', secondPreview.body);
    expect(secondPreview.body.token !== preview.body.token, 'the fresh preview gets its own token', { first: preview.body.token, second: secondPreview.body.token });

    // --- commit ----------------------------------------------------------

    step('Commit the restore');
    const commit = await request('POST', `/api/backup/restore/commit/${secondPreview.body.token}`);
    expect(commit.status === 200, 'restore commit succeeds', commit.body);
    expect(commit.body.restartRequired === true, 'RestoreResult.restartRequired is true', commit.body);
    expect(commit.body.counts.chatOverlays >= 1, 'commit counts at least one restored chat overlay', commit.body.counts);

    step('The restored chat overlay and metadata preset exist under FRESH local ids');
    const overlaysAfter = await request('GET', '/api/chat-overlays');
    expect(overlaysAfter.status === 200, 'list chat overlays succeeds', overlaysAfter.status);
    const restoredOverlay = overlaysAfter.body.items.find((o) => o.name === 'Verify overlay');
    expect(restoredOverlay !== undefined, 'the restored chat overlay is present by name', overlaysAfter.body);
    restoredOverlayId = restoredOverlay.id;
    expect(restoredOverlayId !== originalOverlayId, 'the restored chat overlay got a fresh id, not the original one', { original: originalOverlayId, restored: restoredOverlayId });

    const presetsAfter = await request('GET', '/api/metadata-presets');
    expect(presetsAfter.status === 200, 'list metadata presets succeeds', presetsAfter.status);
    const restoredPreset = presetsAfter.body.find((p) => p.name === 'Verify preset');
    expect(restoredPreset !== undefined, 'the restored metadata preset is present by name', presetsAfter.body);
    restoredPresetId = restoredPreset.id;
    expect(restoredPresetId !== originalPresetId, 'the restored metadata preset got a fresh id, not the original one', { original: originalPresetId, restored: restoredPreset.id });

    step('The restored destination genuinely reports its stream key as NOT configured');
    const platformsAfter = await request('GET', '/api/platforms');
    expect(platformsAfter.status === 200, 'list platforms succeeds', platformsAfter.status);
    const restoredPlatform = platformsAfter.body.platforms.find((p) => p.providerId === 'twitch');
    expect(restoredPlatform !== undefined, 'a Twitch destination exists after restore', platformsAfter.body);
    restoredPlatformId = restoredPlatform.id;
    const credStatus = await request('GET', `/api/platforms/${restoredPlatformId}/credentials`);
    expect(credStatus.status === 200, 'credential status request succeeds', credStatus.body);
    expect(credStatus.body.streamKey.configured === false, 'the restored destination has NO stream key configured - restore never restores secrets', credStatus.body);

    // --- restart persistence ---------------------------------------------

    step('Restart the backend: the restored configuration survives');
    await stopBackend(backend);
    backend = null;
    backend = await startBackend(databasePath);
    const overlayAfterRestart = await request('GET', `/api/chat-overlays/${restoredOverlayId}`);
    expect(overlayAfterRestart.status === 200, 'the restored chat overlay survived the restart', overlayAfterRestart.body);
    const presetAfterRestart = await request('GET', `/api/metadata-presets/${restoredPresetId}`);
    expect(presetAfterRestart.status === 200, 'the restored metadata preset survived the restart', presetAfterRestart.body);

    step("Search every captured HTTP response body and the backend's own stdout/stderr for the seeded secret");
    const haystack = secretScanChunks.join('\n');
    expect(!haystack.includes(SEEDED_STREAM_KEY), 'the seeded stream key never appears in any captured HTTP body or backend log line', undefined);
    pass(`scanned ${haystack.length} bytes of HTTP bodies and backend output`);

    console.log('\nAll steps passed.');
  } catch (error) {
    if (backend !== null && process.env.STREAMING_TREE_VERIFY_DEBUG === '1') {
      console.error('\n--- backend output (debug) ---');
      console.error(backend.getOutput?.() ?? '(unavailable)');
      console.error('--- end backend output ---\n');
    }
    throw error;
  } finally {
    if (backend !== null) {
      await stopBackend(backend).catch(() => undefined);
    }
    rmSync(tempDir, { recursive: true, force: true });
    console.log(`Removed the temporary root: ${tempDir}`);
  }
}

main().catch((error) => {
  console.error(`\nBackup/restore verification FAILED: ${error.message}`);
  process.exitCode = 1;
});
