GO ?= go
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --verify HEAD 2>/dev/null || echo unknown)

.PHONY: fmt-check vet test race build

fmt-check:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then printf '%s\n' "$$out"; exit 1; fi

vet:
	$(GO) vet ./...

test:
	$(GO) test -count=1 ./...

race:
	$(GO) test -race -count=1 ./...

build:
	mkdir -p bin
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)" -o bin/textstats .
