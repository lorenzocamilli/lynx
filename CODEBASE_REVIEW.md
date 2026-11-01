# First-release code quality review

## Scope and architecture

This is a codebase-wide architecture and release-readiness pass with targeted
source review, automated checks, and regression fixes. It is not an exhaustive
security audit or a claim that every execution path has been tested. Generated
GraphQL implementations were treated as generated artifacts, not refactoring
targets.

| Area | Responsibility |
| --- | --- |
| `cmd/lynx` | Startup, service wiring, shared admin/proxy listener, auth/CSRF, configuration endpoints, shutdown, embedded UI |
| `pkg/proxy` | Reverse proxy, CONNECT/TLS MITM, certificates, modifier chains, gzip decoding |
| `pkg/proxy/intercept` | Pending request/response queues, editing/cancellation, interception filters |
| `pkg/reqlog` | Traffic capture, storage limits/redaction, search, live-update events |
| `pkg/sender` | Persisted request editing/replay, protocol-aware outbound transports, history/search |
| `pkg/proj`, `pkg/scope` | Active-project settings and scope matching |
| `pkg/db/bolt` | Project-scoped BoltDB persistence using gob encoding |
| `pkg/filter` | Search lexer/parser, expression AST and header matching |
| `pkg/api` | GraphQL schema, resolvers and HTTP transport |
| `pkg/config`, `pkg/log`, `pkg/sse` | YAML configuration, logging, event fan-out |
| `admin/src/features` | Next.js pages/features for projects, logs, interception, sender, scope and settings |
| `admin/src/lib` | Shared UI, auth/Apollo/SSE clients and request-editing helpers |

The package boundaries are generally reasonable. A wholesale rewrite or a new
service abstraction is not justified. The main weaknesses are ownership of
mutable project state and HTTP bodies, duplicated search/editing logic, and
insufficient tests at the API/UI boundaries.

## Fixed in this pass

- **Logging corrupted forwarded traffic at the storage limit.** Request and
  response bodies were replaced with only their logged prefix. Logging now
  replays the prefix followed by the unread remainder, preserving the original
  body's `Close`. Unlogged requests without an active project are not buffered.
- **CONNECT handlers leaked after tunnel closure.** `ConnNotify.Close` did not
  implement `net.Conn.Close() error`, and the listener received the embedded
  connection rather than the notifier. Notification now uses a once-closed
  channel, the actual wrapper is accepted, and listener addresses remain valid
  after accepting. The real CONNECT test now checks handler completion.
- **Gzip decoding buffered the entire expanded response and lost the upstream
  closer.** Decoding now streams, closes the upstream body, and removes the
  obsolete compressed content length. Interception/body filters can still
  buffer bodies separately; see below.
- **Response logs could follow the wrong active project.** Requests now carry
  their capture project in context; asynchronous response storage uses that ID,
  not a later global active project. Request-log settings access is synchronized,
  and response metadata is snapshotted before background persistence.
- **Clearing sender history deleted a nonexistent nested bucket.** The operation
  now clears/recreates the sender bucket under the project and remains usable
  and repeatable afterward.
- **Sender scope was missing in startup wiring.** Scope-filtered sender queries
  now receive the shared scope service rather than a nil pointer.
- **Intercepted request protocol used the wrong GraphQL representation.** The
  resolver now returns mapped enums (`HTTP11`, etc.), not wire strings
  (`HTTP/1.1`). All three supported protocols have regression coverage.
- **Invalid/abandoned filter parsing could leave a lexer goroutine blocked on
  token delivery.** Scanning now happens on demand without a producer goroutine;
  repeated reads after completion consistently return EOF. Parser EOF checks now
  use the token constant rather than the rune sentinel.
- Removed a duplicate sender response assignment, redundant response encoding in
  BoltDB, unused React imports/a callback parameter, and unused `allotment` and
  `deepmerge` dependencies with their unneeded transitive packages.
- Enabled TypeScript unused-local/parameter checks. Fixed `make check` to enable
  CGO specifically for race detection, despite the default static-build setting.

No public Go function was deleted simply because it has no in-repository caller.
Staticcheck found no remaining findings; a static import graph found all admin
source modules reachable from Next.js pages. Neither check proves all exports
are necessary for external consumers.

## Remaining priorities before release

### 1. Shared-state and body ownership — high priority

Issue #22 is implemented locally: project settings read/modify/persist/apply
operations and project Open/Close/Delete share a coordinator lock. Sender and
request-log operations take this gate only while capturing their ID, filter and
owned scope snapshot, then release all state locks before repository I/O,
interception waits or network work. A switch cannot expose partially applied
settings to those snapshots. Standalone service state remains protected by its
own mutex; filter project IDs follow the service's active project.

Operation semantics: a request belongs to the project captured when the operation
starts, even if another project opens while it is paused. Sender cloning reads
its source log with an explicit captured project ID. Responses and clones are
stored in that original project, never redirected to the newly active one. If
the original project is deleted while work is in flight, persistence may fail;
it must not fall back to the new project. Configuration of the shared gate is a
setup-only step performed by `proj.NewService`; do not rebind live services.

Scope getters/setters now copy their slices and in-flight operations retain an
independent scope. Compiled regexes and search-expression ASTs are treated as
immutable after configuration.

Issue #23 is implemented locally: interception settings and both pending queues
share a mutex. Admissions prepare/filter bodies without holding it, then validate
a settings version under the lock; a disable during preparation cannot enqueue
stale work afterward. Queue clearing closes abort channels without waiting for
receivers. Mutation resolution and removal are serialized under the same lock,
with a single buffered handoff so only one competing action can win.

Pending queues retain private snapshots, not the live forwarding objects.
`Items`/`ItemByID` return independent headers/URLs and body readers, including the
request attached to a response. Mutations hand off independent copies; caller
access during a mutation must be exclusive. Filter helpers consume caller-owned
views and close a consumed original body when replacing it with replayable bytes.
Capture failures cancel request forwarding without losing request context values.

When response interception is enabled before sending, request bodies are made
replayable for later response inspection. If interception is enabled only after
a request was sent and no `GetBody` is available, its consumed request body is
unavailable: response inspection returns metadata, never a live/closed stream.
Contexts, TLS/certificate metadata, compiled regexes and filter ASTs remain
read-only by contract. The service does not make concurrent mutation of a
caller's same HTTP object safe.

**Resource policy remains #32:** pending bodies are buffered in memory before
publication. There are still no inspection/replay body-size or queue-count caps;
this ownership fix does not claim bounded memory or solve slow body reads. No
state lock is held during body I/O or while waiting for user action.

### 2. Dependency remediation — release decision required

Actual scan results from this pass:

- `govulncheck` reports **GO-2024-2920** in `gqlparser/v2@v2.2.0` (fixed in
  `v2.5.14`) and **GO-2026-6278** in `gorilla/websocket@v1.4.2` (fixed in
  `v1.5.3`) as symbol-level findings. Reachability is the scanner's result, not
  proof of practical exploitation in Lynx. GraphQL is behind admin auth.
- After removing unused dependencies, online `npm audit --omit=dev` reports
  **4 affected packages**: `next` (critical), `sharp` (high), `source-map-js`
  (high), and `baseline-browser-mapping` (moderate). Several advisories concern
  Next.js server functionality absent from the shipped Go/static-export runtime;
  development/Next-server deployment exposure must be assessed separately.

Upgrade compatible dependencies and test GraphQL regeneration and actual UI
operations. Do not apply a blanket force-update or call every npm severity a
confirmed vulnerability in the embedded binary. Offline npm installation's
"0 vulnerabilities" output is **not** an online security scan.

### 3. Admin boundary and token handling — high priority review

`cmd/lynx/lynx.go`'s admin matcher accepts origin-form requests regardless of Host;
`/api/token` returns the bearer token without auth. CSRF checks cover selected
state-changing methods, not token/bootstrap reads. Review explicit Host/origin
validation and test DNS-rebinding/alternate-host behavior in browsers. This is a
source-level concern, not a demonstrated browser exploit in this review.

`loadOrCreateToken` also accepts an existing whitespace-only token file, which
allows empty-token SSE authentication. Reject invalid token files at startup.
For non-loopback binds, bootstrap token disclosure is already part of the
current documented trust model; do not mistake bearer auth alone for remote
access protection.

### 4. Functional API/UI gaps

- Pending-request "Copy request to Sender" uses a stored-log mutation, but the
  logger calls `next(req)` (interception) before storing the request. A new
  request paused in interception has no stored log yet. Introduce a clone path
  for pending items or deliberately change capture semantics, with tests.
- Project opening only invalidates some Apollo cache fields and references a
  nonexistent `Project.type` field in handwritten fragments. Reset/invalidate
  project-dependent results consistently and test switching between populated
  projects, including sender history and interception settings.
- Request URL helpers decode an already encoded query and mishandle fragments.
  Preserve escaped delimiters and fragments; test `%26`, `%23`, repeated keys,
  Unicode and malformed URLs before refactoring the duplicated editing code.
- Settings POST writes disk while GET keeps serving startup `cfg`. Validation is
  duplicated/incomplete; unknown JSON fields and nonpositive body caps are not
  consistently rejected. Centralize validation and distinguish saved settings
  from currently effective settings.
- UI mutations/clipboard actions frequently lack error handling. "Drop all"
  cancels requests even for response-phase items and can leave its loading flag
  set on failure. Handle both phases and restore UI state in `finally`.

### 5. Resource limits and lifecycle

Sender response parsing, interception body filters and GraphQL serialization
still use unbounded `io.ReadAll`; requests without query limits can return all
stored bodies. Apply explicit resource policies rather than silently truncating
forwarded traffic. Expose logged-body truncation in the schema/UI so replaying a
partial capture is not mistaken for a full replay.

Startup uses `logger.Fatal` inside the reusable `run` function and serving
background goroutine. Shutdown handles Interrupt but not SIGTERM, doesn't track
hijacked tunnels, and doesn't drain asynchronous response-log writes. CA creation
also leaves file handles unclosed. Separate setup/serve/teardown, return errors,
and test cancellation, bind failures, CA persistence, and graceful termination.

### 6. Maintainability after correctness fixes

The request-log, sender and interception search evaluators duplicate substantial
logic; request editors duplicate query/header editing and HTTP-body conversions.
Extract small pure operations with common behavioral tests, not a generic
framework. Split the large resolver file along feature boundaries and move
settings handlers out of startup once their behavior has coverage. Preserve
legacy gob registration names when refactoring persisted types.

## Verification and limitations

Executed with Go **1.26.10** and Node.js **22.23.4**:

- `make check`: Go build, tests, race detector, vet, admin lint and stricter
  type-checking passed. npm installation used cached/offline packages.
- `make build` and `./lynx --version`: static UI export and real embedded Go
  binary built and the version command ran successfully.
- Staticcheck over `./cmd/... ./pkg/...` with `noembed`: passed.
- Regression coverage includes full upload/download preservation with both
  plain and gzip responses under a small log cap, project-bound response storage,
  body closure, CONNECT completion, history clearing, protocol mapping and EOF.
- Online dependency scans produced the findings above, not a clean result.

Not verified: interactive browser workflows, actual Go 1.25 execution,
Windows/macOS binaries, Docker runtime/networking, signing/publishing, or the
remaining concurrency/security scenarios. Do not equate successful builds and
current tests with release approval.

### Issue #22 follow-up verification

Before the fix, new regression tests reproduced lost concurrent settings updates,
cloning across mismatched projects, and requests changing their log project after
an inner modifier returned. Focused race tests also reported unsynchronized
Sender/project state access. The fix adds coverage for serialized settings,
concurrent project switching, Sender state access, explicit-project cloning,
in-flight send persistence, scope ownership, and query admission during a partial
project transition. GitHub status remains In review until the local changes are
reviewed and merged; this is not a claim that the release blockers are all fixed.

### Issue #23 follow-up verification

Before the fix, tests reproduced shared pending-body/header mutation, stale
admission after disabling during body preparation, and settings data races.
Regression coverage now includes concurrent request/response views and API
serialization during forwarding, competing clear/cancel/forward actions,
response overrides, body closure/capture errors and response-phase inspection
of a request already consumed by Transport. The real proxy integration test
now approves both phases of a POST and verifies the sent/logged/replayed bodies.
Focused race tests were repeated 20 times. This work remains local and in review;
interactive browser and resource-limit behavior are not claimed verified.

### Issue #43 — HTTP protocol upgrades

A local integration regression reproduced a `502` instead of the expected `101`
when response logging was active. Ordinary body capture replaced the upgraded
bidirectional stream with a read-only body; interception also consumed/closed
it, and gzip handling could try to decode protocol bytes as an HTTP document.

These paths now recognize `101 Switching Protocols`. Decompression and response
interception bypass it (including explicit response overrides); logging/parser
paths retain handshake metadata only, without reading, closing or replacing the
live stream. Request-phase interception remains available. The README documents
that upgraded messages/WebSocket frames are not captured or editable and that
a handshake's zero logged body size does not measure stream traffic.

Regression coverage uses real TCP connections and a local HTTP upgrade server,
with a greeting and repeated client-to-server/server-to-client exchanges. The
27-case matrix covers logging with/without an active project, response settings
and explicit overrides, request-phase approval, gzip headers, client closure,
proxy-context cancellation and upstream closure. Unit tests check untouched
stream ownership, write capability, metadata/redaction/project ownership and
standalone response parsing. This is transport-level upgrade coverage, not an
interactive browser or WebSocket frame-protocol conformance test. Resource
limits (#32), CA onboarding (#44) and review/merge remain separate release work.
