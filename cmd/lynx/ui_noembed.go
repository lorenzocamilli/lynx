//go:build noembed

package main

import "embed"

// adminContent is an empty stub used when building or testing with the
// `noembed` build tag, so the admin UI (a build artifact) does not need to be
// compiled first. `go test`/`go vet`/CI use this to exercise the Go code
// without running `make build-admin`; the served file system simply returns
// 404s. Release builds omit the tag and embed the real UI (see ui_embed.go).
var adminContent embed.FS
