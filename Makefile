# Build, test and release the photos CLI. No toolchain beyond Go.

BINARY  := photos
# The version is stamped in, not committed: the tag without its "v", or
# "dev" outside a tagged checkout.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//')
ifeq ($(strip $(VERSION)),)
VERSION := dev
endif
LDFLAGS := -s -w -X main.version=$(VERSION)
# -buildvcs=false: the version comes from VERSION above, so the build does
# not need (and does not fail without) a git checkout around it.
GOBUILD := go build -trimpath -buildvcs=false -ldflags "$(LDFLAGS)"

# The platforms a release is built for.
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

.PHONY: build test vet fmt check usage dist install clean

build:
	$(GOBUILD) -o dist/$(BINARY) ./cmd/$(BINARY)

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

# What CI runs. gofmt is checked, not applied: a formatting fix belongs in
# the commit that caused it.
check:
	@test -z "$$(gofmt -l .)" || { echo "gofmt:"; gofmt -l .; exit 1; }
	go vet ./...
	go test -race ./...

# USAGE.md is generated from the command tree; `make check` fails when it is
# stale.
usage:
	go test ./internal/cli -run TestUsageIsInSync -update

# One static binary per platform, named photos-<os>-<arch>[.exe], and
# checksums.txt beside them, which installer.sh verifies a download against.
dist: check
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		out=dist/$(BINARY)-$$os-$$arch; \
		if [ "$$os" = windows ]; then out=$$out.exe; fi; \
		echo "$$out"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GOBUILD) -o $$out ./cmd/$(BINARY) || exit 1; \
	done
	@cd dist && { sha256sum $(BINARY)-* 2>/dev/null || shasum -a 256 $(BINARY)-*; } > checksums.txt
	@echo "wrote dist/checksums.txt ($(VERSION))"

install:
	go install -trimpath -buildvcs=false -ldflags "$(LDFLAGS)" ./cmd/$(BINARY)

# Only what this Makefile makes: the binaries and their checksums.
clean:
	rm -f dist/$(BINARY) dist/$(BINARY)-* dist/checksums.txt
