export CGO_ENABLED = 0
export NEXT_TELEMETRY_DISABLED = 1

.PHONY: build
build: build-admin
	go build ./cmd/lynx

.PHONY: build-admin
build-admin:
	cd admin && \
	npm ci --legacy-peer-deps && \
	npm run export && \
	rm -rf ../cmd/lynx/admin && \
	mv dist ../cmd/lynx/admin

.PHONY: check
check:
	go build ./cmd/... ./pkg/...
	go test ./cmd/... ./pkg/...
	go test -race ./cmd/... ./pkg/...
	go vet ./cmd/... ./pkg/...
	cd admin && npm ci --legacy-peer-deps && npm run lint && npm run typecheck

.PHONY: clean
clean:
	rm -f lynx
	rm -rf ./cmd/lynx/admin
	rm -rf ./admin/dist
	rm -rf ./admin/.next