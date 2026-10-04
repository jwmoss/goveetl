.PHONY: build test vet fmt tidy check clean release-snapshot

BINARY ?= goveetl
PKG := ./...

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

check: fmt tidy vet test build
	@if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then git diff --exit-code -- go.mod go.sum; fi

release-snapshot:
	goreleaser release --snapshot --clean

clean:
	rm -rf bin dist
