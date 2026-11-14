BINARY  := photokit
PKG     := ./cmd/photokit
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
  -X photokit/internal/version.Version=$(VERSION) \
  -X photokit/internal/version.Commit=$(COMMIT) \
  -X photokit/internal/version.Date=$(DATE)

.DEFAULT_GOAL := help

.PHONY: help build install test test-race cover vet fmt tidy dist clean

help: ## Show this help
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-10s %s\n", $$1, $$2}'

build: ## Build ./photokit for this machine
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) $(PKG)

install: ## Install photokit into $GOBIN
	CGO_ENABLED=0 go install -trimpath -ldflags '$(LDFLAGS)' $(PKG)

test: ## Run tests (integration tests need exiftool and skip without it)
	go test ./...

test-race: ## Run tests with the race detector
	go test -race -count=1 ./...

cover: ## Run tests with a coverage report
	go test -coverprofile=coverage.txt ./... && go tool cover -func=coverage.txt | tail -n 1

vet: ## Run go vet
	go vet ./...

fmt: ## Format the code
	gofmt -w cmd internal

tidy: ## Update go.mod / go.sum
	go mod tidy

dist: ## Cross-compile all release targets into ./dist
	@mkdir -p dist
	@for t in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do \
	  os=$${t%/*}; arch=$${t#*/}; ext=""; [ $$os = windows ] && ext=".exe"; \
	  echo "building $$os/$$arch"; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags '$(LDFLAGS)' -o dist/$(BINARY)-$$os-$$arch$$ext $(PKG) || exit 1; \
	done

clean: ## Remove build output
	rm -rf $(BINARY) $(BINARY).exe dist coverage.txt
