export CGO_ENABLED = 0
export NEXT_TELEMETRY_DISABLED = 1

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo dev)
GO_LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build
build: build-admin
	go build -ldflags "$(GO_LDFLAGS)" ./cmd/lynx

.PHONY: build-admin
build-admin:
	cd admin && \
	npm ci --legacy-peer-deps && \
	npm run export && \
	rm -rf ../cmd/lynx/admin && \
	mv dist ../cmd/lynx/admin

.PHONY: check
# noembed: build/test the Go code without the embedded admin UI (a build
# artifact), so `make check` works from a clean tree without `make build-admin`.
check:
	go build -tags noembed ./cmd/... ./pkg/...
	go test -tags noembed ./cmd/... ./pkg/...
	go test -race -tags noembed ./cmd/... ./pkg/...
	go vet -tags noembed ./cmd/... ./pkg/...
	cd admin && npm ci --legacy-peer-deps && npm run lint && npm run typecheck

.PHONY: clean
clean:
	rm -f lynx
	rm -rf ./cmd/lynx/admin
	rm -rf ./admin/dist
	rm -rf ./admin/.next