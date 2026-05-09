# Lynx

**Lynx** is an HTTP toolkit for security research. An open source alternative
to commercial software like Burp Suite Pro, with powerful features tailored to
the needs of the infosec and bug bounty community.

> Fork of [hetty](https://github.com/dstotijn/hetty) by David Stotijn.

## Features

- Machine-in-the-middle (MITM) HTTP proxy, with logs and advanced search
- HTTP client for manually creating/editing requests, and replay proxied requests
- Intercept requests and responses for manual review (edit, send/receive, cancel)
- Scope support, to help you stay focused on the target
- Project-based: stores data per project in a local database

## Getting started

### Prerequisites

- Go 1.23+
- Node.js 20+ (for building the admin UI)

### Build

```bash
make build
```

This builds the Next.js admin UI, then compiles the Go binary at `./lynx`.

### Run

```bash
go run ./cmd/lynx
```

Open [http://localhost:8080](http://localhost:8080) in your browser.

### Configure your browser proxy

Set your browser's HTTP/HTTPS proxy to `127.0.0.1:8080`.

### Install the CA certificate

Click **Download CA certificate** on the Lynx home page to install the root CA
into your browser or system trust store. This allows Lynx to intercept TLS
traffic.

## Data storage

All data is stored under `~/.lynx/`:

| File | Purpose |
|------|---------|
| `~/.lynx/lynx_cert.pem` | CA certificate |
| `~/.lynx/lynx_key.pem` | CA private key |
| `~/.lynx/lynx.db` | BoltDB database |
| `~/.lynx/config.yaml` | Application config |

## License

MIT
