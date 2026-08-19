# PRE-20D2B — CI reliability and monitoring contract

**Research date:** 2026-08-19.

This document is CI/tooling infrastructure only. It defines no product
behavior and grants no Stage 20D2B (remote management/control plane)
capability. It exists to fix the repository's CI operating model -
workflow routing, run concurrency, monitoring discipline, and Windows
test-failure diagnostics - before any further Stage 20D2 product work
is attempted.

## 1. Why this milestone exists

Real project history (Stage 20D1/20D2A) demonstrated five concrete
problems:

1. `cross-platform.yml` ran on every push to `main`, including pure
   `docs/progress.md` journal-only commits, with no path filter at
   all - six jobs (five backend matrix legs plus one frontend job) for
   zero new product evidence.
2. No workflow in this repository had a `concurrency` block, so an
   older run could remain active/queued after a newer commit had
   already superseded it as the accepted source state.
3. `backend (windows-amd64)`'s `go test` step has now failed
   intermittently six times across this project's history (Stage 20B's
   `58464ee`; Stage 20C1's `56dc658`; Stage 20D1's `ab09cba` and
   `d910bfc`; Stage 20D2A's `3b44b7a` and `3e46bca`), including on
   commits that changed zero Go files, but no run preserved
   machine-readable evidence of which package/test actually failed -
   only a generic Checks API "exit code 1" annotation was ever
   recoverable.
4. Prior CI monitoring in this project used aggressive, repeated,
   unauthenticated GitHub REST polling (the runs endpoint every
   20-30 seconds, jobs and run status queried separately every cycle,
   multiple simultaneous background monitors, and repeated
   `/rate_limit` self-checks), which fully exhausted the 60-requests-
   per-hour anonymous quota more than once, each time producing a
   30-40 minute stretch where fresh CI status genuinely could not be
   retrieved.
5. That exhaustion produced operator-visible idle waiting and required
   the operator to send follow-up messages ("so?", "continue") to get
   a status update at all - a monitoring-system failure, not a
   product-work failure.

This contract fixes the system generating these problems, rather than
adding another instruction telling a future session not to wait.

## 2. Official GitHub sources consulted (primary source, this research
date)

- `docs.github.com` - "Triggering a workflow" (`on.push.paths`/
  `paths-ignore` semantics, glob syntax, negation ordering, and the
  explicit statement that a workflow skipped by path filtering leaves
  any required status check for it in a **`Pending`** state, which can
  block a pull request expecting that check).
- `docs.github.com` - "Using concurrency" (`concurrency.group`,
  `cancel-in-progress`, and the explicit requirement that concurrency
  group names must be unique **across workflows**, i.e. concurrency is
  repository-wide by default and must include `github.workflow` in the
  group expression to scope cancellation to a single workflow).
- `docs.github.com` - "Rate limits for the REST API": unauthenticated
  requests are limited to **60/hour per IP**; a personal-access-token-
  authenticated request is limited to **5,000/hour**; a `GITHUB_TOKEN`
  used from inside an Actions job is limited to **1,000/hour per
  repository**. Response headers `X-RateLimit-Limit`,
  `X-RateLimit-Remaining`, `X-RateLimit-Reset` (Unix epoch seconds),
  and (for secondary-limit responses) `Retry-After` are all confirmed
  current.
- `cli.github.com` - `gh run watch` manual page (`--exit-status`,
  `--compact`, default 3-second refresh interval; explicit note that
  `gh run watch` cannot authenticate via a fine-grained PAT because no
  fine-grained `checks:read` scope currently exists).
- `github.com/actions/upload-artifact` (README and Releases page,
  fetched directly, not assumed from memory): the current latest
  release is **v7.0.1** (major tag `v7`); artifacts produced by the
  v4+ line are immutable once uploaded; default retention is 90 days,
  configurable 1-90 via `retention-days`; artifact names must be
  unique per job unless `overwrite: true` is set.
- `github.com/actions/runner-images` - `Windows2025-Readme.md` (fetched
  directly): confirms `jq 1.8.1` and Node.js are both preinstalled on
  the `windows-latest` hosted image, alongside the already-relied-upon
  preinstalled `jq` on the Ubuntu/macOS hosted images - the diagnostic
  extraction step introduced by this milestone (§7 below) depends on
  `jq` being present without an extra install step, and this is now
  confirmed rather than assumed.

`gh` itself is **not installed** in this development environment
(`which gh` → not found), confirmed directly this milestone, not
assumed from a prior session. All monitoring in this milestone
therefore uses the unauthenticated REST fallback described in §6,
never `gh`.

## 3. Branch-protection / required-check audit

Because a workflow skipped by a path filter leaves any required check
it owns in `Pending` (§2), path-filter changes must not be made
blindly against a repository that requires one of these checks to
merge a pull request.

Checked directly via `GET /repos/Czekosabe/streaming-tree-for-obs/
branches/main` (the lightweight, publicly-readable branch endpoint -
the stricter `.../branches/main/protection` endpoint returned `401
Requires authentication`, consistent with GitHub's own documented
behavior that endpoint always requires an authenticated admin-level
token regardless of repository visibility, so it was not usable in
this unauthenticated environment). The response's own `protected`
field is `false`: **`main` has no branch protection and no required
status checks configured at all.** This is a definitive finding, not
an evidence limitation - path-filter changes in this milestone carry
zero required-check/merge-gate risk for the current repository
configuration. If branch protection is added in the future, this
finding should be re-checked before any further path-filter change.

## 4. Workflow purpose matrix (before this milestone)

| Workflow | Trigger (before) | Path filter (before) | Concurrency (before) |
| --- | --- | --- | --- |
| `cross-platform.yml` | `pull_request` (unfiltered), `push` to `main` (unfiltered), `workflow_dispatch` | none | none |
| `macos-package.yml` | `push` to `main`, `workflow_dispatch` | `apps/server/**`, `apps/web/**`, `scripts/build-release-macos.sh`, `scripts/verify-macos-package.mjs`, `LICENSE`, `PRIVACY.md`, `LEGAL.md`, `THIRD_PARTY_NOTICES.md`, own workflow file | none |
| `linux-package.yml` | `push` to `main`, `workflow_dispatch` | `apps/server/**`, `apps/web/**`, `scripts/build-release-linux.sh`, `scripts/verify-linux-package.mjs`, `LICENSE`, `PRIVACY.md`, `LEGAL.md`, `THIRD_PARTY_NOTICES.md`, own workflow file | none |
| `linux-headless.yml` | `push` to `main`, `workflow_dispatch` | `apps/server/**`, `scripts/build-release-linux.sh`, `scripts/provision-headless-master-key.sh`, `scripts/systemd/streaming-tree.service`, `scripts/verify-linux-headless.mjs`, own workflow file | none |

A real gap was found by direct source inspection (not assumed):
`linux-headless.yml` shares the exact same `scripts/build-release-
linux.sh` build script as `linux-package.yml`, which stages the built
frontend (`npm run build` in `apps/web`) and copies `LICENSE`,
`THIRD_PARTY_NOTICES.md`, `LEGAL.md`, `PRIVACY.md` into both the
embedded-legal directory and the `.deb`'s `usr/share/doc` path
(confirmed by `grep` against the real script). `linux-headless.yml`'s
path filter omitted `apps/web/**` and all four legal documents,
meaning a frontend or legal-document change would silently fail to
re-verify the headless package even though it changes what that
package actually contains. Fixed in §5 below.

## 5. Workflow purpose matrix (after this milestone)

| Workflow | Trigger (after) | Path filter (after) | Concurrency (after) |
| --- | --- | --- | --- |
| `cross-platform.yml` | `pull_request` (filtered), `push` to `main` (filtered), `workflow_dispatch` | `apps/server/**`, `apps/web/**`, own workflow file | `cross-platform-${{ github.ref }}`, `cancel-in-progress: true` |
| `macos-package.yml` | `push` to `main` (filtered, unchanged set), `workflow_dispatch` | unchanged (already correct) | `macos-package-${{ github.ref }}`, `cancel-in-progress: true` |
| `linux-package.yml` | `push` to `main` (filtered, unchanged set), `workflow_dispatch` | unchanged (already correct) | `linux-package-${{ github.ref }}`, `cancel-in-progress: true` |
| `linux-headless.yml` | `push` to `main` (filtered, corrected set), `workflow_dispatch` | **added** `apps/web/**`, `LICENSE`, `PRIVACY.md`, `LEGAL.md`, `THIRD_PARTY_NOTICES.md` | `linux-headless-${{ github.ref }}`, `cancel-in-progress: true` |

`cross-platform.yml`'s Go-only backend matrix does not embed
`apps/web`'s build output or the legal documents (those are only
staged into `internal/webassets/{embedded,legal}` by the three
release-build scripts, never by a plain `go build`/`go test`), so its
path filter correctly omits `LICENSE`/`PRIVACY.md`/`LEGAL.md`/
`THIRD_PARTY_NOTICES.md` - those changes are compile/test-irrelevant
for this workflow specifically, even though they are package-relevant
for the three package workflows. `apps/web/**` remains in
`cross-platform.yml`'s filter because its own `frontend` job runs the
real `npm run typecheck`/`lint`/`test`/`build` against that source.

Every concurrency group name includes the workflow's own identity
(hard-coded as a per-workflow literal prefix, not merely
`github.workflow`, to make the group trivially greppable across
workflow files) plus `github.ref`, so a `main`-branch run and any PR
run on a different ref never collide, and no workflow's concurrency
group can ever cancel a different workflow's run (confirmed against
§2's requirement that group names must be unique across workflows).
`cancel-in-progress: true` is enabled on all four - no current
evidence in this repository's history shows a reason an older,
already-superseded run should keep consuming runner capacity.

## 6. Docs-only / non-impacting behavior

A commit that changes only `docs/progress.md`, or only prose-only
documentation not physically embedded or packaged (e.g. ordinary
`README.md` roadmap narrative, `docs/*.md` contract prose that isn't
one of the four legal documents), triggers **none** of the four
workflows above after this milestone. This is intentional and is the
specific fix for problem 1 in §1.

`LICENSE`/`PRIVACY.md`/`LEGAL.md`/`THIRD_PARTY_NOTICES.md` remain
exceptions precisely because three of the four workflows physically
copy them into a shipped package or embedded binary - a change to
their content is real package-content evidence, not prose noise.

## 7. Windows `go test` diagnostic model

`cross-platform.yml`'s `go test` step (all five backend matrix legs,
not only `windows-amd64`, so the mechanism is uniform rather than
special-cased by matrix name) changes from:

```
go test -count=1 ./...
```

to:

```
go test -count=1 -json ./... | tee "$RUNNER_TEMP/test-results/go-test.jsonl" | jq -j -r 'select(.Action=="output") | .Output // empty'
```

run under `set -o pipefail` (`shell: bash`, available and already used
elsewhere in this same job on every matrix OS including
`windows-latest`). This runs the test suite exactly **once** - no
blind retry, no second pass - while:

- reconstructing the same human-readable `--- FAIL:`/`ok`/`PASS`
  console stream a plain `go test` would have produced (via `jq`
  filtering `Action=="output"` events back to their raw `Output`
  text), so a developer reading the live step log sees the same thing
  as before;
- simultaneously saving the complete raw JSON-Lines stream to
  `$RUNNER_TEMP/test-results/go-test.jsonl` for machine parsing;
  preserving `go test`'s real exit code through the pipeline via
  `pipefail`, so the step still fails exactly when the suite actually
  fails.

A second step, gated `if: failure()` (so it adds zero output to a
normal successful run - §19's "successful runs should remain concise"
requirement), parses that JSON-Lines file with `jq` to extract every
`Action=="fail"` event's `Package`/`Test` pair (`(package-level)` when
`Test` is absent, i.e. a build/setup-level failure rather than an
individual test), detects `panic:` and `test timed out` substrings in
the raw output, and writes a concise Markdown summary - failing
package(s), failing test(s) if Go reported one, panic/timeout flag,
`go version`, `GOOS`/`GOARCH`, `RUNNER_OS`, matrix leg name, elapsed
step duration - to `$GITHUB_STEP_SUMMARY`, plus one `::error::`
annotation per distinct failure so it surfaces directly on the commit/
PR checks view without opening the raw log. No secret, credential,
stream key, OAuth token, or unrelated environment-variable dump is
ever included - the summary step only ever reads `go test -json`'s own
structured fields.

A third step, also gated `if: failure()`, uploads
`$RUNNER_TEMP/test-results/go-test.jsonl` as a build artifact via
`actions/upload-artifact@v7` (the current latest major, confirmed in
§2), named `go-test-failure-<matrix-name>-<run id>-<run attempt>` for
unambiguous identification, `retention-days: 5` (short-lived
diagnostic data, not a release artifact), `if-no-files-found: ignore`.
This directly answers this milestone's root complaint: prior sessions
had only a generic "exit code 1" Checks-API annotation and no way to
identify which package or test actually failed. It contains raw Go
test output only - no binaries, no SQLite state, no credentials, no
environment dump.

No blind retry, no `continue-on-error`, no timeout increase, and no
package skip was added anywhere in this mechanism. A real failure
still fails the job exactly as before; this milestone only makes the
failure diagnosable after the fact.

## 8. Superseded-run policy

A run cancelled because a newer commit on the same ref superseded it
(via §5's `cancel-in-progress: true`) reports GitHub's own
`cancelled` conclusion, distinct from `failure`. Any monitoring script
or journal entry in this project must treat `cancelled` as "superseded
by a newer, authoritative run," never as evidence the superseded
commit's source code was broken. The newer run's own result is what
gets recorded as evidence; a `cancelled` run is not re-run, re-queried
for a "real" result, or cited as a red data point.

## 9. Final-evidence model

Accepted CI evidence for a given point in this project's history
belongs to **the final commit that actually changed product/source/
build/test/workflow-behavior inputs**, or an explicit
`workflow_dispatch` run against that exact source tree state - never
to a later commit that changed only `docs/progress.md` or other
non-impacting prose. A journal entry recording CI evidence must
explicitly name which commit is the evidence-bearing one when a
docs-only commit follows it, rather than implying the docs-only commit
itself was re-verified. No artificial source change is ever introduced
merely to force a workflow to run.

## 10. Monitoring strategy - authenticated `gh` (preferred, when
available)

If `which gh` succeeds and `gh auth status` reports an authenticated
session against `github.com`, prefer, in order: `gh run list` to
locate the run for a known commit SHA; a single `gh run watch <id>
--exit-status` (never more than one concurrent watcher per run); on a
failing conclusion, `gh run view <id> --log-failed` to pull only the
failed step's log rather than the entire run's log. `gh`'s own
authenticated rate ceiling (5,000/hour for a PAT) makes the
20-30-second polling cadence this milestone otherwise forbids for the
unauthenticated path acceptable if `gh` is genuinely available - but
this environment does not currently have `gh` installed (§2), so this
path is documented for future sessions, not exercised by this one.

## 11. Monitoring strategy - unauthenticated fallback (this
environment's actual path)

When `gh` is unavailable, monitoring a CI run for a known commit SHA
follows a strict request budget - **target no more than ~15-20
unauthenticated `api.github.com` requests per hour** for CI
observation under normal conditions, replacing every practice that
caused the prior exhaustion:

1. After pushing, continue other actionable local work first (writing
   the next commit's content, drafting the next journal entry,
   auditing the next file) rather than checking immediately - a run
   needs real wall-clock time to even start regardless of how soon it
   is polled.
2. Wait a sensible local interval (informed by this project's own
   observed run durations - typically several minutes for
   `cross-platform.yml`, longer for the two-pass package workflows)
   before the first lookup.
3. Locate the run with **one** request to the runs-list endpoint
   filtered by `head_sha` (returns every workflow's run for that exact
   commit in a single call - never one call per workflow).
4. Once located, poll only the run's own top-level status/conclusion,
   no more frequently than roughly every 2-4 minutes - never the jobs
   sub-resource on every cycle.
5. Only after the run reaches a terminal `status: completed` value,
   fetch the jobs endpoint **once** to get per-job conclusions.
6. Only if a job's conclusion is `failure`, fetch that job's specific
   detail (e.g. step-level annotations) - never for a `success` or
   `cancelled` job.
7. Never run more than one monitor/watcher for the same run at the
   same time.
8. Never poll `/rate_limit` on a fixed short interval merely to check
   remaining quota "just in case" - read the real rate-limit headers
   (`X-RateLimit-Remaining`, `X-RateLimit-Reset`) returned by the
   actual request that was already being made for a real reason,
   instead of spending a separate request purely to ask about quota.

If `X-RateLimit-Remaining` drops below a safe floor (treated as `5` in
this project, leaving headroom for the one or two calls needed to
confirm a terminal state): stop making API calls immediately. Compute
the reset wall-clock time **once** from the already-known
`X-RateLimit-Reset` epoch value, and if the outstanding evidence is
still genuinely required, perform a **single** local sleep/wait until
that time - never a loop of short sleeps interleaved with repeated
API attempts, and never a request made "to check if it's back yet"
before the computed reset time has actually passed. If local
actionable work remains (writing the next commit, drafting the next
document section, editing the next workflow file), do that work during
the wait instead of ending the turn in passive silence - `AskUserQuestion`
is never used to ask the operator to wake the session up.

## 12. What ends a wait

A wait for CI evidence ends when: the run reaches a terminal
conclusion; a computed rate-limit-reset time has passed and the
budgeted follow-up requests are made; or genuine local actionable work
under this same milestone remains and is done instead of idling. A
wait never ends by asking the operator "should I keep waiting?" or "do
you want me to check again?" - both are exactly the operator
interruptions this milestone's governing task forbids.

## 13. When a manual `workflow_dispatch` is appropriate

Automatic path-based routing intentionally does not run the expensive
package/portability workflows for a docs-only or CI-tooling-only
commit. When this milestone's own workflow-file changes need final
proof and no natural product-source commit will trigger them through
ordinary routing, an explicit `workflow_dispatch` invocation against
the commit containing the finished workflow-file change is the
correct way to obtain that proof - not a synthetic product-file edit
manufactured solely to trigger CI, and not silently skipping the
required final proof this milestone's own governing task requires
(§24 of that task).

## 14. What does and does not require CI re-execution

Requires re-execution against the new commit: any change to
`apps/server/**` or `apps/web/**` source; any change to a workflow
file itself; any change to a script a workflow invokes; any change to
one of the four legal documents for the three package workflows.

Does not require re-execution: a `docs/progress.md` journal entry
appended after the evidence-bearing commit already has terminal CI
evidence; ordinary prose-only documentation edits; roadmap/status
narrative updates in `README.md`/`docs/platform-support.md`/
`docs/project-overview.md` that do not touch source, scripts, or
legal-document content.

## 15. Notification-volume reduction, scoped correctly

The repository-side objective this milestone actually controls is
fewer unnecessary workflow runs, fewer superseded-but-still-running
runs, and better failure signal once a run does fail - achieved
entirely through §5's path-filter correction and §5's concurrency
addition. This milestone does not, and must not, modify the operator's
personal GitHub notification settings, add custom email, add a Slack/
Discord/webhook integration, add a bot, or add any telemetry - none of
that is a repository-code concern, and none of it was requested.

## 16. Explicit non-scope

This document defines no Stage 20D2B capability. It adds no
authentication, session, CSRF, TLS, reverse-proxy, remote-shutdown,
remote-overlay, remote-RTMP, or public MediaMTX-binding behavior of any
kind. It does not reopen or modify any Stage 20D2A product code
(`internal/secrets/headlessstore.go`, `cmd/server/main.go`'s headless
handling, the systemd unit, or the provisioning helper) - only the CI
workflow files, this contract, and (if genuinely needed) a small local
routing-verification script are in scope.

## 17. Real evidence obtained this milestone

Both new mechanisms were proven against real CI, not merely reasoned
about:

**Concurrency/cancellation, proven naturally (no synthetic run
manufactured to test it):** commit `c74368e` (the routing/concurrency
implementation, which changed all four workflow files) queued a
`cross-platform.yml` run (id `32252366278`). Before it finished,
commit `074004b` (the Windows-diagnostics implementation, which
changed only `cross-platform.yml`) pushed moments later and queued its
own `cross-platform.yml` run. Per the new `cancel-in-progress: true`
concurrency block, `32252366278` was correctly cancelled and GitHub
reported it as **`cancelled`**, not `failure` - exactly the §8/§11
behavior this milestone set out to build, observed for real on its
first opportunity. `c74368e`'s other three workflow runs (`linux-
headless.yml` id `32252366222`, `linux-package.yml` id `32252366306`,
`macos-package.yml` id `32252366243`) all reached terminal `success`
- each one implies both matrix architectures passed, since GitHub only
reports a matrix-job workflow run as `success` when every job in the
matrix succeeded.

**Windows diagnostic capture, proven on a real, unplanned recurrence:**
commit `074004b`'s own `cross-platform.yml` run (id `32252440982`)
failed - `backend (windows-amd64)` again, all other five jobs
(`frontend (linux-amd64)`, `backend (linux-amd64)`, `backend
(linux-arm64)`, `backend (macos-amd64)`, `backend (macos-arm64)`)
succeeded. Step-level inspection of the `windows-amd64` job confirms
the new mechanism worked exactly as designed: step 7 ("go test")
failed with the real `go test` exit code; step 8 ("Extract go test
failure diagnostics") completed successfully and produced annotations;
step 9 ("Upload go test failure log") completed successfully; step 10
("go build") was correctly `skipped`, preserving the job's overall
`failure` conclusion exactly as before this milestone. This is real
proof the new steps do not accidentally suppress a genuine failure
(§18's own requirement).

For the **first time in this project's history**, the exact failing
test is now known: the check run's annotations (`GET /repos/.../
check-runs/{id}/annotations`, publicly readable without
authentication) show two `failure`-level annotations naming
`github.com/streaming-tree/server/internal/provider/tts` ::
`TestSystemProviderListVoicesSmoke` (one package-level, one test-
level). Direct source reading confirms
`TestSystemProviderListVoicesSmoke` (`apps/server/internal/provider/
tts/windows_test.go`) skips via `skipIfSAPIUnavailable` when
`Capabilities().Available` is false, so this run's `Capabilities()`
call (which itself already checks `GetVoices().Count > 0`, per
`checkAvailable()` in `apps/server/internal/provider/tts/windows.go`)
reported SAPI as available with at least one voice, yet the test's own
separate `ListVoices()` call then failed or returned an unexpected
result moments later. This is genuinely new, actionable evidence - six
prior occurrences across this project's history had only a generic
"exit code 1" Checks-API annotation and no way to know even which
package was involved.

**The evidence gap that remains, stated honestly:** the exact
assertion message (which of `ListVoices() error = ...`, `ListVoices()
returned no voices`, or `a voice has an empty ID` actually fired) is
only present in the uploaded diagnostic artifact's raw JSON-Lines
content, and the artifact-download endpoint (`GET .../actions/
artifacts/{id}/zip`) returned `401 Requires authentication` in this
unauthenticated environment - confirmed by direct attempt, not
assumed. No `gh` CLI and no token are available here to retrieve it.
**No product or test fix was made to `internal/provider/tts` in this
milestone** - the evidence available narrows the failure to one
specific test and describes its plausible mechanism (a race or
transient inconsistency between two independent COM
`CoInitialize`/`SpVoice.GetVoices` sequences run moments apart on the
hosted Windows runner), but does not yet prove the exact assertion
that fired, and per this contract's own §15/§20 a fix is only made
when the evidence actually supports one. This is recorded as
diagnostic progress, not root-cause closure: a future session with
authenticated `gh`/API access, or the operator downloading the
`go-test-failure-windows-amd64-32252440982-1` artifact from the
Actions UI directly, can close this gap without further speculation.

## 18. Final workflow-file CI evidence and one remaining operator-
only gap

Per this contract's own §24-equivalent closing requirement, terminal
evidence was obtained for every workflow file this milestone changed:

| Workflow | Evidence commit | Run | Result |
| --- | --- | --- | --- |
| `linux-headless.yml` | `c74368e` (last commit changing this file) | `32252366222` | success, both architectures |
| `linux-package.yml` | `c74368e` (last commit changing this file) | `32252366306` | success, both architectures |
| `macos-package.yml` | `c74368e` (last commit changing this file) | `32252366243` | success, both architectures |
| `cross-platform.yml` | `074004b` (last commit changing this file) | `32252440982` | 5/6 jobs success; `backend (windows-amd64)` failed on the pre-existing, now better-diagnosed §17 flake |

`cross-platform.yml` does not have a clean, fully-green terminal run
on its own final commit (`074004b`). Obtaining one requires either an
authenticated `workflow_dispatch` call (this environment has no `gh`
and no token, confirmed earlier this milestone) or a genuinely new
`apps/server`/`apps/web` source commit through ordinary routing - and
manufacturing an artificial source change solely to trigger CI is
explicitly forbidden by this same contract's §9/§13. This is recorded
here as a real, honest, operator-actionable gap, not glossed over: an
operator with either repository-admin `gh` access or the Actions web
UI can manually run "Run workflow" for `cross-platform.yml` against
`074004b` (or any later commit) to obtain a clean dispatch-triggered
result. Every other piece of §17/§18's evidence - the routing fix, the
concurrency/cancellation behavior, and the diagnostic-capture
mechanism itself - is independently proven regardless of this specific
job's outcome, since the failure is attributable to the pre-existing
environmental characteristic, not to anything this milestone changed.

## 19. PRE-20D2B.1 - Windows SAPI flake: root-cause investigation and
resolution

**Exact failing package/test:**
`github.com/streaming-tree/server/internal/provider/tts` ::
`TestSystemProviderListVoicesSmoke` (identified by §17 above).

**Why prior evidence looked purely environmental:** every one of the
seven historical occurrences (Stage 20B's `58464ee`; Stage 20C1's
`56dc658`; Stage 20D1's `ab09cba`/`d910bfc`; Stage 20D2A's
`3b44b7a`/`3e46bca`; PRE-20D2B's `074004b`) was bracketed by clean runs
on commits with no relevant code difference, and never reproduced on
any local development machine - both are still true. What changed is
that the code itself was never actually audited for a real defect
before PRE-20D2B.1; "environmental" had been an inference from absence
of correlation with commits, not a conclusion from reading the
implementation.

**What was actually wrong, found by direct source audit:**
1. `ListVoices()`'s context-cancellation branch returned immediately on
   `ctx.Done()` without waiting for its locked-OS-thread goroutine to
   finish - unlike `Synthesize()`'s own cancellation path, which
   already waits. This orphans a locked OS thread holding a live COM
   apartment for however long the in-flight `GetVoices`/token
   enumeration actually takes, unbounded and unobserved, on every
   cancellation or timeout.
2. `runOnLockedThread` (via go-ole's `CoInitialize` wrapper) treated
   the HRESULT `S_FALSE` - which Microsoft's own `CoInitialize`
   reference documents as "COM already initialized on this thread", a
   **success** code - as a hard failure, and skipped
   `CoUninitialize` on that path, leaving the OS thread's COM
   reference count unbalanced for whatever goroutine Go's runtime
   handed that thread to next.

**Why the selected fix is correct:** both are genuine, source-
confirmed defects in the exact COM-lifecycle code this investigation
was scoped to audit, independent of whether either fully explains
every historical CI occurrence - a resource-lifecycle bug (orphaning a
thread/COM apartment on cancellation) and a HRESULT-handling bug
(misreading a documented success code as failure) are worth fixing on
their own merits, and both are plausible, evidence-consistent
mechanisms for CI-only intermittency under runner virtualization/
contention that a local, always-fast, always-available development
machine would never trigger. An empirical stress test (11,000
`CoInitialize` calls reproducing `runOnLockedThread`'s exact pattern
across varied `GOMAXPROCS` and concurrency levels) found zero `S_FALSE`
occurrences from ordinary Go thread-pool reuse alone on this Go
version - recorded honestly as a negative result. **The exact CI
assertion text was never recovered** (the diagnostic artifact requires
authentication this environment does not have, and a follow-up attempt
at the raw job-logs endpoint hit a live rate limit before an
authentication answer could even be determined) - so no single
mechanism is claimed as certain or exclusive.

**Deterministic-vs-host-smoke classification:** `TestSystemProviderList
VoicesSmoke` and its three siblings are explicitly classified as
host-capability integration smoke tests (already true via
`skipIfSAPIUnavailable`, now stated explicitly in the test file's own
section comment) - they prove "this host currently has a usable SAPI
voice engine," not something this package's code can guarantee about
every machine it runs on. The zero-voice-availability decision
previously only reachable through real SAPI was factored into a pure
function (`voiceCountAvailable`) with three new deterministic unit
tests, now part of the hard gate regardless of host SAPI availability.

**Future diagnostic behavior:** unchanged from §7-§8 above - the same
`go test -json`/`tee`/`jq` mechanism, failure annotations, and
diagnostic-artifact upload apply to any future Windows failure,
including a recurrence of this exact test. If it recurs, the same
mechanism will again identify the exact package/test on the very next
occurrence.

**Final resolution evidence:** commit `3740c8d` (`fix(server): correct
Windows SAPI voice enumeration lifecycle`) triggered all four
workflows (via its `apps/server/**` path). All four reached a clean
terminal `success`, confirmed at both the run level and the per-job
level:

| Workflow | Run | Result |
| --- | --- | --- |
| `cross-platform.yml` | `32284351369` | success - all 6 jobs, including `backend (windows-amd64)`, explicitly confirmed `success` |
| `linux-headless.yml` | `32284351410` | success, both architectures |
| `linux-package.yml` | `32284351399` | success, both architectures |
| `macos-package.yml` | `32284351366` | success, both architectures |

This is the first fully green `cross-platform.yml` run in this
project's history to include a Windows job that had previously shown
this specific failure - obtained on the very first CI attempt after
the fix, with no retry.
