# sshdeck — common tasks. `make help` lists them.

BINARY   := sshdeck
PKG      := ./cmd/sshdeck
BIN_DIR  := bin
DIST_DIR := dist
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X main.version=$(VERSION)
BUILDFLAGS ?= -trimpath
LINUX_ARCHES := amd64 arm64

export CGO_ENABLED ?= 0

.DEFAULT_GOAL := build
.PHONY: help build run install test race cover fmt fmt-check vet tidy tidy-check lint check \
        linux snapshot next-version clean

help: ## show this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## build bin/sshdeck for this machine
	go build $(BUILDFLAGS) -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$(BINARY) $(PKG)

run: build ## build and start sshdeck (ARGS="-sessions /tmp/s.yaml" to pass flags)
	./$(BIN_DIR)/$(BINARY) $(ARGS)

install: ## go install into $(go env GOPATH)/bin
	go install $(BUILDFLAGS) -ldflags '$(LDFLAGS)' $(PKG)

test: ## run the tests
	go test ./...

race: ## run the tests with the race detector (what CI runs)
	CGO_ENABLED=1 go test -race -count=1 ./...

cover: ## tests with a coverage report in coverage.html
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@go tool cover -func=coverage.out | tail -1

fmt: ## gofmt all files
	gofmt -w .

fmt-check: ## fail if a file is not gofmt'ed
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "not gofmt'ed:"; echo "$$out"; exit 1; fi

vet: ## go vet
	go vet ./...

tidy: ## go mod tidy
	go mod tidy

tidy-check: ## fail if go.mod / go.sum are not tidy
	go mod tidy -diff

lint: fmt-check vet tidy-check ## all static checks

check: lint race ## everything CI checks on a pull request

linux: ## static Linux binaries in dist/ (amd64, arm64)
	@mkdir -p $(DIST_DIR)
	@for arch in $(LINUX_ARCHES); do \
		echo "linux/$$arch"; \
		GOOS=linux GOARCH=$$arch go build $(BUILDFLAGS) -ldflags '$(LDFLAGS)' \
			-o $(DIST_DIR)/$(BINARY)-linux-$$arch $(PKG) || exit 1; \
	done

snapshot: ## full release build with goreleaser, without publishing
	goreleaser release --snapshot --clean

next-version: ## print the tag the next merge would create (and why)
	@bash scripts/next-version.sh -v

clean: ## remove build output
	rm -rf $(BIN_DIR) $(DIST_DIR) coverage.out coverage.html
