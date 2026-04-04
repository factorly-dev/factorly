.PHONY: build test test-unit test-integration clean vet lint fix fmt tidy ci release version run init

BINARY  := factorly
OUTDIR  := build
SRCDIR  := src
VERSION := $(shell grep 'Version' $(SRCDIR)/internal/version.go | head -1 | sed 's/.*"\(.*\)".*/\1/')
LDFLAGS := -s -w

# Default: build for the host platform
build:
	@mkdir -p $(OUTDIR)
	cd $(SRCDIR) && CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o ../$(OUTDIR)/$(BINARY) ./cmd/factorly

run:
	cd $(SRCDIR) && go run ./cmd/factorly $(ARGS)

test: test-unit test-integration

test-unit:
	cd $(SRCDIR) && $(GOTESTSUM) --format testname -- ./...

test-integration: build
	cd $(SRCDIR) && $(GOTESTSUM) --format testname -- -tags integration ./test/...

vet:
	cd $(SRCDIR) && go vet ./...

tidy:
	cd $(SRCDIR) && go mod tidy

init:
	cd $(SRCDIR) && go mod download
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install gotest.tools/gotestsum@latest

GOLANGCI_LINT := $(or $(shell which golangci-lint 2>/dev/null),$(shell go env GOPATH)/bin/golangci-lint)
GOTESTSUM     := $(or $(shell which gotestsum 2>/dev/null),$(shell go env GOPATH)/bin/gotestsum)

lint:
	cd $(SRCDIR) && $(GOLANGCI_LINT) run ./...

fix:
	cd $(SRCDIR) && $(GOLANGCI_LINT) run --fix ./...
	gofmt -w $(SRCDIR)

fmt: fix

ci: tidy fmt vet lint test

clean:
	rm -rf $(OUTDIR)

# Bump patch version: 0.1.0 → 0.1.1
# Usage: make version            (bump patch)
#        make version BUMP=minor  (0.1.0 → 0.2.0)
#        make version BUMP=major  (0.1.0 → 1.0.0)
BUMP ?= patch
version:
	@major=$$(echo "$(VERSION)" | cut -d. -f1); \
	minor=$$(echo "$(VERSION)" | cut -d. -f2); \
	patch=$$(echo "$(VERSION)" | cut -d. -f3); \
	case "$(BUMP)" in \
		major) major=$$((major+1)); minor=0; patch=0;; \
		minor) minor=$$((minor+1)); patch=0;; \
		patch) patch=$$((patch+1));; \
		*) echo "Unknown BUMP=$(BUMP). Use major, minor, or patch." >&2; exit 1;; \
	esac; \
	NEW="$$major.$$minor.$$patch"; \
	sed -i "s/Version    = \"$(VERSION)\"/Version    = \"$$NEW\"/" $(SRCDIR)/internal/version.go; \
	echo "$(VERSION) → $$NEW"

# Cross-platform release builds
release: clean
	@mkdir -p $(OUTDIR)
	cd $(SRCDIR) && GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o ../$(OUTDIR)/$(BINARY)-linux-amd64 ./cmd/factorly
	cd $(SRCDIR) && GOOS=darwin  GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o ../$(OUTDIR)/$(BINARY)-darwin-amd64 ./cmd/factorly
	cd $(SRCDIR) && GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o ../$(OUTDIR)/$(BINARY)-darwin-arm64 ./cmd/factorly
	cd $(SRCDIR) && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o ../$(OUTDIR)/$(BINARY)-windows-amd64.exe ./cmd/factorly
	@echo "Built $(VERSION) binaries:"
	@ls -lh $(OUTDIR)/$(BINARY)-*
