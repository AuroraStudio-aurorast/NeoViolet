BINARY  ?= neoviolet
GO      ?= go
GOOS    ?= $(shell $(GO) env GOOS)
GOARCH  ?= $(shell $(GO) env GOARCH)
OUTPUT   = $(BINARY)

ifeq ($(GOOS),windows)
	OUTPUT := $(OUTPUT).exe
endif

VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS   = -s -w -X github.com/AuroraStudio-aurorast/neoviolet/cmd/neoviolet/cmd.Version=$(VERSION)
BUILD_FLAGS ?= -ldflags="$(LDFLAGS)"
TEST_FLAGS  ?= -count=1

HAS_OPENMPT := $(shell pkg-config --exists libopenmpt 2>/dev/null && echo 1 || echo 0)
ifeq ($(HAS_OPENMPT),1)
	BUILD_FLAGS += -tags openmpt
	TEST_FLAGS  += -tags openmpt
endif

.PHONY: all build build/race build/debug build/noopenmpt run test test/race test/verbose test/short test/cover clean lint vet tidy install help

all: build

# --- Build ---

build:
	$(GO) build $(BUILD_FLAGS) -o $(OUTPUT) ./cmd/$(BINARY)

build/race:
	$(GO) build -race $(BUILD_FLAGS) -o $(OUTPUT) ./cmd/$(BINARY)

build/debug:
	$(GO) build -gcflags="all=-N -l" -o $(OUTPUT) ./cmd/$(BINARY)

build/noopenmpt:
	$(GO) build -ldflags="$(LDFLAGS)" -o $(OUTPUT) ./cmd/$(BINARY)

# --- Run ---

run: build
	./$(OUTPUT) $(ARGS)

# --- Test ---

test:
	$(GO) test $(TEST_FLAGS) ./...

test/race:
	$(GO) test -race $(TEST_FLAGS) ./...

test/verbose:
	$(GO) test -v $(TEST_FLAGS) ./...

test/short:
	$(GO) test -short $(TEST_FLAGS) ./...

test/cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

# --- Code quality ---

vet:
	$(GO) vet ./...

lint:
	golangci-lint run ./... 2>/dev/null || $(GO) vet ./...

tidy:
	$(GO) mod tidy

# --- Clean ---

clean:
	rm -f $(BINARY) $(BINARY).exe
	rm -f coverage.out coverage.html

# --- Install ---

install:
	$(GO) install $(BUILD_FLAGS) ./cmd/$(BINARY)

install/desktop:
	@mkdir -p $(DESTDIR)$(PREFIX)/share/applications
	cp neoviolet.desktop $(DESTDIR)$(PREFIX)/share/applications/
	@echo "Install desktop file to $(DESTDIR)$(PREFIX)/share/applications/"

install/icons:
	@mkdir -p $(DESTDIR)$(PREFIX)/share/icons/hicolor/scalable/apps
	@mkdir -p $(DESTDIR)$(PREFIX)/share/icons/hicolor/48x48/apps
	@echo "Place neoviolet.svg in share/icons/hicolor/scalable/apps/"
	@echo "Place neoviolet.png in share/icons/hicolor/48x48/apps/"

install/all: install install/desktop

# --- Help ---

help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "Build:"
	@echo "  build              Build binary (default, with openmpt if available)"
	@echo "  build/race         Build with race detector"
	@echo "  build/debug        Build without optimizations (dlv compatible)"
	@echo "  build/noopenmpt    Build without libopenmpt support"
	@echo ""
	@echo "Run:"
	@echo "  run ARGS=...       Build and run with optional arguments"
	@echo ""
	@echo "Test:"
	@echo "  test               Run all tests"
	@echo "  test/race          Run tests with race detector"
	@echo "  test/verbose       Run tests verbosely"
	@echo "  test/cover         Run tests with coverage report"
	@echo ""
	@echo "Code quality:"
	@echo "  vet                Run go vet"
	@echo "  lint               Run golangci-lint (fallback: go vet)"
	@echo "  tidy               Run go mod tidy"
	@echo ""
	@echo "Other:"
	@echo "  clean              Remove build artifacts"
	@echo "  install            Install binary to GOPATH/bin"
