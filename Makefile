export CGO_ENABLED = 0
export NEXT_TELEMETRY_DISABLED = 1

.PHONY: build
build: build-admin
	go build ./cmd/lynx

.PHONY: build-admin
build-admin:
	cd admin && \
	npm install --legacy-peer-deps && \
	npm run export && \
	rm -rf ../cmd/lynx/admin && \
	mv dist ../cmd/lynx/admin

.PHONY: clean
clean:
	rm -f lynx
	rm -rf ./cmd/lynx/admin
	rm -rf ./admin/dist
	rm -rf ./admin/.next