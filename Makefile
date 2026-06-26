# MCSR Ranked Bedrock — Makefile
#
# Common targets:
#   make build    — compile queen + worker binaries into ./bin/
#   make run-queen — build + run the queen service
#   make typecheck — verify all packages compile
#   make tidy     — go mod tidy
#   make clean    — remove built binaries + local data

GO ?= go
BIN := bin
LDFLAGS := -s -w \
  -X github.com/mcsr-ranked-bedrock/pkg/shared/version.Version=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev) \
  -X github.com/mcsr-ranked-bedrock/pkg/shared/version.Commit=$(shell git rev-parse --short HEAD 2>/dev/null || echo unknown) \
  -X github.com/mcsr-ranked-bedrock/pkg/shared/version.BuildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)

.PHONY: build queen worker tidy typecheck clean run-queen test

build: queen worker

queen:
	@mkdir -p $(BIN)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN)/queen ./pkg/queen

worker:
	@mkdir -p $(BIN)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN)/worker ./pkg/worker

tidy:
	$(GO) mod tidy

typecheck:
	$(GO) build ./...
	$(GO) vet ./...

test:
	$(GO) test ./...

run-queen: queen
	./$(BIN)/queen

clean:
	rm -rf $(BIN) data/
