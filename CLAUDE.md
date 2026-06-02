# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

---

## Identity

This is **Lynx** — a fork of [hetty](https://github.com/dstotijn/hetty) by David Stotijn. Rebrand complete (2026-05-08): module path `github.com/lorenzocamilli/lynx`, binary `lynx`, config dir `~/.lynx/`.

- **Full name:** Lynx
- **Short name:** Lynx
- **No wordmark suffix** — the `://` has been removed; the name stands alone

---

## Brand Identity

### Colour Palette

| Token | Hex | Use |
|---|---|---|
| Accent | `#38BDF8` | Buttons, links, active states, highlights, step numbers |
| Page background | `#0D1117` | App background |
| Surface | `#161B22` | Cards, drawers, panels |
| Subtle surface | `#21262D` | Secondary surfaces, selected rows, chip backgrounds |
| Border / divider | `#30363D` | Borders, dividers, card outlines |
| Deep black | `#010409` | AppBar background |
| Body text | `#E6EDF3` | Primary text — 14:1 contrast (WCAG AAA) |
| Muted text | `#8B949E` | Secondary labels, descriptions, captions — 4.6:1 (WCAG AA) |

### Typography

| Role | Typeface | Weight | Use |
|---|---|---|---|
| Body | Inter | 300/400/500/600/700 | All UI copy, labels, form fields, body paragraphs |
| Headings | Space Grotesk | 700 | `h2`–`h6`, page titles, section headers, step numbers |
| Code / Monaco | JetBrains Mono | 400/500/700/800 | Request/response bodies, inline `<code>` |

```css
/* Font stacks */
--font-body:    'Inter', sans-serif;
--font-heading: 'Space Grotesk', sans-serif;
--font-code:    'JetBrains Mono', monospace;

/* Google Fonts import */
/* Inter + Space Grotesk loaded in admin/src/pages/_document.tsx */
/* JetBrains Mono self-hosted via admin/public/style.css */
```

### CSS Variables (quick reference)

```css
:root {
  --color-accent:      #38BDF8;
  --color-bg:          #0D1117;
  --color-surface:     #161B22;
  --color-surface-2:   #21262D;
  --color-border:      #30363D;
  --color-bg-deep:     #010409;
  --color-text:        #E6EDF3;
  --color-text-muted:  #8B949E;

  --font-body:    'Inter', sans-serif;
  --font-heading: 'Space Grotesk', sans-serif;
  --font-code:    'JetBrains Mono', monospace;
}
```

### Source files

| Concern | File |
|---|---|
| MUI theme (palette + typography) | `admin/src/lib/mui/theme.ts` |
| Font loading (Inter + Space Grotesk) | `admin/src/pages/_document.tsx` |
| JetBrains Mono self-hosted faces | `admin/public/style.css` |
| PWA manifest (name, theme-color) | `admin/public/site.webmanifest` |

---

## Future Improvements

Features identified as valuable but not yet implemented, in rough priority order:

1. **Copy as cURL** — right-click any request → copies a `curl` command to clipboard. Purely frontend (one utility function + menu item in `RequestLogs.tsx` and `History.tsx`), no backend changes needed.
2. **Import from cURL** — paste a curl command into Sender to pre-populate the request. Natural companion to #1.
3. **Match & Replace rules** — auto-modify requests/responses based on configurable rules (e.g. swap headers, strip cookies). Full-stack: new service + bolt bucket + GraphQL mutations + UI tab. Equivalent to Burp's Match and Replace.
4. **Response time + size columns** in proxy logs — requires storing timing/size in `RequestLog`, small schema change.
5. **Repeat N times** in Sender — send the same request N times (basis for simple fuzzing).

## Performance Improvements

1. ~~**Backend pagination for proxy logs and sender history**~~ — done (2026-04-29). Newest-first display; "Load older" appends at bottom; absolute row numbers via `httpRequestLogsCount`; scroll preserved via Apollo `previousData`.
2. ~~**Raise poll intervals for logs and sender**~~ — done (4000ms).
3. ~~**Replace polling with SSE or WebSockets**~~ — done (SSE). See 2026-04-24 session log.
4. **Memoize `RequestsTable` rows** — on every poll Apollo re-renders the whole table even if data is unchanged. Wrap the row component in `React.memo` or ensure Apollo cache normalizes by `id` so unchanged rows skip re-render. (`admin/src/lib/components/RequestsTable.tsx`)
5. **Debounce search/filter input** — if the search field triggers a refetch on every keystroke, that causes unnecessary DB hits. A 300ms debounce is a one-liner fix. Check `admin/src/features/reqlog/components/Search.tsx`.
6. **Store request/response bodies separately** — the gob-encoded `RequestLog`/`SenderRequest` bundles headers and full body in one blob, so list queries decode everything even though the table only displays method/URL/status. Storing bodies under a separate bucket key would make list scans much cheaper. Most invasive change — requires a data migration.

---

## Stack / Dependency Debt

- **Upgrade `gqlgen` v0.14 → v0.17** — the main source of the manual `generated.go` patching. Breaking changes in resolver interfaces make this non-trivial but it would restore `go run github.com/99designs/gqlgen generate` as a working workflow.
- **Bump `go` directive in `go.mod`** — currently `go 1.23`, runtime is Go 1.25. Low priority, just unlocks newer stdlib features.

---

## Session Change Log

At the end of every code session, append a dated entry here summarizing all final changes made (what was changed, why, and which files were affected). Use this format:

```
### YYYY-MM-DD
- <change summary> (`path/to/file.go`)
```

Do not log exploratory reads or intermediate steps — only the final committed changes.

### 2026-06-02 (session 2)
- Phase 5 — operational hardening (server lifecycle). Bounded graceful shutdown: `httpServer.Shutdown` now uses a 10s `context.WithTimeout` instead of `context.Background()`, so a hung/slow connection can no longer block process exit forever (the "Press Ctrl+C to force quit" message now has real backing). Added `ReadHeaderTimeout` (20s) to the admin `http.Server` to mitigate slow-header (Slowloris) attacks — chosen over `ReadTimeout`/`WriteTimeout` because it only applies before a CONNECT tunnel is hijacked, so it doesn't cap long-lived proxy connections or streamed bodies. New `shutdownTimeout`/`readHeaderTimeout` consts. (`cmd/lynx/lynx.go`)

### 2026-06-02
- Phase 3.2 — release pipeline. Migrated `.goreleaser.yml` from the deprecated v1 schema to v2: added `version: 2`, replaced `archives.replacements` with a `name_template` (title-cased OS, `x86_64` for amd64), `format` → `formats` in `format_overrides`, `snapshot.name_template` → `version_template`. Stripped to the chosen **GitHub Release + ghcr only** scope: removed the `brews`, `scoop`, `snapcrafts` blocks and the Docker Hub (`lorenzocamilli/lynx:*`) image tags, keeping only `ghcr.io` tags; added an explicit `release.github` + `prerelease: auto` block. Added `.github/workflows/release.yml`: triggers on `v*` tags, logs in to ghcr with `GITHUB_TOKEN` (no extra secrets), runs `goreleaser-action@v6` (`~> v2`) with `release --clean`. Validated via `goreleaser check` (Docker image). Note: goreleaser warns `dockers` is being phased out in favour of `dockers_v2` — non-breaking, deferred. (`.goreleaser.yml`, `.github/workflows/release.yml`)

### 2026-06-01
- Phase 3.1 — version injection. `Makefile` now derives `VERSION` from `git describe --tags --always --dirty` (leading `v` stripped) and passes `-ldflags "-s -w -X main.version=$(VERSION)"` to `go build`, so local/CI builds embed a real version instead of the `0.0.0` default. Added a `-v`/`--version` flag to `main()` that prints `lynx <version>` and exits before loading config (matches the goreleaser Homebrew `lynx -v` test). Release workflow (Phase 3.2) intentionally deferred. (`Makefile`, `cmd/lynx/main.go`)
- Fixed "Copy request to Sender" issues. (1) Cloned requests didn't appear in the Sender history list: `History` ran `GetSenderRequests` with default `cache-first` and never refetched, so the request created on the proxy-logs route was written to the DB but the list cache stayed stale. Switched the query to `cache-and-network` so the list refreshes whenever `/sender` mounts, and changed the render guards to key off `data` instead of `loading` to avoid the list flickering during background refetches. Also added `refetchQueries: ["GetSenderRequests"]` to the `createOrUpdateSenderRequest` mutation to cover the create-and-send flow where `History` is already mounted. (`admin/src/features/sender/components/History.tsx`, `admin/src/features/sender/components/EditRequest.tsx`)
- Fixed Sender form loading the wrong protocol: the request-load `useEffect` synced url/method/body/headers/queryParams but never set `proto`, so the protocol dropdown always showed the default HTTP/2.0. Now reverse-maps `senderRequest.proto` through `httpProtoMap` and sets the proto state. (`admin/src/features/sender/components/EditRequest.tsx`)

### 2026-05-25
- Gated GraphQL Playground behind `LYNX_DEV=1` env var; returns 404 in production — prevents schema exposure. (`pkg/api/http.go`)
- Added configurable body size limit (`MaxBodyBytes`, default 10 MB) to `config.Config` and `reqlog.Config`; applied `io.LimitReader` to request and response bodies in `RequestModifier`/`ResponseModifier` with an info log on truncation. (`pkg/config/config.go`, `pkg/reqlog/reqlog.go`, `cmd/lynx/lynx.go`)
- Eliminated all deprecated `io/ioutil` usage — replaced `ioutil.ReadAll`/`ioutil.NopCloser` with `io.ReadAll`/`io.NopCloser` across three packages. (`pkg/reqlog/reqlog.go`, `pkg/proxy/intercept/filter.go`, `pkg/api/resolvers.go`)
- Fixed `go test ./...` traversal: all Makefile and CI test commands now use `./cmd/... ./pkg/...` to avoid entering `admin/node_modules`. (`Makefile`, `.github/workflows/build-test.yml`)
- Added `make check` target: builds, tests, race-detects, vets Go code, and runs admin lint + typecheck. (`Makefile`)
- Added `npm run typecheck` script (`tsc --noEmit`) to admin. (`admin/package.json`)
- Switched `npm install` → `npm ci` in Makefile and CI for reproducible builds. (`Makefile`, `.github/workflows/lint.yml`)
- Added `test-race` and `govulncheck` jobs to `build-test.yml`. (`.github/workflows/build-test.yml`)
- Added `lint-go` job (golangci-lint-action@v6) and `npm audit --omit=dev --audit-level=high` gate to `lint.yml`. (`.github/workflows/lint.yml`)
- Created `.github/dependabot.yml`: weekly auto-PRs for Go modules, npm, Docker, and GitHub Actions. (`.github/dependabot.yml`)

### 2026-05-21 (session 3)
- Renamed `cmd/lynx/hetty.go` → `cmd/lynx/lynx.go` — last remaining file with the old upstream name. (`cmd/lynx/lynx.go`)
- Rewrote `.goreleaser.yml`: replaced all `hetty`/`hettysoft`/`dstotijn` references with `lynx`/`lorenzocamilli`; updated binary name, Docker image tags, Homebrew/Scoop/Snap entries, build arg, commit author, and source URL. (`.goreleaser.yml`)
- Fixed `.dockerignore`: `/hetty` and `/cmd/hetty/admin` → `/lynx` and `/cmd/lynx/admin`. (`.dockerignore`)
- Generated all favicon sizes from `linx-logo.jpg` using ImageMagick: `favicon-16x16.png`, `favicon-32x32.png`, `apple-touch-icon.png`, `android-chrome-192x192.png`, `android-chrome-512x512.png`, `favicon.ico`. Added missing favicon `<link>` tags to `_document.tsx` (they were absent, so the browser never picked up the custom icon). (`admin/public/`, `admin/src/pages/_document.tsx`)

### 2026-05-21 (session 2)
- Bound server to `127.0.0.1` by default: added `Host string` field to `Config` (default `127.0.0.1`), updated listen address to `host:port` format, added host validation in settings POST handler. (`pkg/config/config.go`, `cmd/lynx/lynx.go`)
- Added local admin token authentication: `loadOrCreateToken` generates a 32-byte hex token on first run and stores it in `~/.lynx/token` (0600); token is printed to the terminal at startup; `GET /api/token` serves it without auth (protected by localhost bind); middleware requires `Authorization: Bearer <token>` on all other `/api/*` routes. (`cmd/lynx/lynx.go`)
- Added Origin/Host CSRF protection: middleware rejects state-changing requests (POST/PUT/DELETE) whose `Origin` header doesn't match the configured listen address. Wired alongside auth middleware. (`cmd/lynx/lynx.go`)
- Frontend auth wiring: new `lib/auth.ts` exports `getToken()` (fetches `/api/token`, caches in memory + localStorage) and `authedFetch()` (adds Bearer header); Apollo Client extended with `setContext` authLink; Settings.tsx `fetch` calls migrated to `authedFetch`. (`admin/src/lib/auth.ts`, `admin/src/lib/graphql/useApollo.ts`, `admin/src/features/settings/components/Settings.tsx`)
- Settings UI: added "Listen address" field for bind host with a warning when set to `0.0.0.0`; added "Admin token" read-only display with copy button; updated port helper text; fixed stale `lynxpipe` links → correct `lorenzocamilli/lynx`. (`admin/src/features/settings/components/Settings.tsx`)
- Created `PLAN.md` (repo root): full 9-phase, 3-milestone development roadmap derived from codebase audit (`CODEBASE_REPORT.md`). (`PLAN.md`)
- Fixed all ESLint import/order and Prettier formatting errors surfaced during `make build-admin`: moved `ContentCopyIcon` import before `@mui/lab` in Settings.tsx; moved `React` import after `@mui/material` in RequestsTable.tsx; collapsed multi-line expressions that Prettier wanted on one line in History.tsx and EditRequest.tsx. (`admin/src/features/settings/components/Settings.tsx`, `admin/src/lib/components/RequestsTable.tsx`, `admin/src/features/sender/components/History.tsx`, `admin/src/features/sender/components/EditRequest.tsx`)

### 2026-05-21
- Fixed `.gitignore`: removed dead `/hetty` and `/cmd/hetty/admin` entries, added `/lynx`, `/gqlgen`, `/cmd/lynx/admin`. Untracked `lynx` binary, `gqlgen` binary, and all 49 built `cmd/lynx/admin/` files from git (`git rm --cached`). Deleted stale `PLAN.md` (already untracked). (`.gitignore`)
- Updated CI workflows: `build-test.yml` now tests Go 1.24 and 1.25 (dropped 1.21–1.23), Node 20, and action versions `@v4`; `lint.yml` updated from yarn/Node 16 to npm/Node 20 with `@v4` actions. (`.github/workflows/build-test.yml`, `.github/workflows/lint.yml`)
- Added "Copy as cURL" to proxy log and sender history context menus: new `curlFromRequest` utility generates shell-safe `curl` commands (single-quote escaping for URL, headers, body); lazy GraphQL queries fetch full request data on demand; clipboard API copies the result; success Snackbar confirms copy. (`admin/src/lib/curlFromRequest.ts`, `admin/src/features/reqlog/components/RequestLogs.tsx`, `admin/src/features/sender/components/History.tsx`)
- Added "Import from cURL" to Sender: new `parseCurl` utility tokenizes and parses curl commands (handles `-X`, `-H`, `-d`/`--data-raw`, line continuations, single/double-quoted strings); clipboard-icon button opens a MUI Dialog; on Import, method/URL/queryParams/headers/body are all populated into the form. (`admin/src/lib/parseCurl.ts`, `admin/src/features/sender/components/EditRequest.tsx`)
- Added `TestDeleteRequestLog` and `TestCountRequestLogs` to bolt reqlog tests; `TestDeleteSenderRequest` to bolt sender tests — verify delete + `ErrRequestNotFound` + idempotent re-delete + count reflects deletes. (`pkg/db/bolt/reqlog_test.go`, `pkg/db/bolt/sender_test.go`)
- Created `pkg/sse/sse_test.go` with three black-box tests: single-client broadcast, multi-client fan-out, client-disconnect cleanup. Fixed production bug: SSE handler now calls `flusher.Flush()` immediately after setting headers so the client receives the `200 OK` and `Content-Type: text/event-stream` before any event arrives (previously headers were only sent on first event). (`pkg/sse/sse.go`, `pkg/sse/sse_test.go`)
- Bumped `go` directive from `1.23.0` to `1.25.0` to match the Dockerfile's `golang:1.25-alpine` base. (`go.mod`)

### 2026-05-08
- Full rebrand from hetty → Lynx. Renamed `cmd/hetty/` → `cmd/lynx/`; updated Go module path `github.com/dstotijn/hetty` → `github.com/lorenzocamilli/lynx` in `go.mod` and all 27 `.go` files; updated file paths (`~/.lynx/`), proxy hostname (`lynx.proxy`), CA subject (`Lynx CA`), binary name, startup log. (`go.mod`, `cmd/lynx/lynx.go`, `pkg/config/config.go`, `pkg/proxy/cert.go`)
- Updated build/deploy configs: binary cmd path, ARG names, image tags, Homebrew/Scoop/Snap entries, Docker image labels. (`Makefile`, `Dockerfile`, `.goreleaser.yml`, `.golangci.yml`, `gqlgen.yml`)
- Updated all frontend UI strings (`Hetty://` → `Lynx://`, welcome text, settings body, scope and projects descriptions, dead `hetty.xyz` links replaced). (`admin/src/pages/_app.tsx`, `admin/src/features/Layout.tsx`, `admin/src/pages/index.tsx`, `admin/src/features/settings/components/Settings.tsx`, `admin/src/pages/projects/index.tsx`, `admin/src/pages/scope/index.tsx`)
- Updated MUI theme: new GitHub dark palette (primary `#38BDF8`, bg `#0D1117`/`#161B22`, text `#E6EDF3`), `fontFamily: 'Inter'`; swapped Roboto/Space Mono Google Fonts for Inter (weights 300–700). (`admin/src/lib/mui/theme.ts`, `admin/src/pages/_document.tsx`)
- Updated `admin/package.json` name, `site.webmanifest` name/theme-color, `README.md` (full rewrite), `CONTRIBUTING.md` (GitHub URLs), `CLAUDE.md` (identity section, architecture references). (`admin/package.json`, `admin/public/site.webmanifest`, `README.md`, `CONTRIBUTING.md`, `CLAUDE.md`)
- Replaced heading font JetBrains Mono with **Space Grotesk** (weight 700) — more legible display font for headings while keeping JetBrains Mono for code only; added Space Grotesk to Google Fonts import. (`admin/src/lib/mui/theme.ts`, `admin/src/pages/_document.tsx`)
- Removed `://` wordmark suffix from all UI strings (page title, sidebar logo, homepage hero). (`admin/src/pages/_app.tsx`, `admin/src/features/Layout.tsx`, `admin/src/pages/index.tsx`)
- Completely redesigned homepage: hero section with tagline and CTAs, 4-feature card grid (Proxy logs, Intercept, Sender, Scope), numbered 3-step getting-started guide. (`admin/src/pages/index.tsx`)
- Added Brand Identity section to `CLAUDE.md` covering full colour palette, typography roles/weights, CSS variable quick-reference, and source file pointers. (`CLAUDE.md`)
- Settled on final name **Lynx** (dropped LynxPipe): reverted module path back to `github.com/lorenzocamilli/lynx`, all UI strings back to "Lynx", Docker image tags, site.webmanifest, docs. Everything — public name, binary, module path, repo — is now uniformly `lynx`. (`go.mod`, `.golangci.yml`, `gqlgen.yml`, `.goreleaser.yml`, all `.go` imports, `admin/src/**/*.tsx`, `admin/public/site.webmanifest`, `README.md`, `CONTRIBUTING.md`, `CLAUDE.md`)

### 2026-05-07
- Resolved merge conflict between `dev` and `chore/update-frontend`: `dev` was missing the SSE broadcaster wiring; `chore/update-frontend` was missing `pkg/config`, settings endpoints, and CA cert endpoint. Combined both: `run()` function structure from `dev`, `Broadcaster` fields in `reqLogService`/`interceptService` and `/api/events` route from `chore/update-frontend`. (`cmd/lynx/lynx.go`, `pkg/config/config.go`)
- Fixed duplicate `formatContent` function in `Editor.tsx` introduced by the merge (both branches had independently added the same function). (`admin/src/lib/components/Editor.tsx`)

### 2026-04-29
- Fixed proxy log pagination UX: newest request now at top (highest absolute sequence number), "Load older" appends older requests at the bottom without resetting scroll. Used Apollo `previousData` to keep the table visible during refetch so scroll position is preserved. (`admin/src/features/reqlog/components/RequestLogs.tsx`)
- Added `httpRequestLogsCount: Int!` GraphQL query for absolute row numbering: bolt `CountRequestLogs` uses `b.Stats().KeyN`; service `CountRequests` wraps it; resolver `HTTPRequestLogsCount` returns 0 on no-project. Manually patched `generated.go` (ComplexityRoot field, QueryResolver interface, complexity case, execution function, dispatcher case, embedded schema string). (`pkg/db/bolt/reqlog.go`, `pkg/reqlog/repo.go`, `pkg/reqlog/reqlog.go`, `pkg/api/schema.graphql`, `pkg/api/generated.go`, `pkg/api/resolvers.go`)
- Added `rowNumberBase` prop to `RequestsTable`: when provided, uses it as the base for row numbering (`base - index`) so the top row always shows the absolute sequence number (e.g. #200 if 200 total requests). Falls back to `rows.length` when base is 0. (`admin/src/lib/components/RequestsTable.tsx`)
- Frontend count query regenerated via `npm run generate`. (`admin/src/features/reqlog/graphql/httpRequestLogs.graphql`, `admin/src/lib/graphql/generated.tsx`)

### 2026-04-28
- Backend pagination for proxy logs and sender history: `FindRequestLogs` and `FindSenderRequests` now use BoltDB cursor-based reverse iteration (newest-first) with `limit`/`offset` support, replacing the full `b.ForEach` scan. (`pkg/db/bolt/reqlog.go`, `pkg/db/bolt/sender.go`)
- Added `Limit` and `Offset` fields to `FindRequestsFilter` in both reqlog and sender packages; `FindRequests` now accepts `limit, offset int` and merges them into a copy of the stored filter before each query. (`pkg/reqlog/reqlog.go`, `pkg/sender/sender.go`)
- Added `limit: Int, offset: Int` args to `httpRequestLogs` and `senderRequests` in the GraphQL schema and manually patched `generated.go`: updated `QueryResolver` interface, `ComplexityRoot.Query` field types, added `field_Query_httpRequestLogs_args`/`field_Query_senderRequests_args` functions, updated both execution functions to parse and pass args, added `unmarshalOInt2ᚖint` helper. (`pkg/api/schema.graphql`, `pkg/api/generated.go`)
- Updated resolvers to accept `limit *int, offset *int` and pass resolved values to the service layer. (`pkg/api/resolvers.go`)
- Updated frontend GraphQL queries to accept `$limit: Int, $offset: Int` variables and regenerated TypeScript types/hooks. (`admin/src/features/reqlog/graphql/httpRequestLogs.graphql`, `admin/src/features/sender/graphql/senderRequests.graphql`, `admin/src/lib/graphql/generated.tsx`)
- Added "Load more" button (increments limit by 50) to proxy logs and sender history; default page size is 50 rows. (`admin/src/features/reqlog/components/RequestLogs.tsx`, `admin/src/features/sender/components/History.tsx`)

### 2026-04-24 (session 2)
- Fixed SSE regression: `request_log` events fired before the response was stored, so clicking a newly appeared row showed no body. `LogDetail` now subscribes to SSE and refetches the detail query on `response_log`, so the body fills in automatically without re-clicking. (`admin/src/features/reqlog/components/LogDetail.tsx`)
- Fixed SSE performance regression: no debounce meant a browser page load (30 sub-resources) triggered 60 immediate refetches. Added `debounceMs` param to `useSSE` hook (default 0); `RequestLogs` uses 300ms to collapse bursts into a single refetch. Intercept context left at 0ms (must remain instant). (`admin/src/lib/useSSE.ts`, `admin/src/features/reqlog/components/RequestLogs.tsx`)
- Memoized `RequestsTable` rows with `React.memo`: extracted `RequestRow` component; used ref+`useCallback` pattern inside `RequestsTable` to provide stable function references so memo actually skips re-renders on unchanged rows. Fixed latent crash: replaced non-null assertion on `stableActionsCell` with safe optional call. (`admin/src/lib/components/RequestsTable.tsx`)

### 2026-04-24
- Replaced Apollo polling with SSE for real-time updates — eliminated all idle DB reads. Created `pkg/sse/sse.go`: a thread-safe broadcaster hub with non-blocking fan-out to connected SSE clients; HTTP handler serves `GET /api/events` as a `text/event-stream` endpoint. (`pkg/sse/sse.go`)
- Wired broadcaster into `reqlog.Service`: emits `request_log` after storing a request, `response_log` after storing a response (in the async goroutine). (`pkg/reqlog/reqlog.go`)
- Wired broadcaster into `intercept.Service`: emits `intercepted` when a request or response enters or leaves the intercept maps (both on arrival and on send/cancel/timeout). Restructured deferred cleanup to use direct unlock so emit runs after the lock is released. (`pkg/proxy/intercept/intercept.go`)
- Registered `GET /api/events` route on `adminRouter` in the server entrypoint; injected broadcaster into both service configs. (`cmd/lynx/lynx.go`)
- Created `useSSE` React hook wrapping `EventSource`; uses a ref so the callback is always current without re-creating the SSE connection. (`admin/src/lib/useSSE.ts`)
- Replaced 1 s poll in `InterceptedRequestsContext` with SSE `intercepted` event. (`admin/src/lib/InterceptedRequestsContext.tsx`)
- Replaced 4 s poll in `RequestLogs` with SSE `request_log`/`response_log` events. (`admin/src/features/reqlog/components/RequestLogs.tsx`)
- Removed 4 s poll from sender `History` — sender requests are created by explicit user mutations so Apollo cache invalidation handles updates without polling or SSE. (`admin/src/features/sender/components/History.tsx`)
- Added `/api/events` dev-mode proxy rewrite to `next.config.js` so the Next.js dev server forwards the SSE connection to the Go backend. (`admin/next.config.js`)

### 2026-04-22
- Removed `pkg/chrome` package and all chromedp usage: deleted `pkg/chrome/chrome.go`, removed `--chrome` flag and startup block from `cmd/lynx/lynx.go`, dropped `chromedp`, `cdproto`, and `sysutil` from `go.mod`/`go.sum` via `go mod tidy`. (`pkg/chrome/chrome.go`, `cmd/lynx/lynx.go`, `go.mod`, `go.sum`)
- Raised poll interval for proxy logs and sender history from 1000ms to 4000ms — neither view needs sub-second freshness, cuts DB reads by 4x. (`admin/src/features/reqlog/components/RequestLogs.tsx`, `admin/src/features/sender/components/History.tsx`)

### 2026-04-18
- Added right-click "Delete request" to the proxy log table: new `deleteHTTPRequestLog(id: ID!)` GraphQL mutation reusing `ClearHTTPRequestLogResult`; bolt deletes the single record from the request_logs bucket; resolver wired in resolvers.go; `generated.go` patched manually (gqlgen bootstrap issue). (`pkg/api/schema.graphql`, `pkg/api/generated.go`, `pkg/api/resolvers.go`, `pkg/reqlog/repo.go`, `pkg/reqlog/reqlog.go`, `pkg/db/bolt/reqlog.go`, `admin/src/features/reqlog/graphql/deleteHttpRequestLog.graphql`, `admin/src/features/reqlog/components/RequestLogs.tsx`)
- Added right-click "Delete request" to the Sender history list: new `deleteSenderRequest(id: ID!)` GraphQL mutation reusing `DeleteSenderRequestsResult`; bolt deletes the single key from the sender_requests bucket; service method `DeleteRequest` wraps the repo call; `History.tsx` navigates away if the deleted request was active. (`pkg/api/schema.graphql`, `pkg/api/generated.go`, `pkg/api/resolvers.go`, `pkg/sender/repo.go`, `pkg/sender/sender.go`, `pkg/db/bolt/sender.go`, `admin/src/features/sender/graphql/deleteSenderRequest.graphql`, `admin/src/features/sender/components/History.tsx`)

### 2026-04-15 (session 3)
- Fixed Sender showing raw binary when the server returns a gzip-encoded response body: `ParseHTTPResponse` now checks `Content-Encoding: gzip`, wraps the body in a `gzip.Reader` before reading, and strips the header from the logged response — the proxy path was already decompressing before calling this function so it's a no-op there. (`pkg/reqlog/reqlog.go`)

### 2026-04-15 (session 2)
- Changed intercepted requests list to display oldest-first (top = #1, newest = highest number): used the existing `oldestFirst` prop in `RequestsTable` to reverse the render order and compute row numbers as `index + 1`; passed `oldestFirst` from the intercept `Requests` component. (`admin/src/lib/components/RequestsTable.tsx`, `admin/src/features/intercept/components/Requests.tsx`)

### 2026-04-15
- Fixed JSON response bodies being unreadable in sender (and proxy logs/intercept): replaced exact-string switch in `languageForContentType` with `startsWith`/`includes` so content-type variants like `application/json;charset=utf-8` and `application/vnd.api+json` correctly get JSON highlighting; added `formatContent()` that pretty-prints JSON bodies with `JSON.stringify(JSON.parse(...), null, 2)`, falling back silently to raw content on parse failure. (`admin/src/lib/components/Editor.tsx`)

### 2026-04-14
- Changed `Config` struct from `Addr string` (host:port) to `Port int`; default is `8080`; config.yaml now stores `port: 8080`. (`pkg/config/config.go`)
- Updated server entrypoint to build listen address as `fmt.Sprintf(":%d", cfg.Port)`; updated admin router host matcher to use `cfg.Port` directly; removed `net.SplitHostPort` call at startup. (`cmd/lynx/lynx.go`)
- Updated `POST /api/settings` handler to validate `1 ≤ port ≤ 65535` instead of parsing a host:port string — fixes crash when user entered a bare port number like `9090`. (`cmd/lynx/lynx.go`)
- Pretty-printed JSON on both `GET` and `POST /api/settings` responses using `enc.SetIndent("", "  ")`. (`cmd/lynx/lynx.go`)
- Changed Settings UI "Listen address" free-text field to a `type="number"` "Listen port" field (1–65535). (`admin/src/features/settings/components/Settings.tsx`)
- Added `formatContent()` to the Monaco editor wrapper: when content type is `application/json`, pretty-prints the body with `JSON.stringify(JSON.parse(...), null, 2)` before display; falls back to raw content if parsing fails. Applies to request logs, intercept, and sender views. (`admin/src/lib/components/Editor.tsx`)
- Removed copy icon button column from proxy history table; "Copy request to Sender" is now only accessible via right-click context menu. (`admin/src/features/reqlog/components/RequestLogs.tsx`)
- Added row number column on the left of the proxy history table; oldest request is #1, newest has the highest number (backend returns newest-first so number is computed as `requests.length - index`). (`admin/src/lib/components/RequestsTable.tsx`)
- Fixed pre-existing panic in `CancelRequest`: it passes `nil` to `ModifyRequest` which was unconditionally dereferencing it — wrapped the `WithContext` calls in a nil check. (`pkg/proxy/intercept/intercept.go`)
- Added "Drop all" button above the intercepted requests table; calls `cancelRequest` sequentially for each pending request then navigates back to `/proxy/intercept`. (`admin/src/features/intercept/components/Requests.tsx`)
- Added right-click context menu to intercepted requests list with two options: "Intercept response" (forwards the request with `modifyResponse: true`, clears it from cache, navigates to it so the intercepted response appears automatically) and "Copy request to Sender". (`admin/src/features/intercept/components/Requests.tsx`)
- Removed enable/disable switches for request and response interception from Settings; kept only the filter text fields. (`admin/src/features/settings/components/Settings.tsx`)
- Added "Intercept" toggle switch directly in the intercept view action bar (alongside Send/Cancel buttons) for instant on/off without visiting Settings. Removed the settings gear icon button. (`admin/src/features/intercept/components/EditRequest.tsx`)

### 2026-04-10
- Removed all CLI flags and the entire `ffcli` subcommand framework, including `cert install`/`cert uninstall` (cert download handled via `/api/ca.crt` UI button), `--chrome`, `--verbose`, `--json`, `--cert`, `--key`, `--addr`, `--db`. (`cmd/lynx/cert.go` deleted, `cmd/lynx/config.go` deleted)
- Deleted `pkg/chrome` package and removed `chromedp` and `ffcli` dependencies. (`go.mod`, `go.sum`)
- All file paths hardcoded to `~/.lynx/` defaults (cert, key, db). Logger always uses human-readable console format. (`cmd/lynx/lynx.go`)
- Created `pkg/config/config.go`: reads/writes `~/.lynx/config.yaml` (only field: `port`); `Load` returns defaults when file doesn't exist, `Save` creates parent directories. (`pkg/config/config.go`)
- Entry point now loads config from YAML and starts the server — no flags. (`cmd/lynx/main.go`)
- Added `GET /api/settings` and `POST /api/settings` REST endpoints for reading/writing `config.yaml` at runtime. (`cmd/lynx/lynx.go`)
- Added Settings gear icon and nav link to the sidebar. (`admin/src/features/Layout.tsx`)
- Added "Application" tab to the Settings page with a Listen port field; saves via `POST /api/settings` and shows a persistent "restart required" warning banner after save. (`admin/src/features/settings/components/Settings.tsx`)
- Deleted stray untracked `pkg/api/resolver_root.go` (leftover from a previous `gqlgen generate` run, conflicted with `resolvers.go`).

### 2026-04-09
- Added CA certificate download endpoint `GET /api/ca.crt` serving DER-encoded cert with `Content-Type: application/x-x509-ca-cert` so browsers (Firefox/Chrome) trigger their native CA import dialog. (`cmd/lynx/lynx.go`)
- Added "Download CA certificate" button on the homepage linking to `/api/ca.crt` without a `download` attribute, so the browser handles the MIME type and shows the trust dialog instead of saving a file. (`admin/src/pages/index.tsx`)

### 2026-04-07 (session 2)
**Dependency upgrades — eliminated all npm deprecation warnings:**
- Upgraded Next.js 14→15.5.14 (fixes glob@10 from @next/eslint-plugin-next) and ESLint 8→9 (fixes eslint@8, @humanwhocodes/object-schema, @humanwhocodes/config-array). (`admin/package.json`)
- Upgraded @typescript-eslint v5→v8, eslint-plugin-prettier v4→v5, eslint-config-prettier v8→v10, prettier v2→v3, added @eslint/eslintrc@^3. (`admin/package.json`)
- Pinned `trailingComma: "es5"` in prettierrc to preserve existing formatting under prettier v3 (v3 changed default to "all"). (`admin/.prettierrc.json`)
- Switched Makefile from `npm ci` to `npm install` (package-lock.json was not yet committed). Added `rm -rf ../cmd/lynx/admin` before `mv dist` to prevent nested output dir on repeated builds. (`Makefile`)
- Note: `@mui/base@5.0.0-beta.70` deprecation warning cannot be eliminated — transitive dep of `@mui/lab@6.0.1-beta.36` (no stable @mui/lab@6 exists yet).

**ESLint flat config migration (ESLint 9 requires flat config to avoid legacy-mode warnings):**
- Deleted `.eslintrc.json`, created `eslint.config.mjs` using FlatCompat for all extends (next/core-web-vitals, plugin:@typescript-eslint/recommended, plugin:import/typescript) and native recommended export for eslint-plugin-prettier. (`admin/eslint.config.mjs`, deleted `admin/.eslintrc.json`)
- Added `allowShortCircuit/allowTernary` to `@typescript-eslint/no-unused-expressions` (v8 is stricter about `&&` JSX patterns). Turned off `import/named` (false positives with flat config resolver; TypeScript covers this at compile time). (`admin/eslint.config.mjs`)

**Source fixes required by stricter ESLint 9 / prettier v3:**
- prettier v3 trailing comma: added missing trailing comma in tuple return type. (`admin/src/lib/components/useContextMenu.tsx`)
- prettier v3 function signature formatting. (`admin/src/lib/components/Link.tsx`)
- Unused catch binding: `catch (e) {}` → `catch {}`. (`admin/src/features/scope/components/AddRule.tsx`)
- Changed `import clsx from "clsx"` to named import `import { clsx } from "clsx"` (clsx@2 exports a named export; default import triggered `import/no-named-as-default` warning). (`admin/src/lib/components/Link.tsx`)
- Removed stale `// eslint-disable-next-line react/no-danger` (rule not active in new config; ESLint 9 flags unused disable directives). (`admin/src/pages/_document.tsx`)

**Apollo Client 3.14 `onCompleted` warnings fixed:**
- Apollo 3.14 fires `onCompleted` on every render where cached data is available, not just on initial fetch — setting state inside it can cause infinite loops. Replaced `onCompleted` on `useQuery` hooks with `useEffect` watching `data` in all three affected components. (`admin/src/features/intercept/components/EditRequest.tsx`, `admin/src/features/reqlog/components/Search.tsx`, `admin/src/features/sender/components/EditRequest.tsx`)

### 2026-04-07
- Upgraded frontend dependencies: Next.js 12→14.2.35, React 17→18.3.1, MUI v5→v6.5, Apollo Client 3.2→3.14.1, TypeScript 4→5.8.3. (`admin/package.json`)
- Replaced `next export` (removed in Next.js 14) with `output: 'export'` + `distDir: 'dist'` in next.config.js, gated behind `NEXT_EXPORT=1` env so dev rewrites still work. (`admin/next.config.js`, `admin/package.json`, `Makefile`)
- Fixed Next.js 14 `Link` compatibility: added `legacyBehavior` to `NextLink`, removed deprecated `locale` prop. (`admin/src/lib/components/Link.tsx`)
- Fixed MUI v6 Grid v2 breaking change: removed `item` prop (no longer exists in v2). (`admin/src/pages/scope/index.tsx`, `admin/src/pages/projects/index.tsx`)
- Migrated `Alert` imports from `@mui/lab` to `@mui/material` (promoted in MUI v5.4, removed from lab in v6). Merged duplicate MUI imports. (`admin/src/features/scope/components/Rules.tsx`, `AddRule.tsx`; `admin/src/features/reqlog/components/Actions.tsx`, `Search.tsx`, `LogDetail.tsx`; `admin/src/features/projects/components/ProjectList.tsx`)
- Fixed MUI v6 `styled()` class-component props issue: cast `ReactSplitPane` to `React.ComponentType<SplitPaneProps & { children?: React.ReactNode }>` so children prop is recognized. (`admin/src/lib/components/SplitPane.tsx`)
- Fixed Apollo Client 3.14 stricter `cache.modify` types: replaced `HttpRequest[]` with `readonly Reference[]` on modifier callbacks. (`admin/src/features/intercept/components/EditRequest.tsx`)
- Fixed `ApolloError` not assignable to `ReactNode`: use `.message` in Alert. (`admin/src/features/reqlog/components/Actions.tsx`)
- Switched build from yarn to npm (yarn not installed on build host). (`Makefile`)

### 2026-04-02 (session 2)
- Fixed `ClearRequestLogs` deleting the request logs bucket without recreating it, causing "request logs bucket not found" errors on all subsequent proxy requests after clearing logs via the UI. Now deletes then immediately recreates the empty bucket. Also imported `go.etcd.io/bbolt/errors` to use the non-deprecated `ErrBucketNotFound`. (`pkg/db/bolt/reqlog.go`)
- Fixed `RequestModifier` not setting `LogBypassedKey` on a `StoreRequestLog` failure, which caused `ResponseModifier` to return a hard error ("reqlog: request is missing ID") and the proxy to return 502 for every proxied request. Now sets `LogBypassedKey=true` on storage failure so the response modifier skips gracefully. (`pkg/reqlog/reqlog.go`)

### 2026-04-02
- Updated Go direct dependencies: `bbolt` beta.0→v1.4.3 (stable), `gorilla/mux` v1.7.4→v1.8.1, `google/go-cmp` v0.5.6→v0.7.0, `peterbourgon/ff/v3` v3.1.2→v3.4.0, `smallstep/truststore` v0.11.0→v0.13.0, `zap` v1.21.0→v1.27.1; updated indirect deps accordingly. Kept `gqlgen`/`gqlparser` pinned (require code regeneration to update) and `chromedp` at v0.7.8 (newer versions require Go 1.24+ via cdproto dependency chain, incompatible with CI matrix). (`go.mod`, `go.sum`)
- Removed stale CLAUDE.md note about bbolt beta needing upgrade (`CLAUDE.md`)

---

## What is Lynx

Lynx is an open-source HTTP security testing toolkit: a MITM proxy, request interceptor/replayer, and web UI packaged as a single statically-linked binary. The embedded Next.js admin UI and the Go backend are both served on the same port (default `:8080`).

---

## Commands

### Build

```bash
make build          # Build admin UI (Next.js), then Go binary at ./hetty
make build-admin    # Build only the Next.js UI (outputs to cmd/lynx/admin/)
make clean          # Remove build artifacts
```

The Go binary embeds the admin UI via `//go:embed admin` in [cmd/lynx/lynx.go](cmd/lynx/lynx.go). The admin must be built first (`make build-admin`) before `go build ./cmd/lynx` will work.

### Run

```bash
go run ./cmd/lynx                  # Run without building
./lynx :8080                # Run with defaults
# (chrome flag removed)                    # Launch with Chrome pre-configured
# (cert install removed)                # Install CA cert to system trust store
```

### Test

```bash
go test ./pkg/...                   # Run all Go tests
go test ./pkg/filter/...            # Run tests for a specific package
```

### Lint

```bash
golangci-lint run                   # Go linting (config in .golangci.yml)
cd admin && yarn run lint           # TypeScript/React linting
```

### GraphQL Code Generation

After modifying [pkg/api/schema.graphql](pkg/api/schema.graphql):

```bash
go run github.com/99designs/gqlgen generate   # Regenerates pkg/api/generated.go and models_gen.go
cd admin && yarn run codegen                   # Regenerates TypeScript types/hooks
```

---

## Architecture

### End-to-End Request Flow

```
Browser / curl / any HTTP client
         │
         ▼
gorilla/mux router  (cmd/lynx/lynx.go)
         │
         ├─ Host == admin (lynx.proxy | localhost:PORT | no absolute URI)
         │       ├─ /api/graphql/  →  GraphQL handler (gqlgen)
         │       └─ /*             →  FileServer (embedded Next.js)
         │
         └─ everything else  →  MITM Proxy handler
                  │
                  ▼
         Proxy.ServeHTTP()
           - generates ULID request ID, stores in context
           - HTTP:    ReverseProxy.Director → modifyRequest chain → upstream
           - HTTPS:   handleConnect() → hijack TCP → wrap in MITM TLS
                      → http.Serve on one-shot listener
                  │
                  ▼
         RequestModifier chain (applied in reverse registration order)
           1. intercept.RequestModifier   — may BLOCK until UI acts
           2. reqlog.RequestModifier      — logs request body
                  │
                  ▼
         Upstream server
                  │
                  ▼
         ResponseModifier chain
           1. auto-gunzip
           2. intercept.ResponseModifier  — may BLOCK if modifyResponse=true
           3. reqlog.ResponseModifier     — logs response async
```

### MITM TLS Certificate Generation (`pkg/proxy/cert.go`)

1. On startup, lynx loads or generates a self-signed CA (RSA 2048-bit).
2. When a browser sends `CONNECT host:443`, the proxy hijacks the TCP connection.
3. `CertConfig.TLSConfig()` returns a `tls.Config` with a `GetCertificate` callback.
4. On first TLS handshake the callback generates a per-host leaf certificate, signed by the CA, with the SNI hostname as CN/SAN, valid for 30 days.
5. Certificates are cached in memory keyed by hostname.
6. The browser trusts lynx's CA (installed on home page), so the leaf cert is valid.

### Request Intercept Pause Mechanism (`pkg/proxy/intercept/`)

**Blocking** (proxy goroutine):
```
InterceptService.RequestModifier() {
    if !enabled || !matchesFilter(req) { return next(req) }

    ch   := make(chan *http.Request)
    done := make(chan struct{})
    requests[reqID] = {req, ch, done}
    defer close(done); defer delete(requests, reqID)

    select {
        case modReq := <-ch:  return modReq   // ← unblocked by UI
        case <-ctx.Done():    return ctx.Err()
    }
}
```

**Unblocking** (GraphQL mutation → resolver):
```
InterceptService.ModifyRequest(reqID, modReq, modifyResponse) {
    req := requests[reqID]
    select {
        case <-req.done:   return ErrRequestDone
        case req.ch <- modReq:  return nil    // ← sends, proxy goroutine resumes
    }
}
```

**Cancellation**: sends `nil` into the channel → proxy returns `ErrRequestAborted`.

**Response interception**: if `modifyResponse=true`, the context carries a flag; `ResponseModifier` checks it and applies the same blocking pattern.

### Admin UI ↔ Backend Communication

- Single GraphQL endpoint: `POST /api/graphql/`
- Frontend (Apollo Client) polls `interceptedRequests` query every **1000 ms** (no WebSockets).
- Mutations (`modifyRequest`, `cancelRequest`, `modifyResponse`, `cancelResponse`) unblock the proxy goroutine.
- All other data (request logs, projects, scope, sender) is fetched on demand via GraphQL queries/mutations.

### Middleware Chain Registration Order

```go
// cmd/lynx/lynx.go — registration order
proxy.UseRequestModifier(reqLogService.RequestModifier)
proxy.UseResponseModifier(reqLogService.ResponseModifier)
proxy.UseRequestModifier(interceptService.RequestModifier)
proxy.UseResponseModifier(interceptService.ResponseModifier)
```

Because the chain is applied in *reverse* registration order, execution is:
- Request:  intercept → reqlog → upstream
- Response: upstream → intercept → reqlog

---

## Service Layer (`pkg/`)

| Package | Role |
|---------|------|
| `proxy` | MITM proxy core, TLS cert generation, middleware chain |
| `proxy/intercept` | Pauses requests/responses for manual editing via channels |
| `reqlog` | Logs all proxied HTTP traffic to the repository |
| `sender` | Replays/crafts arbitrary HTTP requests |
| `proj` | Project isolation: wires scope/settings across all services |
| `scope` | Regex-based URL/header/body scope rules |
| `filter` | Custom DSL lexer + recursive-descent parser for search expressions |
| `api` | GraphQL resolvers (`gqlgen`-generated), type conversions |
| `db/bolt` | BoltDB (bbolt) repository implementations via gob encoding |
| `log` | Zap logger wrapper |

### Key Design Patterns

**Repository interface pattern** — each service depends on a domain-defined `Repository` interface, not on BoltDB directly. `pkg/db/bolt` is the sole implementation. Makes the data layer swappable and testable.

**Middleware chain** — `RequestModifyMiddleware` is `func(RequestModifyFunc) RequestModifyFunc`. Registered with `proxy.UseRequestModifier()`, applied in reverse order at runtime.

**Context-based request tracking** — each request gets a ULID stored in `context.Value(reqIDKey)`. Used to correlate logs, intercept entries, and DB records across goroutines.

**Project isolation via `proj.Service`** — when a project is opened, `proj.Service` pushes scope rules, intercept settings, and search filters to all dependent services at once.

### Package Deep-Dives

#### `pkg/proxy`

`Proxy` holds a `*CertConfig`, an `http.Handler` (the reverse proxy), and two middleware slices. `ServeHTTP` assigns a ULID, detects HTTP vs HTTPS, and routes accordingly. HTTPS flows through `handleConnect`: TCP is hijacked, wrapped in a TLS server using the on-the-fly cert, then served through a one-shot `net.Listener`.

#### `pkg/proxy/intercept`

`Service` holds two maps (`requests`, `responses`) keyed by ULID, each entry containing an `*http.Request`/`*http.Response`, a send channel, and a done channel. All map access is guarded by `sync.RWMutex`. `Items()` returns a non-blocking snapshot for the GraphQL poll query.

#### `pkg/reqlog`

`RequestModifier` reads and re-buffers the request body, then calls `next(req)`, then stores the log entry. `ResponseModifier` spawns a goroutine with `context.Background()` to avoid being cancelled mid-write by the request context.

#### `pkg/proj`

When a project is opened, `proj.Service` calls into every dependent service to sync its settings (active project ID, scope rules, intercept flags, find filters). `SetScopeRules()` is the single point of truth for scope across reqlog and intercept.

#### `pkg/db/bolt`

BoltDB bucket layout:
```
projects/
  {projectID}        ← gob-encoded proj.Project
  {projectID}/request_logs/{reqLogID}       ← gob-encoded reqlog.RequestLog
  {projectID}/sender_requests/{senderReqID} ← gob-encoded sender.Request
```

All records are serialized with `encoding/gob`. `regexp.Regexp` values use custom `MarshalBinary`/`UnmarshalBinary` (serialize as pattern string).

#### `pkg/filter`

Lexer produces a token stream; the recursive-descent parser builds an AST of `Expression` nodes (`StringLiteral`, `InfixExpression`, `PrefixExpression`, `RegexpLiteral`). Operator precedence: `OR` < `AND` < `NOT` < comparison operators (`=`, `!=`, `=~`, `!~`, `<`, `>`, `<=`, `>=`). `intercept/filter.go` evaluates the AST against an `*http.Request`, mapping keys `proto`, `url`, `method`, `body`, `headers`.

#### `pkg/scope`

A `Scope` holds a `[]Rule` guarded by `sync.RWMutex`. Each `Rule` has optional `*regexp.Regexp` fields for URL, header key, header value, and body. `Match()` returns true if ANY rule matches; within a rule, any non-nil field that matches is sufficient.

#### `pkg/api`

`Resolver` struct references all services. `generated.go` and `models_gen.go` are auto-generated — never edit manually. `resolvers.go` has all hand-written resolver logic plus type conversion helpers (`parseRequestLog`, `parseResponseLog`, `parseInterceptItem`, `parseSenderRequest`, `parseProject`).

---

## Database

BoltDB (`bbolt`) at `~/.lynx/lynx.db` by default (configurable via `--db`). All persistence goes through the `Repository` interface; `pkg/db/bolt` is the sole implementation.

---

## GraphQL API

- Schema (source of truth): [pkg/api/schema.graphql](pkg/api/schema.graphql)
- Generated server code: `pkg/api/generated.go`, `pkg/api/models_gen.go` — do not edit
- Resolver implementations: [pkg/api/resolvers.go](pkg/api/resolvers.go)
- TypeScript client types/hooks: `admin/src/lib/graphql/generated.tsx` — do not edit

---

## Admin UI (`admin/`)

Next.js 12 app with TypeScript, Material-UI v5, Apollo Client, and Monaco Editor. Built with `yarn run export` to a static site, moved to `cmd/lynx/admin/`, then embedded into the Go binary at compile time.

**Context providers** (wired in `_app.tsx`):
- `ApolloProvider` — GraphQL client
- `ActiveProjectProvider` — tracks open project
- `InterceptedRequestsProvider` — 1 s polling loop for intercepted traffic

**Pages**: `/projects`, `/proxy/logs`, `/proxy/intercept`, `/sender`, `/scope`, `/settings`

---

## Key Files

| File | Purpose |
|------|---------|
| [cmd/lynx/lynx.go](cmd/lynx/lynx.go) | Server wiring, service instantiation, router setup, CLI flags |
| [pkg/api/schema.graphql](pkg/api/schema.graphql) | GraphQL schema — source of truth for API shape |
| [pkg/proxy/proxy.go](pkg/proxy/proxy.go) | MITM proxy core, middleware chain |
| [pkg/proxy/cert.go](pkg/proxy/cert.go) | On-the-fly TLS certificate generation |
| [pkg/proxy/intercept/intercept.go](pkg/proxy/intercept/intercept.go) | Channel-based request/response pause/resume |
| [pkg/db/bolt/](pkg/db/bolt/) | All BoltDB repository implementations |
| [pkg/api/resolvers.go](pkg/api/resolvers.go) | Hand-written GraphQL resolver logic |
| [gqlgen.yml](gqlgen.yml) | Code generation config for Go GraphQL server |
| [admin/gqlcodegen.yml](admin/gqlcodegen.yml) | Code generation config for TypeScript client |

---

/## Go Module

`github.com/lorenzocamilli/lynx`, requires Go 1.23. CGO is disabled (`CGO_ENABLED=0`) — the binary is fully statically linked with no C dependencies.
            
