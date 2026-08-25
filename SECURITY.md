# Security Policy

Lynx is a MITM HTTP proxy and traffic-interception toolkit for authorized
security testing. Because of what it does — decrypt and store TLS traffic,
run a local web UI with an admin API — its own security posture matters more
than a typical CLI tool's. This document describes how to report a
vulnerability and what Lynx does and does not protect against.

## Reporting a vulnerability

Please report security issues through
[GitHub Security Advisories](https://github.com/lorenzocamilli/lynx/security/advisories/new)
for this repository ("Report a vulnerability" under the Security tab). This
keeps the report private until a fix is available, and does not require an
email exchange.

Do not open a public issue for a security report.

We aim to acknowledge new reports within 5 business days. Lynx is currently
maintained by a single maintainer, so response times outside that window are
possible — a report left unacknowledged for longer is not being ignored.

## Supported versions

Lynx has not yet cut a `v1.0.0` release. Until then, only the `main` branch
receives fixes. Once versioned releases begin, security fixes will target the
latest minor release line, following semver.

## Threat model

### What Lynx trusts, and why

- **Network exposure.** The admin server (UI, GraphQL API, and proxy control
  plane) binds to `127.0.0.1` by default. This is a deliberate boundary: Lynx
  assumes a single local operator, not a multi-user or network-exposed
  service. The Settings UI lets you rebind to `0.0.0.0` and shows a warning
  when you do — doing so removes the localhost boundary and is not a
  supported deployment model.
- **Admin token.** A 32-byte random token is generated on first run and
  stored at `~/.lynx/token` (mode `0600`). It is also printed to the
  console log at startup, so it's visible without a second command — if you
  redirect Lynx's stdout to a persisted or shared log file, the token will be
  written there too; avoid doing so, or rotate the token (delete
  `~/.lynx/token` and restart) if you do. All `/api/*` routes require
  `Authorization: Bearer <token>` except `GET /api/token` and
  `GET /api/ca.crt`, which serve values that are already protected by the
  localhost bind (the token bootstrap endpoint) or are meant to be public
  (the CA certificate, not its key). `/api/events` (Server-Sent Events)
  accepts the token as a query parameter, compared with
  `crypto/subtle.ConstantTimeCompare`, since `EventSource` cannot set request
  headers.
- **CSRF.** State-changing requests (`POST`/`PUT`/`DELETE`) with a
  cross-origin `Origin` header are rejected.
- **CA private key.** On first run, Lynx generates a self-signed CA
  (`~/.lynx/lynx_key.pem`, mode `0600`, in a `0700` directory) used to mint
  per-host leaf certificates on the fly for MITM interception. **Anyone with
  read access to this key can impersonate any HTTPS site to any client that
  trusts this specific CA.** In practice that trust only extends to systems
  where you've deliberately installed Lynx's CA certificate (typically only
  your own browser/OS trust store, for your own testing) — the key is not
  useful against third parties unless they were separately tricked into
  trusting it. Treat this file like an SSH private key: back it up carefully
  if at all, and never share it.
- **Captured traffic storage.** All proxied requests/responses are stored in
  `~/.lynx/lynx.db` (BoltDB) **in the clear** by default, including headers
  and bodies. `RedactHeaders` (Settings → Application) lets you mask specific
  headers (e.g. `Authorization`, `Cookie`) before storage, but this is
  **opt-in, not the default** — the default favors capturing traffic
  faithfully for pentest/bug-bounty workflows. If you're testing targets that
  return sensitive data (tokens, PII) and don't need it retained verbatim,
  configure redaction. There is currently no at-rest encryption for the
  database file itself; treat `~/.lynx/` with the same care as the CA key.

### Out of scope

- **A fully compromised local machine.** If an attacker already has code
  execution as the same OS user running Lynx, they can read the admin token,
  the CA key, and the traffic database directly — no vulnerability in Lynx
  changes that. This is the same threat model as any local dev tool that
  stores secrets on disk.
- **Misuse against unauthorized targets.** Lynx is built for testing systems
  you are authorized to test. Using it to intercept traffic you don't have
  permission to intercept is a legal and ethical issue, not a Lynx security
  bug.

### Known accepted risk

`govulncheck` reports `GO-2024-2920`, a denial-of-service in `gqlparser`
(pulled in transitively via a pinned `gqlgen v0.14`). It is reachable only
through the admin GraphQL endpoint, which is already localhost-bound and
token-protected. It's tracked for a fix via a `gqlgen` upgrade; until then the
CI vulnerability-check job reports it non-blockingly rather than hiding it.

### Cryptographic choices

- **CA and leaf certificates use RSA-2048**, not ECDSA. This is a deliberate
  compatibility choice, not an oversight: RSA CAs remain more broadly
  accepted by older OSes, embedded devices, and some scanning/pentest
  tooling that a security researcher's target environment might include.
  Switching to ECDSA P-256 would also invalidate every already-installed
  Lynx CA certificate for existing users (a new algorithm means a new
  keypair). This will be revisited if RSA-2048 is ever deprecated as
  insufficient, but it is not currently considered weak.
- **Leaf certificate serials** are drawn from `crypto/rand` over a 160-bit
  range and are not tracked for reuse across issuances. See the comment on
  `MaxSerialNumber` in `pkg/proxy/cert.go` for the collision-probability
  reasoning.
- **SHA-1** appears once, to derive a certificate's `SubjectKeyId` per
  RFC 5280 §4.2.1.2 — a non-cryptographic identifier field, not used as a
  security primitive.
