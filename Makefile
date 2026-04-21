BINARY ?= framescli
OUT_DIR ?= bin

# Version info injected into the binary so `framescli --version` reports
# something meaningful for source builds. goreleaser injects these at release
# time via its own ldflags; without these, `make build` reports "framescli dev"
# with no commit reference, which complicates bug reports from source builds.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS ?= -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build deps deps-whisper smoke-public test test-integration smoke preflight qa-pass generate-schemas fmt tidy run verify release-snapshot release-verify

build:
	mkdir -p $(OUT_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(OUT_DIR)/$(BINARY) ./cmd/frames

deps:
	./scripts/install-deps.sh --install

deps-whisper:
	./scripts/install-deps.sh --install --with-whisper

smoke-public:
	./scripts/public-smoke.sh

test:
	go test ./...

test-integration:
	go test -tags=integration ./internal/media

smoke:
	go run ./cmd/frames --help
	go run ./cmd/frames doctor --help
	go run ./cmd/frames benchmark --help
	go run ./cmd/frames clean --help

preflight:
	./scripts/preflight.sh

qa-pass:
	./scripts/qa-pass.sh

generate-schemas:
	./scripts/generate-schemas.sh

fmt:
	gofmt -w ./cmd ./internal

tidy:
	go mod tidy

run:
	go run ./cmd/frames

verify: tidy fmt test build

release-snapshot:
	goreleaser release --snapshot --clean

release-verify:
	./scripts/release-verify.sh --source dist --dist-dir ./dist
