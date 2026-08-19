#!/usr/bin/env node
// PRE-20D2B CI reliability milestone: a small, local, deterministic
// check that the real `on.push.paths` filters committed in
// .github/workflows/*.yml route representative changed-file sets to
// the expected workflows - see docs/ci-reliability.md sections 4-6 for
// the contract this script proves against the real, live YAML rather
// than a separately-maintained description of it. Not a meta-CI
// framework: it re-implements only the small, negation-free subset of
// GitHub's `paths` glob semantics this repository's own filters
// actually use (literal paths, and a trailing "/**" prefix match).

import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

const REPO_ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

const WORKFLOWS = {
  "cross-platform.yml": [
    "apps/server/**",
    "apps/web/**",
    ".github/workflows/cross-platform.yml",
  ],
  "macos-package.yml": [
    "apps/server/**",
    "apps/web/**",
    "scripts/build-release-macos.sh",
    "scripts/verify-macos-package.mjs",
    "LICENSE",
    "PRIVACY.md",
    "LEGAL.md",
    "THIRD_PARTY_NOTICES.md",
    ".github/workflows/macos-package.yml",
  ],
  "linux-package.yml": [
    "apps/server/**",
    "apps/web/**",
    "scripts/build-release-linux.sh",
    "scripts/verify-linux-package.mjs",
    "LICENSE",
    "PRIVACY.md",
    "LEGAL.md",
    "THIRD_PARTY_NOTICES.md",
    ".github/workflows/linux-package.yml",
  ],
  "linux-headless.yml": [
    "apps/server/**",
    "apps/web/**",
    "scripts/build-release-linux.sh",
    "scripts/provision-headless-master-key.sh",
    "scripts/provision-admin-password.sh",
    "scripts/systemd/streaming-tree.service",
    "scripts/verify-linux-headless.mjs",
    "scripts/verify-linux-remote-management.mjs",
    "LICENSE",
    "PRIVACY.md",
    "LEGAL.md",
    "THIRD_PARTY_NOTICES.md",
    ".github/workflows/linux-headless.yml",
  ],
};

// Extracts the literal `paths:` list actually committed under a
// workflow's `on.push` block, so this script fails loudly if the real
// YAML and the expectations above ever drift apart, instead of only
// checking the expectations against themselves.
function extractCommittedPushPaths(workflowFile) {
  const text = readFileSync(path.join(REPO_ROOT, ".github/workflows", workflowFile), "utf8");
  const pushBlockMatch = text.match(/\n {2}push:\n([\s\S]*?)(?:\n {2}\S|\n\S|$)/);
  if (!pushBlockMatch) throw new Error(`${workflowFile}: no top-level "push:" block found`);
  const pushBlock = pushBlockMatch[1];
  const pathsBlockMatch = pushBlock.match(/paths:\n([\s\S]*?)(?:\n {4}\S|\n {2}\S|$)/);
  if (!pathsBlockMatch) throw new Error(`${workflowFile}: "push:" has no "paths:" list`);
  return [...pathsBlockMatch[1].matchAll(/^\s*-\s*"([^"]+)"\s*$/gm)].map((m) => m[1]);
}

function matchesPattern(changedPath, pattern) {
  if (pattern.endsWith("/**")) {
    const prefix = pattern.slice(0, -3);
    return changedPath === prefix || changedPath.startsWith(prefix + "/");
  }
  return changedPath === pattern;
}

function workflowsTriggeredBy(changedPaths) {
  const triggered = [];
  for (const [workflow, patterns] of Object.entries(WORKFLOWS)) {
    const hit = changedPaths.some((changed) => patterns.some((pattern) => matchesPattern(changed, pattern)));
    if (hit) triggered.push(workflow);
  }
  return triggered.sort();
}

const CASES = [
  { label: "A: docs/progress.md only", changed: ["docs/progress.md"], expected: [] },
  { label: "B: README roadmap prose only", changed: ["README.md"], expected: [] },
  {
    label: "C: apps/server Go source",
    changed: ["apps/server/internal/secrets/headlessstore.go"],
    expected: ["cross-platform.yml", "linux-headless.yml", "linux-package.yml", "macos-package.yml"].sort(),
  },
  {
    label: "D: apps/web source",
    changed: ["apps/web/src/App.tsx"],
    expected: ["cross-platform.yml", "linux-headless.yml", "linux-package.yml", "macos-package.yml"].sort(),
  },
  {
    label: "E: scripts/systemd/streaming-tree.service",
    changed: ["scripts/systemd/streaming-tree.service"],
    expected: ["linux-headless.yml"],
  },
  {
    label: "F: scripts/build-release-macos.sh",
    changed: ["scripts/build-release-macos.sh"],
    expected: ["macos-package.yml"],
  },
  {
    label: "G: LEGAL.md",
    changed: ["LEGAL.md"],
    expected: ["linux-headless.yml", "linux-package.yml", "macos-package.yml"].sort(),
  },
  {
    label: "H: cross-platform.yml itself",
    changed: [".github/workflows/cross-platform.yml"],
    expected: ["cross-platform.yml"],
  },
];

let failures = 0;

console.log("Verifying committed workflow paths: filters match this script's own model...");
for (const workflow of Object.keys(WORKFLOWS)) {
  const committed = extractCommittedPushPaths(workflow).sort();
  const modeled = [...WORKFLOWS[workflow]].sort();
  const same = committed.length === modeled.length && committed.every((p, i) => p === modeled[i]);
  if (same) {
    console.log(`  OK   ${workflow}: committed paths match modeled paths (${committed.length} entries)`);
  } else {
    failures += 1;
    console.log(`  FAIL ${workflow}: committed paths differ from modeled paths`);
    console.log(`       committed: ${JSON.stringify(committed)}`);
    console.log(`       modeled:   ${JSON.stringify(modeled)}`);
  }
}

console.log("\nVerifying representative changed-path routing cases...");
for (const testCase of CASES) {
  const actual = workflowsTriggeredBy(testCase.changed);
  const expected = [...testCase.expected].sort();
  const same = actual.length === expected.length && actual.every((w, i) => w === expected[i]);
  if (same) {
    console.log(`  OK   ${testCase.label} -> [${actual.join(", ") || "none"}]`);
  } else {
    failures += 1;
    console.log(`  FAIL ${testCase.label}`);
    console.log(`       expected: [${expected.join(", ") || "none"}]`);
    console.log(`       actual:   [${actual.join(", ") || "none"}]`);
  }
}

if (failures > 0) {
  console.error(`\n${failures} routing check(s) failed.`);
  process.exit(1);
}
console.log("\nAll CI routing checks passed.");
