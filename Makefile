.PHONY: build test test-unit test-integration test-coverage clean vet lint fix fmt tidy ci release version check-version run init license changelog

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
	cd $(SRCDIR) && $(GOTESTSUM) --format testname -- -count=1 -cover ./...

test-coverage:
	@mkdir -p $(OUTDIR)
	cd $(SRCDIR) && go test -coverprofile=../$(OUTDIR)/coverage.out ./...
	cd $(SRCDIR) && go tool cover -func=../$(OUTDIR)/coverage.out | tail -1
	@echo "Full report: go tool cover -html=$(OUTDIR)/coverage.out"

test-integration: build
	cd $(SRCDIR) && $(GOTESTSUM) --format testname -- -count=1 -tags integration ./test/...

vet:
	cd $(SRCDIR) && go vet ./...

tidy:
	cd $(SRCDIR) && go mod tidy

init:
	cd $(SRCDIR) && go mod download
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install gotest.tools/gotestsum@latest
	go install github.com/google/addlicense@latest

GOLANGCI_LINT := $(or $(shell which golangci-lint 2>/dev/null),$(shell go env GOPATH)/bin/golangci-lint)
GOTESTSUM     := $(or $(shell which gotestsum 2>/dev/null),$(shell go env GOPATH)/bin/gotestsum)
ADDLICENSE    := $(or $(shell which addlicense 2>/dev/null),$(shell go env GOPATH)/bin/addlicense)

lint:
	cd $(SRCDIR) && $(GOLANGCI_LINT) run ./...

fix:
	cd $(SRCDIR) && $(GOLANGCI_LINT) run --fix ./...
	gofmt -w $(SRCDIR)

fmt: fix

check-version:
	@GO_VERSION=$(VERSION); \
	NPM_VERSION=$$(node -p "require('./npm/package.json').version" 2>/dev/null || echo "missing"); \
	PIP_VERSION=$$(grep 'version = ' pip/pyproject.toml 2>/dev/null | head -1 | sed 's/.*"\(.*\)".*/\1/' || echo "missing"); \
	if [ "$$GO_VERSION" != "$$NPM_VERSION" ]; then \
		echo "Version mismatch: Go=$$GO_VERSION npm=$$NPM_VERSION" >&2; \
		exit 1; \
	fi; \
	if [ "$$GO_VERSION" != "$$PIP_VERSION" ]; then \
		echo "Version mismatch: Go=$$GO_VERSION pip=$$PIP_VERSION" >&2; \
		exit 1; \
	fi; \
	echo "Versions aligned: $$GO_VERSION"

ci: tidy fmt vet lint check-version test

clean:
	rm -rf $(OUTDIR)

# Add GPL license headers to source files
license:
	$(ADDLICENSE) -c "Jordan Sherer <hi@jordansherer.com>" -l gpl -s -v -ignore 'vendor/**' -ignore 'node_modules/**' -ignore '**/*.yaml' -ignore '**/*.yml' -ignore '**/*.json' -ignore '**/*.md' -ignore '**/*.toml' .

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
	echo "Bump $(VERSION) → $$NEW?"; \
	printf "[y/N] "; \
	read ans; \
	case "$$ans" in [yY]*) ;; *) echo "Aborted."; exit 1;; esac; \
	sed -i "s/\"$(VERSION)\"/\"$$NEW\"/" $(SRCDIR)/internal/version.go; \
	sed -i "s/\"version\": \"$(VERSION)\"/\"version\": \"$$NEW\"/" npm/package.json; \
	sed -i "s/version = \"$(VERSION)\"/version = \"$$NEW\"/" pip/pyproject.toml; \
	sed -i "s/__version__ = \"$(VERSION)\"/__version__ = \"$$NEW\"/" pip/factorly/__init__.py; \
	sed -i "s/Release-v$(VERSION)-/Release-v$$NEW-/" README.md; \
	sed -i "s/Release-v$(VERSION)-/Release-v$$NEW-/" npm/README.md; \
	sed -i "s/Release-v$(VERSION)-/Release-v$$NEW-/" pip/README.md; \
	echo "$(VERSION) → $$NEW"; \
	git add $(SRCDIR)/internal/version.go npm/package.json pip/pyproject.toml pip/factorly/__init__.py README.md npm/README.md pip/README.md; \
	git commit -m "Bump version to $$NEW"; \
	git tag "v$$NEW"; \
	echo "Tagged v$$NEW"

# Generate changelog from commit messages in a range
# Usage: make changelog RANGE=v0.5.5..v0.6.0
#        make changelog RANGE=v0.6.0..HEAD
RANGE ?=
changelog:
	@RANGE="$(RANGE)"; \
	if [ -z "$$RANGE" ]; then \
		printf "Range (e.g. v0.5.5..v0.6.0): "; \
		read RANGE; \
		if [ -z "$$RANGE" ]; then echo "Aborted."; exit 1; fi; \
	fi; \
	git log $$RANGE --no-merges --format="%s%n%b%n---%n" | sed '/^Bump version/,/^---$$/d' | sed 's/^---$$/\n---/'

# Cross-platform release builds
release: clean build
	@mkdir -p $(OUTDIR)
	cd $(SRCDIR) && GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o ../$(OUTDIR)/$(BINARY)-$(VERSION)-linux-amd64 ./cmd/factorly
	cd $(SRCDIR) && GOOS=darwin  GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o ../$(OUTDIR)/$(BINARY)-$(VERSION)-darwin-amd64 ./cmd/factorly
	cd $(SRCDIR) && GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o ../$(OUTDIR)/$(BINARY)-$(VERSION)-darwin-arm64 ./cmd/factorly
	cd $(SRCDIR) && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o ../$(OUTDIR)/$(BINARY)-$(VERSION)-windows-amd64.exe ./cmd/factorly
	@echo "Built $(VERSION) binaries:"
	@ls -lh $(OUTDIR)/$(BINARY)-*
