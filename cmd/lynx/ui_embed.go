//go:build !noembed

package main

import "embed"

// adminContent holds the compiled Next.js admin UI, embedded into the binary at
// build time. It is served by the file server in run(). The admin UI must be
// built first (`make build-admin`) so that ./admin exists — a plain
// `go build ./cmd/lynx` without it will fail to compile.
//
// `all:admin` includes files and directories whose names begin with "_" or "."
// (notably Next.js's `_next/`), which the default embed pattern would skip.
//
//go:embed all:admin
var adminContent embed.FS
