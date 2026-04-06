BINARY ?= framescli
OUT_DIR ?= bin

.PHONY: build deps deps-whisper smoke-public test test-integration smoke preflight fmt tidy run verify release-snapshot

build:
	mkdir -p $(OUT_DIR)
	go build -o $(OUT_DIR)/$(BINARY) ./cmd/frames

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
	go run ./cmd/frames tui --help

preflight:
	./scripts/preflight.sh

fmt:
	gofmt -w ./cmd ./internal

tidy:
	go mod tidy

run:
	go run ./cmd/frames

verify: tidy fmt test build

release-snapshot:
	goreleaser release --snapshot --clean
