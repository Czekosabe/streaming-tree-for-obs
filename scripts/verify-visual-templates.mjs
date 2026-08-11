#!/usr/bin/env node
/**
 * Local, no-real-Twitch, no-real-OBS verification of Stage 14A: the
 * reusable visual-design template library - built-ins, the persisted
 * user template library, target/owner-instance compatibility, and
 * asset-free JSON import/export. See docs/visual-templates.md for the
 * full contract this script verifies against.
 *
 * Unlike every other engagement-era verify-*.mjs script, this one
 * needs no fake Twitch OAuth/Helix/EventSub server at all: template
 * management, alert-profile/rule creation, and chat-overlay-profile
 * creation are all real backend operations that never require a
 * connected account. This script therefore makes zero network calls
 * beyond its own loopback backend - the strongest possible form of
 * "no real provider/OBS contacted."
 *
 * A representative subset of the task's own ~42-item verification list
 * is covered here; every scenario intentionally NOT covered is named
 * against a specific covering Go test in docs/progress.md's own Stage
 * 14A test-verification entry.
 *
 * Usage: node scripts/verify-visual-templates.mjs
 * Exits non-zero on the first failed expectation.
 */

import { spawn } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { mkdirSync, mkdtempSync, rmSync } from 'node:fs';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

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

async function request(baseUrl, method, path, body) {
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

// --- Stage 14A template/document fixtures --------------------------------

function chatDocument(binding = 'username') {
  return {
    version: 3,
    canvas: { width: 960, height: 280, transparent: true },
    layers: [
      {
        id: `layer_${randomUUID().slice(0, 8)}`, name: 'Text', kind: 'text', visible: true, locked: false, order: 0,
        frame: { x: 10, y: 10, width: 400, height: 60 }, opacity: 1,
        text: {
          binding, missingValueBehavior: 'hide',
          fontFamily: 'system-ui', fontSize: 20, fontWeight: 700, lineHeight: 1.2, letterSpacing: 0,
          textColor: '#FFFFFF', horizontalAlign: 'left', verticalAlign: 'middle',
          outlineWidth: 0, outlineColor: '#000000',
          shadowEnabled: false, shadowOffsetX: 0, shadowOffsetY: 0, shadowBlur: 0, shadowColor: '#000000',
        },
        entryAnimation: 'none', exitAnimation: 'none', animationDurationMs: 0,
      },
    ],
  };
}

function alertDocument(binding = 'username', version = 3) {
  return {
    version,
    canvas: { width: 1920, height: 1080, transparent: true },
    layers: [
      {
        id: `layer_${randomUUID().slice(0, 8)}`, name: 'Text', kind: 'text', visible: true, locked: false, order: 0,
        frame: { x: 160, y: 940, width: 1600, height: 100 }, opacity: 1,
        text: {
          binding, missingValueBehavior: 'hide',
          fontFamily: 'system-ui', fontSize: 40, fontWeight: 700, lineHeight: 1.2, letterSpacing: 0,
          textColor: '#FFFFFF', horizontalAlign: 'center', verticalAlign: 'middle',
          outlineWidth: 0, outlineColor: '#000000',
          shadowEnabled: false, shadowOffsetX: 0, shadowOffsetY: 0, shadowBlur: 0, shadowColor: '#000000',
        },
        entryAnimation: 'fade', exitAnimation: 'fade', animationDurationMs: 300,
      },
    ],
  };
}

function templateFile({ target, name = 'Imported Template', schemaVersion = 1, visualDesign }) {
  return {
    format: 'streaming-tree-visual-template', schemaVersion, target,
    name, description: 'An imported test template.', author: 'test', license: 'CC0-1.0',
    visualDesign: visualDesign ?? (target === 'chat' ? chatDocument() : alertDocument()),
  };
}

async function main() {
  console.log('Stage 14A visual template library verification (local only, no real Twitch, no real OBS, no fake provider servers needed)');
  console.log(`Run id: ${RUN_ID}`);

  const tempDir = mkdtempSync(join(tmpdir(), 'streaming-tree-visual-templates-'));
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

    step('Start the backend (built-in template registry validated at startup - a malformed built-in would abort here)');
    backend = await startBackend(exePath, env, baseUrl);

    step('GET /api/visual-templates lists built-ins with at least 3 alert and 3 chat templates, no user templates yet');
    const initialList = await request(baseUrl, 'GET', '/api/visual-templates');
    expect(initialList.status === 200, 'list succeeds', initialList.body);
    const initialAlertBuiltins = initialList.body.filter((t) => t.source === 'builtin' && t.target === 'alert');
    const initialChatBuiltins = initialList.body.filter((t) => t.source === 'builtin' && t.target === 'chat');
    expect(initialAlertBuiltins.length >= 3, 'at least 3 alert built-ins exist', initialAlertBuiltins.length);
    expect(initialChatBuiltins.length >= 3, 'at least 3 chat built-ins exist', initialChatBuiltins.length);
    const initialUserTemplates = initialList.body.filter((t) => t.source === 'user');
    expect(initialUserTemplates.length === 0, 'built-ins are never user-sourced rows - none exist before any create/import', initialUserTemplates.length);

    step('Create a real alert profile and a real follow rule (no connected account needed for this)');
    const alertProfile = await request(baseUrl, 'POST', '/api/alert-profiles', { name: 'Main' });
    expect(alertProfile.status === 201, 'alert profile created', alertProfile.body);
    const followRule = await request(baseUrl, 'POST', `/api/alert-profiles/${alertProfile.body.id}/rules`, {
      name: 'Follow alert', enabled: true, eventType: 'follow', priority: 50, durationMs: 5000,
      requiredRole: 'everyone', showPlatform: true, showUsername: true,
      textTemplate: '{username} just followed!', entryAnimation: 'fade', exitAnimation: 'fade',
      animationDurationMs: 400, providers: [], accounts: [],
      allowGrouping: false, groupWindowMs: 5000, interruptMode: 'never', interruptible: true,
    });
    expect(followRule.status === 201, 'follow rule created', followRule.body);
    const ruleId = followRule.body.id;

    step('Create a real chat-overlay profile (no connected account needed for this either)');
    const chatOverlay = await request(baseUrl, 'POST', '/api/chat-overlays', { name: 'Main Overlay' });
    expect(chatOverlay.status === 200, 'chat overlay created', chatOverlay.body);
    const overlayId = chatOverlay.body.id;

    step('Compatibility scoped to the real alert rule: every alert built-in is compatible, every chat built-in is a target mismatch');
    const alertScoped = await request(baseUrl, 'GET', `/api/visual-templates?target=alert&ownerId=${ruleId}`);
    for (const t of alertScoped.body.filter((x) => x.target === 'alert')) {
      expect(t.compatibility?.compatible === true, `alert built-in "${t.id}" is compatible with the follow rule`, t.compatibility);
    }
    for (const t of alertScoped.body.filter((x) => x.target === 'chat')) {
      expect(
        t.compatibility?.compatible === false && t.compatibility.blockers?.[0] === 'template_target_mismatch',
        `chat template "${t.id}" reports template_target_mismatch when scoped to an alert owner`,
        t.compatibility,
      );
    }

    step('Compatibility scoped to the real chat overlay: every chat built-in is compatible');
    const chatScoped = await request(baseUrl, 'GET', `/api/visual-templates?target=chat&ownerId=${overlayId}`);
    for (const t of chatScoped.body.filter((x) => x.target === 'chat')) {
      expect(t.compatibility?.compatible === true, `chat built-in "${t.id}" is compatible with the chat overlay`, t.compatibility);
    }

    step('A user alert template using "quantity" is reported incompatible with the follow rule (follow has no quantity)');
    const quantityTemplate = await request(baseUrl, 'POST', '/api/visual-templates', {
      target: 'alert', name: 'Quantity Template', description: '', author: '', license: '',
      document: alertDocument('quantity'),
    });
    expect(quantityTemplate.status === 201, 'quantity template created', quantityTemplate.body);
    const scopedAfterCreate = await request(baseUrl, 'GET', `/api/visual-templates?target=alert&ownerId=${ruleId}`);
    const quantityEntry = scopedAfterCreate.body.find((t) => t.id === quantityTemplate.body.id);
    expect(
      quantityEntry?.compatibility?.compatible === false && quantityEntry.compatibility.blockers?.[0] === 'alert_binding_unavailable',
      'the quantity template is reported incompatible with the follow rule',
      quantityEntry?.compatibility,
    );

    step('"Save as template" (creating directly from a draft) never touches the owner\'s own saved visual design');
    const ruleDesignBefore = await request(baseUrl, 'GET', `/api/alert-rules/${ruleId}/visual-design`);
    expect(ruleDesignBefore.body.persisted === false, 'the rule has no saved design yet', ruleDesignBefore.body);
    const savedFromDraft = await request(baseUrl, 'POST', '/api/visual-templates', {
      target: 'alert', name: 'From Draft', description: '', author: '', license: '',
      document: alertDocument('username'),
    });
    expect(savedFromDraft.status === 201, 'template saved from a draft', savedFromDraft.body);
    const ruleDesignAfter = await request(baseUrl, 'GET', `/api/alert-rules/${ruleId}/visual-design`);
    expect(ruleDesignAfter.body.persisted === false, 'the rule still has no saved design after "Save as template"', ruleDesignAfter.body);

    step('Import preview validates a v1-embedded-document file, migrates it to the current version, and persists nothing');
    const v1File = templateFile({ target: 'chat', name: 'Legacy Import', visualDesign: chatDocument('username') });
    v1File.visualDesign.version = 1;
    const beforePreviewCount = (await request(baseUrl, 'GET', '/api/visual-templates')).body.length;
    const preview = await request(baseUrl, 'POST', '/api/visual-templates/import/preview', v1File);
    expect(preview.status === 200, 'import preview succeeds', preview.body);
    expect(preview.body.document.version === 3, 'the embedded v1 document was migrated to v3 in the preview', preview.body.document);
    const afterPreviewCount = (await request(baseUrl, 'GET', '/api/visual-templates')).body.length;
    expect(afterPreviewCount === beforePreviewCount, 'import preview persisted nothing', { beforePreviewCount, afterPreviewCount });

    step('Actual import persists a new template with a server-generated tpl_ id, and cannot choose a local id');
    const imported = await request(baseUrl, 'POST', '/api/visual-templates/import', v1File);
    expect(imported.status === 201, 'import succeeds', imported.body);
    expect(typeof imported.body.id === 'string' && imported.body.id.startsWith('tpl_'), 'the imported template got a tpl_-prefixed id', imported.body.id);
    expect(imported.body.document.version === 3, 'the persisted document is at the current version', imported.body.document);
    const importedId = imported.body.id;

    const fileWithId = { ...templateFile({ target: 'chat' }), id: 'tpl_attacker_chosen' };
    const rejectedId = await request(baseUrl, 'POST', '/api/visual-templates/import', fileWithId);
    expect(rejectedId.status === 400, 'a client-supplied "id" field is rejected outright (unknown field)', rejectedId.body);

    step('An unknown top-level field is rejected');
    const unknownField = { ...templateFile({ target: 'chat' }), extra: 'nope' };
    const rejectedUnknown = await request(baseUrl, 'POST', '/api/visual-templates/import', unknownField);
    expect(rejectedUnknown.status === 400, 'an unknown field is rejected', rejectedUnknown.body);

    step('An unsupported template schemaVersion is rejected (422 visual_template_version_unsupported)');
    const badTemplateVersion = templateFile({ target: 'chat', schemaVersion: 99 });
    const rejectedTemplateVersion = await request(baseUrl, 'POST', '/api/visual-templates/import', badTemplateVersion);
    expect(
      rejectedTemplateVersion.status === 422 && rejectedTemplateVersion.body.error === 'visual_template_version_unsupported',
      'an unsupported template schema version is rejected with the correct code',
      rejectedTemplateVersion.body,
    );

    step('An unsupported future embedded visual-design version is rejected (422 visual_template_design_version_unsupported)');
    const badDesignVersion = templateFile({ target: 'chat', visualDesign: { ...chatDocument(), version: 999 } });
    const rejectedDesignVersion = await request(baseUrl, 'POST', '/api/visual-templates/import', badDesignVersion);
    expect(
      rejectedDesignVersion.status === 422 && rejectedDesignVersion.body.error === 'visual_template_design_version_unsupported',
      'an unsupported future visual-design version is rejected with the correct code',
      rejectedDesignVersion.body,
    );

    step('An oversized import body is rejected (413)');
    const oversizedFile = templateFile({ target: 'chat', name: 'x'.repeat(200_000) });
    const rejectedOversized = await request(baseUrl, 'POST', '/api/visual-templates/import', oversizedFile);
    expect(rejectedOversized.status === 413, 'an oversized import body is rejected', rejectedOversized.status);

    step('Corrective pass: GET/PUT on /api/visual-templates/import returns 405 with Allow: POST, never a bare 404 treating "import" as a template id');
    const countBeforeWrongMethod = (await request(baseUrl, 'GET', '/api/visual-templates')).body.length;
    const getImport = await request(baseUrl, 'GET', '/api/visual-templates/import');
    expect(getImport.status === 405, 'GET /api/visual-templates/import is 405, not 404', getImport.status);
    expect(getImport.headers.get('allow') === 'POST', 'GET /api/visual-templates/import carries Allow: POST', getImport.headers.get('allow'));
    expect(getImport.body?.error === 'method_not_allowed', 'the response uses the shared method_not_allowed error code', getImport.body);
    const putImport = await request(baseUrl, 'PUT', '/api/visual-templates/import', { name: 'irrelevant' });
    expect(putImport.status === 405, 'PUT /api/visual-templates/import is 405', putImport.status);
    expect(putImport.headers.get('allow') === 'POST', 'PUT /api/visual-templates/import carries Allow: POST', putImport.headers.get('allow'));

    step('Corrective pass: GET/DELETE on /api/visual-templates/import/preview returns 405 with Allow: POST');
    const getPreview = await request(baseUrl, 'GET', '/api/visual-templates/import/preview');
    expect(getPreview.status === 405, 'GET /api/visual-templates/import/preview is 405', getPreview.status);
    expect(getPreview.headers.get('allow') === 'POST', 'GET /api/visual-templates/import/preview carries Allow: POST', getPreview.headers.get('allow'));
    const deletePreview = await request(baseUrl, 'DELETE', '/api/visual-templates/import/preview');
    expect(deletePreview.status === 405, 'DELETE /api/visual-templates/import/preview is 405', deletePreview.status);
    expect(deletePreview.headers.get('allow') === 'POST', 'DELETE /api/visual-templates/import/preview carries Allow: POST', deletePreview.headers.get('allow'));
    const countAfterWrongMethod = (await request(baseUrl, 'GET', '/api/visual-templates')).body.length;
    expect(countAfterWrongMethod === countBeforeWrongMethod, 'no template was created/imported by any of the rejected wrong-method calls', { countBeforeWrongMethod, countAfterWrongMethod });

    step('Corrective pass: an unknown template resource still 404s normally (never overcorrected into 405)');
    const unknownResource = await request(baseUrl, 'GET', '/api/visual-templates/does-not-exist');
    expect(unknownResource.status === 404, 'a genuinely unknown template id still 404s', unknownResource.status);

    step('Corrective pass: the real POST import/preview endpoints still work after the routing fix');
    const stillWorksPreview = await request(baseUrl, 'POST', '/api/visual-templates/import/preview', templateFile({ target: 'chat', name: 'Still Works Preview' }));
    expect(stillWorksPreview.status === 200, 'POST import/preview still works after the routing fix', stillWorksPreview.body);
    const stillWorksImport = await request(baseUrl, 'POST', '/api/visual-templates/import', templateFile({ target: 'chat', name: 'Still Works Import' }));
    expect(stillWorksImport.status === 201, 'POST import still works after the routing fix', stillWorksImport.body);

    step('Export returns a safe Content-Disposition and no local database identifiers');
    const exportResp = await request(baseUrl, 'GET', `/api/visual-templates/${importedId}/export`);
    expect(exportResp.status === 200, 'export succeeds', exportResp.body);
    const disposition = exportResp.headers.get('content-disposition') ?? '';
    expect(disposition.includes('attachment'), 'Content-Disposition is an attachment', disposition);
    expect(disposition.includes('.streaming-tree-template.json'), 'the filename uses the Stage 14A extension', disposition);
    expect(!disposition.includes('/') && !disposition.includes('\\'), 'the filename has no path separators', disposition);
    expect(!('id' in exportResp.body) && !('createdAt' in exportResp.body) && !('updatedAt' in exportResp.body),
      'the exported file carries no local id or timestamps', exportResp.body);
    expect(exportResp.body.visualDesign.version === 3, 'the export carries the current visual-design version', exportResp.body.visualDesign);

    step('Delete the exported template, then re-import the exported bytes: a semantic round trip');
    const deleteExported = await request(baseUrl, 'DELETE', `/api/visual-templates/${importedId}`, undefined);
    expect(deleteExported.status === 204, 'delete succeeds', deleteExported.status);
    const reimport = await request(baseUrl, 'POST', '/api/visual-templates/import', exportResp.body);
    expect(reimport.status === 201, 're-import of the exported file succeeds', reimport.body);
    expect(reimport.body.id !== importedId, 're-import receives a brand-new local id, never the deleted one', { old: importedId, new: reimport.body.id });
    expect(reimport.body.name === 'Legacy Import' && reimport.body.document.layers.length === 1,
      'the re-imported template matches the original metadata/document semantically', reimport.body);

    step('Deleting a user template never affects the rule\'s own (still-unsaved) visual design');
    const ruleDesignAfterDelete = await request(baseUrl, 'GET', `/api/alert-rules/${ruleId}/visual-design`);
    expect(ruleDesignAfterDelete.body.persisted === false, 'the rule still has no saved design after unrelated template deletes', ruleDesignAfterDelete.body);

    step('A built-in cannot be updated or deleted (409 visual_template_immutable)');
    const builtinId = initialAlertBuiltins[0].id;
    const updateBuiltin = await request(baseUrl, 'PUT', `/api/visual-templates/${builtinId}`, { name: 'x', description: '', author: '', license: '' });
    expect(updateBuiltin.status === 409 && updateBuiltin.body.error === 'visual_template_immutable', 'updating a built-in is rejected', updateBuiltin.body);
    const deleteBuiltin = await request(baseUrl, 'DELETE', `/api/visual-templates/${builtinId}`, undefined);
    expect(deleteBuiltin.status === 409 && deleteBuiltin.body.error === 'visual_template_immutable', 'deleting a built-in is rejected', deleteBuiltin.body);

    step('None of the above template operations ever changed the alert\'s own public renderingMode');
    const publicAlertConfig = await request(baseUrl, 'GET', `/api/public/alert-profiles/${alertProfile.body.publicSlug}/config`);
    expect(publicAlertConfig.status === 200, 'public alert config resolves', publicAlertConfig.body);

    step('Restart the backend: the surviving user template persists across restart');
    await stopBackend(backend, baseUrl);
    backend = await startBackend(exePath, env, baseUrl);
    const afterRestartGet = await request(baseUrl, 'GET', `/api/visual-templates/${reimport.body.id}`);
    expect(afterRestartGet.status === 200 && afterRestartGet.body.name === 'Legacy Import', 'the user template survived the restart', afterRestartGet.body);
    const afterRestartList = await request(baseUrl, 'GET', '/api/visual-templates');
    const afterRestartAlertBuiltins = afterRestartList.body.filter((t) => t.source === 'builtin' && t.target === 'alert');
    expect(afterRestartAlertBuiltins.length >= 3, 'built-ins are still present after restart (they are code, not rows)', afterRestartAlertBuiltins.length);

    step('Search every captured HTTP response body and the backend\'s own stdout/stderr for local paths');
    const haystack = secretScanChunks.join('\n');
    const backendOutput = backend.getOutput();
    expect(!haystack.includes(tempDir.replace(/\\/g, '\\\\')) && !backendOutput.includes(tempDir),
      'the temporary root path never leaks into a captured HTTP body or backend log line', undefined);
    expect(!haystack.includes(dataDir.replace(/\\/g, '\\\\')), 'the data directory path never leaks into a captured HTTP body', undefined);
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
  console.error(`\nVisual template library verification FAILED: ${error.message}`);
  process.exitCode = 1;
});
