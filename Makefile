.PHONY: build test vet fmt fmt-check tidy tidy-check check clean release-tool-check release-check release-snapshot

BINARY ?= goveetl
PKG := ./...
GORELEASER_VERSION := $(shell cat .goreleaser-version)
GORELEASER := goreleaser

build:
	mkdir -p bin
	go build -trimpath -o bin/$(BINARY) ./cmd/$(BINARY)

test:
	go test -count=1 $(PKG)

vet:
	go vet $(PKG)

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

tidy:
	go mod tidy

fmt-check:
	@test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './vendor/*' -not -path './node_modules/*'))"

tidy-check:
	go mod tidy -diff

check: fmt-check tidy-check vet test build

release-tool-check:
	@$(GORELEASER) --version | grep -Eq "^GitVersion: +v?$(subst .,[.],$(GORELEASER_VERSION:v%=%))$$" || { echo "Install GoReleaser $(GORELEASER_VERSION)"; exit 1; }

release-check: release-tool-check
	$(GORELEASER) check || test "$$?" -eq 2

release-snapshot: release-tool-check
	$(GORELEASER) release --snapshot --clean

clean:
	rm -rf bin dist
