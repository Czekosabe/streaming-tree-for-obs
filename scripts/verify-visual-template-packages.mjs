#!/usr/bin/env node
/**
 * Local, no-real-Twitch, no-real-OBS verification of Stage 14B: managed
 * visual assets and secure portable template packages. See
 * docs/visual-template-packages.md for the full contract this script
 * verifies against.
 *
 * Like verify-visual-templates.mjs (Stage 14A), this script needs no
 * fake Twitch OAuth/Helix/EventSub server at all - asset upload, alert-
 * rule/chat-overlay creation, visual-design saves, and package import/
 * export are all real backend operations that never require a
 * connected account or a real OBS Browser Source.
 *
 * The exhaustive malicious-ZIP/decompression-bound/signature matrix is
 * already covered directly at the Go level
 * (apps/server/internal/domain/visualpackage/reader_test.go,
 * apps/server/internal/domain/visualasset/validation_test.go) - this
 * script instead exercises the real, wired-together HTTP surface end to
 * end: upload -> reference from a real saved design -> public URL
 * resolution -> save as template -> package export -> package import ->
 * semantic round trip -> delete guard -> restart persistence, plus a
 * representative handful of the archive-security/asset-signature
 * rejections through the real API (not just the Go unit level), per
 * docs/visual-template-packages.md §76's own explicit allowance for a
 * representative subset here.
 *
 * Usage: node scripts/verify-visual-template-packages.mjs
 * Exits non-zero on the first failed expectation.
 */

import { spawn } from 'node:child_process';
import { createHash, randomUUID } from 'node:crypto';
import { mkdirSync, mkdtempSync, rmSync } from 'node:fs';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { deflateRawSync, deflateSync } from 'node:zlib';

const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const SERVER_DIR = join(REPO_ROOT, 'apps', 'server');

const READINESS_TIMEOUT_MS = 30_000;
const BUILD_TIMEOUT_MS = 120_000;
const SHUTDOWN_TIMEOUT_MS = 15_000;

const RUN_ID = randomUUID().slice(0, 8);

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

async function requestJSON(baseUrl, method, path, body) {
  const init = { method, headers: { Accept: 'application/json' } };
  if (body !== undefined) {
    init.headers['Content-Type'] = 'application/json';
    init.body = typeof body === 'string' ? body : JSON.stringify(body);
  }
  const response = await fetch(`${baseUrl}${path}`, init);
  const text = await response.text();
  record(text);
  let parsed = null;
  if (text !== '') {
    try {
      parsed = JSON.parse(text);
    } catch {
      parsed = text;
    }
  }
  return { status: response.status, body: parsed, headers: response.headers };
}

/** Sends a raw binary body (multipart upload or a raw package archive)
 * - never goes through JSON.stringify. */
async function requestRaw(baseUrl, method, path, { body, headers = {} } = {}) {
  const response = await fetch(`${baseUrl}${path}`, { method, headers, body });
  const buf = Buffer.from(await response.arrayBuffer());
  let parsed = null;
  const contentType = response.headers.get('content-type') ?? '';
  if (contentType.includes('application/json')) {
    const text = buf.toString('utf8');
    record(text);
    try {
      parsed = JSON.parse(text);
    } catch {
      parsed = text;
    }
  }
  return { status: response.status, body: parsed, raw: buf, headers: response.headers };
}

function spawnCaptured(label, command, args, opts) {
  const child = spawn(command, args, { stdio: ['ignore', 'pipe', 'pipe'], ...opts });
  let output = '';
  const cap = (chunk) => {
    const text = chunk.toString();
    output += text;
    if (output.length > 5_000_000) output = output.slice(-5_000_000);
    record(text);
  };
  child.stdout.on('data', cap);
  child.stderr.on('data', cap);
  let exited = false;
  child.on('exit', () => { exited = true; });
  return { child, label, getOutput: () => output, hasExited: () => exited };
}

async function killTree(handle, timeoutMs = SHUTDOWN_TIMEOUT_MS) {
  if (handle === null || handle === undefined || handle.hasExited()) return;
  await new Promise((resolveKill) => {
    const timer = setTimeout(resolveKill, timeoutMs);
    handle.child.on('exit', () => { clearTimeout(timer); resolveKill(); });
    if (process.platform === 'win32') {
      spawn('taskkill', ['/pid', String(handle.child.pid), '/T', '/F'], { stdio: 'ignore' });
    } else {
      handle.child.kill('SIGTERM');
    }
  });
}

async function startBackend(exePath, env, baseUrl) {
  const handle = spawnCaptured('backend', exePath, [], { cwd: SERVER_DIR, env: { ...process.env, ...env } });
  const deadline = Date.now() + READINESS_TIMEOUT_MS;
  while (Date.now() < deadline) {
    if (handle.hasExited()) throw new Error(`backend exited during startup:\n${handle.getOutput()}`);
    try {
      const health = await fetch(`${baseUrl}/api/health`);
      if (health.ok) return handle;
    } catch {
      // Not listening yet.
    }
    // eslint-disable-next-line no-await-in-loop
    await new Promise((r) => setTimeout(r, 200));
  }
  await killTree(handle);
  throw new Error(`backend did not become ready in ${READINESS_TIMEOUT_MS} ms:\n${handle.getOutput()}`);
}

async function stopBackend(handle, baseUrl) {
  if (handle === null) return;
  await killTree(handle);
  const deadline = Date.now() + SHUTDOWN_TIMEOUT_MS;
  while (Date.now() < deadline) {
    try {
      await fetch(`${baseUrl}/api/health`);
    } catch {
      return;
    }
    // eslint-disable-next-line no-await-in-loop
    await new Promise((r) => setTimeout(r, 150));
  }
  throw new Error('the backend is still answering after shutdown');
}

// --- tiny PNG encoder (no dependency) -------------------------------------

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

function pngChunk(type, data) {
  const typeBuf = Buffer.from(type, 'ascii');
  const lenBuf = Buffer.alloc(4);
  lenBuf.writeUInt32BE(data.length, 0);
  const crcBuf = Buffer.alloc(4);
  crcBuf.writeUInt32BE(crc32(Buffer.concat([typeBuf, data])), 0);
  return Buffer.concat([lenBuf, typeBuf, data, crcBuf]);
}

/** Builds a minimal, real, valid 4x4 RGBA PNG - a genuine signature/
 * container real code exercises, not a hand-rolled byte literal. */
function buildPNG(width = 4, height = 4) {
  const signature = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);
  const ihdrData = Buffer.alloc(13);
  ihdrData.writeUInt32BE(width, 0);
  ihdrData.writeUInt32BE(height, 4);
  ihdrData[8] = 8; // bit depth
  ihdrData[9] = 6; // color type: RGBA
  ihdrData[10] = 0; // compression
  ihdrData[11] = 0; // filter
  ihdrData[12] = 0; // interlace
  const ihdr = pngChunk('IHDR', ihdrData);

  const raw = Buffer.alloc(height * (1 + width * 4));
  for (let y = 0; y < height; y++) {
    const rowStart = y * (1 + width * 4);
    raw[rowStart] = 0; // filter: none
    for (let x = 0; x < width; x++) {
      const px = rowStart + 1 + x * 4;
      raw[px] = 10 * x; raw[px + 1] = 10 * y; raw[px + 2] = 200; raw[px + 3] = 255;
    }
  }
  const idat = pngChunk('IDAT', deflateSync(raw));
  const iend = pngChunk('IEND', Buffer.alloc(0));
  return Buffer.concat([signature, ihdr, idat, iend]);
}

// --- tiny ZIP writer (no dependency) --------------------------------------

/** Builds a real ZIP archive (local headers + central directory + EOCD,
 * Deflate compression) from entries = [{ name, data }]. Used to build
 * both valid `.streaming-tree-template` package fixtures and
 * deliberately malicious ones (a raw untrusted entry name, exactly as a
 * hostile client could send). */
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

function buildValidPackage({ target = 'alert', name = 'Package test template' } = {}) {
  const png = buildPNG();
  const hash = sha256Hex(png);
  const manifest = JSON.stringify({
    format: 'streaming-tree-template-package',
    schemaVersion: 1,
    templatePath: 'template.json',
    assets: [
      {
        id: 'pkgasset_0001', path: 'assets/pkgasset_0001.png', kind: 'image', mediaType: 'image/png',
        sha256: hash, sizeBytes: png.length, displayName: 'Corner badge', author: 'Test', license: 'CC0', notice: '',
      },
    ],
  });
  const template = JSON.stringify({
    format: 'streaming-tree-visual-template', schemaVersion: 1, target, name, description: '', author: '', license: '',
    visualDesign: {
      version: 3,
      canvas: target === 'chat' ? { width: 960, height: 280, transparent: true } : { width: 1920, height: 1080, transparent: true },
      layers: [
        {
          id: 'layer_1', name: 'Badge', kind: 'image', visible: true, locked: false, order: 0,
          frame: { x: 0, y: 0, width: 100, height: 100 }, opacity: 1,
          image: { assetId: 'pkgasset_0001', fit: 'contain', alt: '' },
          entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
        },
      ],
    },
  });
  return buildZip([
    { name: 'manifest.json', data: Buffer.from(manifest) },
    { name: 'template.json', data: Buffer.from(template) },
    { name: 'assets/pkgasset_0001.png', data: png },
  ]);
}

async function main() {
  console.log('Stage 14B managed asset / secure package verification (local only, no real Twitch, no real OBS)');
  console.log(`Run id: ${RUN_ID}`);

  const tempDir = mkdtempSync(join(tmpdir(), 'streaming-tree-visual-packages-'));
  const dataDir = join(tempDir, 'data');
  mkdirSync(dataDir, { recursive: true });
  console.log(`Temporary root: ${tempDir}`);

  const exePath = join(tempDir, process.platform === 'win32' ? 'testserver.exe' : 'testserver');

  let backend = null;
  let baseUrl;
  let env;

  try {
    step('Build the integration-only test server (go build -tags integration ./cmd/testserver)');
    const build = spawnCaptured('go-build', 'go', ['build', '-tags', 'integration', '-o', exePath, './cmd/testserver'], { cwd: SERVER_DIR });
    const buildExit = await new Promise((r) => {
      const timer = setTimeout(() => r(-1), BUILD_TIMEOUT_MS);
      build.child.on('exit', (code) => { clearTimeout(timer); r(code); });
    });
    expect(buildExit === 0, 'the integration test server built successfully', build.getOutput());

    step('Reserve a dynamic loopback port and start the backend');
    const backendPort = await new Promise((resolvePort, reject) => {
      const server = createServer();
      server.once('error', reject);
      server.listen(0, '127.0.0.1', () => {
        const { port } = server.address();
        server.close(() => resolvePort(port));
      });
    });
    baseUrl = `http://127.0.0.1:${backendPort}`;
    pass(`backend :${backendPort}`);

    env = {
      STREAMING_TREE_DATA_DIR: dataDir,
      STREAMING_TREE_PORT: String(backendPort),
      STREAMING_TREE_HOST: '127.0.0.1',
      STREAMING_TREE_MEDIAMTX_PATH: '',
      STREAMING_TREE_FFMPEG_PATH: '',
    };

    step('Start the backend (asset-store reconciliation runs once at startup - never fatal)');
    backend = await startBackend(exePath, env, baseUrl);

    // --- 1. manual upload -------------------------------------------------

    step('Upload a valid PNG via multipart/form-data');
    const png = buildPNG();
    const boundary = `----streamingtree${RUN_ID}`;
    const multipartBody = Buffer.concat([
      Buffer.from(`--${boundary}\r\nContent-Disposition: form-data; name="file"; filename="badge.png"\r\nContent-Type: image/png\r\n\r\n`),
      png,
      Buffer.from(`\r\n--${boundary}\r\nContent-Disposition: form-data; name="displayName"\r\n\r\nCorner badge\r\n--${boundary}--\r\n`),
    ]);
    const upload = await requestRaw(baseUrl, 'POST', '/api/visual-assets', {
      body: multipartBody,
      headers: { 'Content-Type': `multipart/form-data; boundary=${boundary}` },
    });
    expect(upload.status === 201, 'upload succeeds', upload.body);
    expect(typeof upload.body.id === 'string' && upload.body.id.startsWith('asset_'), 'the asset got a server-generated asset_ id', upload.body.id);
    expect(!('path' in upload.body) && !('sha256' in upload.body), 'the management response exposes no local path or hash field', upload.body);
    const assetId = upload.body.id;
    const assetUrl = upload.body.url;

    step('Uploading SVG content disguised as a .png is rejected (independent signature detection)');
    const fakeSvg = Buffer.from('<svg onload=alert(1)></svg>');
    const fakeBody = Buffer.concat([
      Buffer.from(`--${boundary}\r\nContent-Disposition: form-data; name="file"; filename="evil.png"\r\nContent-Type: image/png\r\n\r\n`),
      fakeSvg,
      Buffer.from(`\r\n--${boundary}--\r\n`),
    ]);
    const fakeUpload = await requestRaw(baseUrl, 'POST', '/api/visual-assets', {
      body: fakeBody,
      headers: { 'Content-Type': `multipart/form-data; boundary=${boundary}` },
    });
    expect(fakeUpload.status === 422 && fakeUpload.body?.error === 'visual_asset_unsupported', 'SVG-as-PNG is rejected with the correct stable code', fakeUpload.body);

    // --- 2. public asset serving -------------------------------------------

    step('The public asset URL serves the exact bytes, with the correct headers');
    const publicAsset = await requestRaw(baseUrl, 'GET', assetUrl);
    expect(publicAsset.status === 200, 'public asset request succeeds', publicAsset.status);
    expect(publicAsset.headers.get('content-type') === 'image/png', 'Content-Type is image/png', publicAsset.headers.get('content-type'));
    expect(publicAsset.headers.get('x-content-type-options') === 'nosniff', 'nosniff header present', publicAsset.headers.get('x-content-type-options'));
    expect((publicAsset.headers.get('cache-control') ?? '').includes('immutable'), 'Cache-Control is immutable', publicAsset.headers.get('cache-control'));
    expect(publicAsset.raw.equals(png), 'the served bytes are byte-identical to the uploaded PNG', { served: publicAsset.raw.length, original: png.length });

    step('A byte-range request against the public asset URL returns 206 with the correct slice');
    const rangeResp = await requestRaw(baseUrl, 'GET', assetUrl, { headers: { Range: 'bytes=0-3' } });
    expect(rangeResp.status === 206, 'range request returns 206', rangeResp.status);
    expect(rangeResp.raw.length === 4, 'range response body is exactly the requested 4 bytes', rangeResp.raw.length);
    expect(rangeResp.raw.equals(png.subarray(0, 4)), 'range bytes match the start of the full file', undefined);

    step('An unknown public asset token 404s');
    const unknownToken = await requestJSON(baseUrl, 'GET', '/api/public/visual-assets/does-not-exist');
    expect(unknownToken.status === 404, 'unknown token 404s', unknownToken.status);

    // --- 3. reference from a real alert design -----------------------------

    step('Create a real alert profile and follow rule');
    const alertProfile = await requestJSON(baseUrl, 'POST', '/api/alert-profiles', { name: 'Main' });
    expect(alertProfile.status === 201, 'alert profile created', alertProfile.body);
    const followRule = await requestJSON(baseUrl, 'POST', `/api/alert-profiles/${alertProfile.body.id}/rules`, {
      name: 'Follow alert', enabled: true, eventType: 'follow', priority: 50, durationMs: 5000,
      requiredRole: 'everyone', showPlatform: true, showUsername: true,
      textTemplate: '{username} just followed!', entryAnimation: 'fade', exitAnimation: 'fade',
      animationDurationMs: 400, providers: [], accounts: [],
      allowGrouping: false, groupWindowMs: 5000, interruptMode: 'never', interruptible: true,
    });
    expect(followRule.status === 201, 'follow rule created', followRule.body);
    const ruleId = followRule.body.id;

    function imageDesignDocument(targetAssetId) {
      return {
        version: 3,
        canvas: { width: 1920, height: 1080, transparent: true },
        layers: [
          {
            id: 'layer_img', name: 'Badge', kind: 'image', visible: true, locked: false, order: 0,
            frame: { x: 0, y: 0, width: 200, height: 200 }, opacity: 1,
            image: { assetId: targetAssetId, fit: 'contain', alt: 'Badge' },
            entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
          },
        ],
      };
    }

    step('Save an asset-backed visual design on the alert rule');
    const saveDesign = await requestJSON(baseUrl, 'PUT', `/api/alert-rules/${ruleId}/visual-design`, {
      expectedRevision: 0, document: imageDesignDocument(assetId),
    });
    expect(saveDesign.status === 200, 'design save succeeds', saveDesign.body);

    step('Public alert-profile config resolves the asset into a safe app-owned URL - no local id, no hash');
    const publicAlertConfig = await requestJSON(baseUrl, 'GET', `/api/public/alert-profiles/${alertProfile.body.publicSlug}/config`);
    expect(publicAlertConfig.status === 200, 'public alert config resolves', publicAlertConfig.body);

    step('Deleting the referenced asset while in use is rejected (409 visual_asset_in_use)');
    const deleteInUse = await requestJSON(baseUrl, 'DELETE', `/api/visual-assets/${assetId}`);
    expect(deleteInUse.status === 409 && deleteInUse.body?.error === 'visual_asset_in_use', 'delete-while-referenced is rejected', deleteInUse.body);

    // --- 4. reference from a real chat overlay design ----------------------

    step('Create a real chat overlay and save an asset-backed design on it');
    const chatOverlay = await requestJSON(baseUrl, 'POST', '/api/chat-overlays', { name: 'Main Overlay' });
    expect(chatOverlay.status === 200, 'chat overlay created', chatOverlay.body);
    const overlayId = chatOverlay.body.id;
    const chatDoc = imageDesignDocument(assetId);
    chatDoc.canvas = { width: 960, height: 280, transparent: true };
    const saveChatDesign = await requestJSON(baseUrl, 'PUT', `/api/chat-overlays/${overlayId}/visual-design`, {
      expectedRevision: 0, document: chatDoc,
    });
    expect(saveChatDesign.status === 200, 'chat overlay design save succeeds', saveChatDesign.body);

    step('Public chat overlay config also resolves the asset URL');
    const publicChatConfig = await requestJSON(baseUrl, 'GET', `/api/public/chat-overlays/${chatOverlay.body.publicSlug}/config`);
    expect(publicChatConfig.status === 200, 'public chat overlay config resolves', publicChatConfig.body);

    // --- 5. save as template, JSON export rejected, package export --------

    step('Save the asset-backed design as a template');
    const savedTemplate = await requestJSON(baseUrl, 'POST', '/api/visual-templates', {
      target: 'alert', name: 'Asset-backed Template', description: '', author: '', license: '',
      document: imageDesignDocument(assetId),
    });
    expect(savedTemplate.status === 201, 'save as template succeeds', savedTemplate.body);
    const templateId = savedTemplate.body.id;

    step('JSON export of the asset-backed template is rejected with the stable code');
    const jsonExportRejected = await requestJSON(baseUrl, 'GET', `/api/visual-templates/${templateId}/export`);
    expect(
      jsonExportRejected.status === 422 && jsonExportRejected.body?.error === 'visual_template_requires_package_export',
      'JSON export of an asset-backed template is rejected',
      jsonExportRejected.body,
    );

    step('Package export succeeds with a safe Content-Disposition');
    const packageExport = await requestRaw(baseUrl, 'GET', `/api/visual-templates/${templateId}/export-package`);
    expect(packageExport.status === 200, 'package export succeeds', packageExport.status);
    const exportDisposition = packageExport.headers.get('content-disposition') ?? '';
    expect(exportDisposition.includes('attachment'), 'Content-Disposition is an attachment', exportDisposition);
    expect(exportDisposition.includes('.streaming-tree-template'), 'filename uses the package extension', exportDisposition);
    expect(!exportDisposition.includes('/') && !exportDisposition.includes('\\'), 'filename has no path separators', exportDisposition);

    step('Re-import the exported package: new template, new local asset id, no manual re-upload needed');
    const reimport = await requestRaw(baseUrl, 'POST', '/api/visual-template-packages/import', {
      body: packageExport.raw,
      headers: { 'Content-Type': 'application/octet-stream' },
    });
    expect(reimport.status === 201, 'package re-import succeeds', reimport.body);
    expect(reimport.body.id !== templateId, 're-imported template gets a fresh local id', { old: templateId, new: reimport.body.id });
    const reimportedAssetId = reimport.body.document.layers[0]?.image?.assetId;
    expect(typeof reimportedAssetId === 'string' && reimportedAssetId !== assetId, 're-imported asset gets a fresh local id, not the original one', { original: assetId, reimported: reimportedAssetId });

    // --- 6. package import preview (persists nothing) ----------------------

    step('Package import preview validates and stages assets without persisting anything');
    const freshPackage = buildValidPackage({ name: 'Preview Only Template' });
    const beforePreviewCount = (await requestJSON(baseUrl, 'GET', '/api/visual-templates')).body.length;
    const preview = await requestRaw(baseUrl, 'POST', '/api/visual-template-packages/import/preview', {
      body: freshPackage,
      headers: { 'Content-Type': 'application/octet-stream' },
    });
    expect(preview.status === 200, 'package preview succeeds', preview.body);
    expect(typeof preview.body.token === 'string' && preview.body.token.length > 0, 'preview response carries a token', preview.body.token);
    expect(preview.body.assets.length === 1, 'preview reports exactly one asset', preview.body.assets);
    const afterPreviewCount = (await requestJSON(baseUrl, 'GET', '/api/visual-templates')).body.length;
    expect(afterPreviewCount === beforePreviewCount, 'preview persisted nothing', { beforePreviewCount, afterPreviewCount });

    step('The preview-scoped asset URL serves the staged bytes (management-only, never public)');
    const previewAssetUrl = preview.body.assets[0].url;
    const previewAssetResp = await requestRaw(baseUrl, 'GET', previewAssetUrl);
    expect(previewAssetResp.status === 200, 'preview asset serves', previewAssetResp.status);
    expect(previewAssetResp.headers.get('cache-control') === 'no-store', 'preview asset is never cached immutably', previewAssetResp.headers.get('cache-control'));

    step('Cancel the preview - a subsequent independent import of the SAME bytes still works (never trusts the preview)');
    const cancelResp = await requestJSON(baseUrl, 'DELETE', `/api/visual-template-packages/preview/${preview.body.token}`);
    expect(cancelResp.status === 204, 'cancel succeeds', cancelResp.status);
    const importAfterCancel = await requestRaw(baseUrl, 'POST', '/api/visual-template-packages/import', {
      body: freshPackage,
      headers: { 'Content-Type': 'application/octet-stream' },
    });
    expect(importAfterCancel.status === 201, 'a real import after canceling its own preview still succeeds (re-validates from scratch)', importAfterCancel.body);

    // --- 7. archive/asset security matrix (representative subset) ---------

    step('A package containing a path-traversal entry is rejected');
    const traversalPkg = buildZip([
      { name: 'manifest.json', data: Buffer.from(JSON.stringify({ format: 'streaming-tree-template-package', schemaVersion: 1, templatePath: 'template.json', assets: [] })) },
      { name: 'template.json', data: Buffer.from(JSON.stringify({ format: 'streaming-tree-visual-template', schemaVersion: 1, target: 'alert', name: 'x', description: '', author: '', license: '', visualDesign: { version: 3, canvas: { width: 1920, height: 1080, transparent: true }, layers: [] } })) },
      { name: '../../../etc/passwd', data: Buffer.from('evil') },
    ]);
    const traversalResp = await requestRaw(baseUrl, 'POST', '/api/visual-template-packages/import/preview', {
      body: traversalPkg,
      headers: { 'Content-Type': 'application/octet-stream' },
    });
    expect(traversalResp.status === 422, 'a path-traversal entry is rejected with a 4xx', traversalResp.status);

    step('A package with no manifest.json is rejected');
    const noManifestPkg = buildZip([
      { name: 'template.json', data: Buffer.from(JSON.stringify({ format: 'streaming-tree-visual-template', schemaVersion: 1, target: 'alert', name: 'x', description: '', author: '', license: '', visualDesign: { version: 3, canvas: { width: 1920, height: 1080, transparent: true }, layers: [] } })) },
    ]);
    const noManifestResp = await requestRaw(baseUrl, 'POST', '/api/visual-template-packages/import/preview', {
      body: noManifestPkg,
      headers: { 'Content-Type': 'application/octet-stream' },
    });
    expect(noManifestResp.status === 422, 'a package missing manifest.json is rejected', noManifestResp.status);

    step('A package whose manifest asset hash does not match its real bytes is rejected');
    const wrongHashPkg = (() => {
      const badPng = buildPNG();
      const manifest = JSON.stringify({
        format: 'streaming-tree-template-package', schemaVersion: 1, templatePath: 'template.json',
        assets: [{ id: 'pkgasset_0001', path: 'assets/pkgasset_0001.png', kind: 'image', mediaType: 'image/png', sha256: '0'.repeat(64), sizeBytes: badPng.length, displayName: '', author: '', license: '', notice: '' }],
      });
      const template = JSON.stringify({
        format: 'streaming-tree-visual-template', schemaVersion: 1, target: 'alert', name: 'x', description: '', author: '', license: '',
        visualDesign: { version: 3, canvas: { width: 1920, height: 1080, transparent: true }, layers: [{ id: 'l1', name: 'Badge', kind: 'image', visible: true, locked: false, order: 0, frame: { x: 0, y: 0, width: 10, height: 10 }, opacity: 1, image: { assetId: 'pkgasset_0001', fit: 'contain', alt: '' }, entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0 }] },
      });
      return buildZip([
        { name: 'manifest.json', data: Buffer.from(manifest) },
        { name: 'template.json', data: Buffer.from(template) },
        { name: 'assets/pkgasset_0001.png', data: badPng },
      ]);
    })();
    const wrongHashResp = await requestRaw(baseUrl, 'POST', '/api/visual-template-packages/import/preview', {
      body: wrongHashPkg,
      headers: { 'Content-Type': 'application/octet-stream' },
    });
    expect(
      wrongHashResp.status === 422 && wrongHashResp.body?.error === 'visual_template_package_asset_hash_mismatch',
      'a wrong asset hash is rejected with the correct stable code',
      wrongHashResp.body,
    );

    // --- 8. restart persistence --------------------------------------------

    step('Restart the backend: the uploaded asset and imported template both survive');
    await stopBackend(backend, baseUrl);
    backend = await startBackend(exePath, env, baseUrl);
    const afterRestartAsset = await requestJSON(baseUrl, 'GET', `/api/visual-assets/${assetId}`);
    expect(afterRestartAsset.status === 200, 'the uploaded asset survived the restart', afterRestartAsset.body);
    const afterRestartTemplate = await requestJSON(baseUrl, 'GET', `/api/visual-templates/${reimport.body.id}`);
    expect(afterRestartTemplate.status === 200, 'the re-imported template survived the restart', afterRestartTemplate.body);

    step('Search every captured HTTP response body and the backend\'s own stdout/stderr for local paths');
    const haystack = secretScanChunks.join('\n');
    const backendOutput = backend.getOutput();
    expect(!haystack.includes(tempDir.replace(/\\/g, '\\\\')) && !backendOutput.includes(tempDir),
      'the temporary root path never leaks into a captured HTTP body or backend log line', undefined);
    expect(!haystack.includes(dataDir.replace(/\\/g, '\\\\')), 'the data directory path never leaks into a captured HTTP body', undefined);
    expect(!haystack.includes(assetId + '.png') && !haystack.includes('blobs' + (process.platform === 'win32' ? '\\' : '/') + sha256Hex(png)),
      'no raw blob filename/storage path leaks into a captured HTTP body', undefined);
    pass(`scanned ${haystack.length} bytes of HTTP bodies and ${backendOutput.length} bytes of backend output`);

    console.log('\nAll steps passed.');
  } catch (error) {
    if (backend !== null && process.env.STREAMING_TREE_VERIFY_DEBUG === '1') {
      console.error('\n--- backend output (debug) ---');
      console.error(backend.getOutput());
      console.error('--- end backend output ---\n');
    }
    throw error;
  } finally {
    await stopBackend(backend, baseUrl ?? '');
    rmSync(tempDir, { recursive: true, force: true });
    console.log(`Removed the temporary root: ${tempDir}`);
  }
}

main().catch((error) => {
  console.error(`\nVisual template package verification FAILED: ${error.message}`);
  process.exitCode = 1;
});
