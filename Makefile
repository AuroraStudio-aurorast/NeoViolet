BINARY  ?= neoviolet
GO      ?= go
GOOS    ?= $(shell $(GO) env GOOS)
GOARCH  ?= $(shell $(GO) env GOARCH)
OUTPUT   = $(BINARY)

ifeq ($(GOOS),windows)
	OUTPUT := $(OUTPUT).exe
endif

BUILD_FLAGS ?= -ldflags="-s -w"
TEST_FLAGS  ?= -count=1

HAS_OPENMPT := $(shell pkg-config --exists libopenmpt 2>/dev/null && echo 1 || echo 0)
ifeq ($(HAS_OPENMPT),1)
	BUILD_FLAGS += -tags openmpt
	TEST_FLAGS  += -tags openmpt
endif

HAS_OPENMPT := $(shell pkg-config --exists libopenmpt 2>/dev/null && echo 1 || echo 0)
ifeq ($(HAS_OPENMPT),1)
	BUILD_FLAGS += -tags openmpt
	TEST_FLAGS  += -tags openmpt
endif

.PHONY: all build build/race build/debug run test test/race test/verbose test/short test/cover clean lint vet tidy install help

all: build

# --- Build ---

build:
	$(GO) build $(BUILD_FLAGS) -o $(OUTPUT) ./cmd/$(BINARY)

build/race:
	$(GO) build -race $(BUILD_FLAGS) -o $(OUTPUT) ./cmd/$(BINARY)

build/debug:
	$(GO) build -gcflags="all=-N -l" -o $(OUTPUT) ./cmd/$(BINARY)

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

# --- Help ---

help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "Build:"
	@echo "  build              Build binary (default)"
	@echo "  build/race         Build with race detector"
	@echo "  build/debug        Build without optimizations (dlv compatible)"
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
