MODULE   := github.com/geoffmcc/nodex
BIN      := nodex
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILT    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GOVERSION ?= $(shell go version | cut -d' ' -f3)

GO       := go
GOFMT    := gofmt
LDFLAGS  := -X '$(MODULE)/internal/version.Version=$(VERSION)' \
            -X '$(MODULE)/internal/version.Commit=$(COMMIT)' \
            -X '$(MODULE)/internal/version.BuildDate=$(BUILT)' \
            -X '$(MODULE)/internal/version.GoVersion=$(GOVERSION)'

.PHONY: build test vet fmt lint staticcheck clean

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/nodex/

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

fmt:
	$(GOFMT) -s -w .

fmt-check:
	@echo "Checking gofmt..."
	@diff=$$($(GOFMT) -s -d .); \
	if [ -n "$$diff" ]; then \
		echo "$$diff"; \
		exit 1; \
	fi

lint: fmt-check vet
	@echo "All lint checks passed."

staticcheck:
	staticcheck ./...

clean:
	rm -f $(BIN)

.DEFAULT_GOAL := build
