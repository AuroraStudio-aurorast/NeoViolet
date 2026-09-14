BINARY  ?= neoviolet
GO      ?= go
GOOS    ?= $(shell $(GO) env GOOS)
GOARCH  ?= $(shell $(GO) env GOARCH)
OUTPUT   = $(BINARY)

ifeq ($(GOOS),windows)
	OUTPUT := $(OUTPUT).exe
endif

VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS   = -s -w -X github.com/AuroraStudio-aurorast/neoviolet/internal/version.Version=$(VERSION)
BUILD_FLAGS ?= -ldflags="$(LDFLAGS)"
TEST_FLAGS  ?= -count=1

HAS_OPENMPT := $(shell pkg-config --exists libopenmpt 2>/dev/null && echo 1 || echo 0)
ifeq ($(HAS_OPENMPT),1)
	BUILD_FLAGS += -tags openmpt
	TEST_FLAGS  += -tags openmpt
endif

HAS_CARGO := $(shell which cargo 2>/dev/null | grep -q . && echo 1 || echo 0)

APECLI_DIR := tools/apecli
APECLI_BIN := $(APECLI_DIR)/target/release/apecli$(shell [ "$(GOOS)" = windows ] && echo ".exe")

GUI_DIR   := tools/neoviolet-gui
GUI_OUT   := neoviolet-gui$(shell [ "$(GOOS)" = windows ] && echo ".exe")
GUI_BIN   := $(GUI_DIR)/target/release/neoviolet-gui$(shell [ "$(GOOS)" = windows ] && echo ".exe")
# Use runtime_shaders on macOS to avoid Metal Toolchain requirement.
# Shaders compile at runtime via system Metal framework (no xcrun metal needed).
GUI_FEATURES :=
ifeq ($(GOOS),darwin)
  GUI_FEATURES := --no-default-features -F gpui/runtime_shaders
endif

.PHONY: all build build/race build/debug build/noopenmpt build/osxappbundle run test test/race test/verbose test/short test/cover test/rust lint lint/rust check check/linelength clean vet tidy install apetools apetools/debug gui gui/debug run/gui help

all: build

# --- Build ---

build: apetools gui
	$(GO) build $(BUILD_FLAGS) -o $(OUTPUT) ./cmd/$(BINARY)

build/race:
	$(GO) build -race $(BUILD_FLAGS) -o $(OUTPUT) ./cmd/$(BINARY)

build/debug:
	$(GO) build -gcflags="all=-N -l" -o $(OUTPUT) ./cmd/$(BINARY)

build/noopenmpt:
	$(GO) build -ldflags="$(LDFLAGS)" -o $(OUTPUT) ./cmd/$(BINARY)

# --- GUI (Rust + gpui-ce) ---

gui:
	@if [ $(HAS_CARGO) -eq 0 ]; then echo "Warning: cargo not found, GUI will not be built"; exit 0; fi
	cd $(GUI_DIR) && cargo build --release $(GUI_FEATURES)
	@cp $(GUI_BIN) . 2>/dev/null || true

gui/debug:
	@if [ $(HAS_CARGO) -eq 0 ]; then echo "Warning: cargo not found, GUI will not be built"; exit 0; fi
	cd $(GUI_DIR) && cargo build $(GUI_FEATURES)

# --- APE toolchain (Rust) ---

apetools:
	@if [ $(HAS_CARGO) -eq 0 ]; then echo "Warning: cargo not found, apecli will not be built (ffmpeg/mac fallback still works)"; exit 0; fi
	cd $(APECLI_DIR) && cargo build --release
	@cp $(APECLI_BIN) . 2>/dev/null || true

apetools/debug:
	@if [ $(HAS_CARGO) -eq 0 ]; then echo "Warning: cargo not found, apecli will not be built"; exit 0; fi
	cd $(APECLI_DIR) && cargo build

build/osxappbundle:
	@tools/osx-appbundle-builder/build.sh

# --- Run ---

run: build
	./$(OUTPUT) $(ARGS)

run/gui: gui
	./$(GUI_OUT) $(ARGS)

# --- Test ---

test: test/rust
	$(GO) test $(TEST_FLAGS) ./...

test/race:
	$(GO) test -race $(TEST_FLAGS) ./...

test/verbose:
	$(GO) test -v $(TEST_FLAGS) ./...

test/short:
	$(GO) test -short $(TEST_FLAGS) ./...

test/rust:
	@if [ $(HAS_CARGO) -eq 0 ]; then echo "Warning: cargo not found, skipping Rust tests"; exit 0; fi
	cd $(APECLI_DIR) && cargo test
	cd $(GUI_DIR) && cargo test

lint/rust:
	@if [ $(HAS_CARGO) -eq 0 ]; then echo "Warning: cargo not found, skipping Rust lint"; exit 0; fi
	cd $(APECLI_DIR) && cargo fmt --check && cargo clippy --all-targets -- -D warnings
	cd $(GUI_DIR) && cargo fmt --check && cargo clippy --all-targets -- -D warnings

test/cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

# Lower bound on total coverage. CI runs this same target, so raising it here is
# enough to tighten the gate everywhere.
COVERAGE_MIN ?= 50

test/coverage:
	$(GO) test -coverprofile=coverage.out $(TEST_FLAGS) ./...
	@total=$$($(GO) tool cover -func=coverage.out | tail -1 | awk '{print $$3}' | tr -d '%'); \
	echo "total coverage: $${total}%"; \
	if [ "$$(awk -v t="$${total}" -v m="$(COVERAGE_MIN)" 'BEGIN { print (t < m) ? 1 : 0 }')" = "1" ]; then \
		echo "coverage $${total}% is below the $(COVERAGE_MIN)% gate"; exit 1; \
	fi

# --- Code quality ---

vet:
	@# stdmethods is relaxed for internal/mediactl only: its MPRIS D-Bus methods
	@# must keep the names and signatures the spec defines (Seek, OpenUri), which
	@# govet's io.Seeker heuristic misreads as a bug. `make lint` still enforces it
	@# for the rest of the tree through golangci-lint's govet, which honours nolint.
	@pkgs="$$($(GO) list ./... | grep -v '/internal/mediactl$$')" && $(GO) vet $$pkgs
	$(GO) vet -stdmethods=false ./internal/mediactl/...

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "WARNING: golangci-lint not found, falling back to go vet"; \
		$(GO) vet ./...; \
	fi

# Aggregate quality gate used by CI and local dev.
check: vet lint lint/rust test check/linelength
	@echo "All checks passed."

check/linelength:
	@fail=0; \
	for f in $$(find . -type f -name "*.go" -print | grep -v '_test\.go$$'); do \
		lines=$$(wc -l < "$$f"); \
		if [ $$lines -gt 500 ]; then echo "$$f: $$lines lines (>500)"; fail=1; fi; \
	done; \
	for f in $$(find . -type f -name "*_test.go" -print); do \
		lines=$$(wc -l < "$$f"); \
		if [ $$lines -gt 800 ]; then echo "$$f: $$lines lines (>800)"; fail=1; fi; \
	done; \
	if [ $$fail -ne 0 ]; then exit 1; fi; \
	echo "Line length check passed."

tidy:
	$(GO) mod tidy

# Read-only counterpart of `make tidy` (go mod tidy -diff exits non-zero when
# go.mod/go.sum need changes). CI runs this instead of mutating the checkout.
tidy/check:
	$(GO) mod tidy -diff

# --- Clean ---

clean:
	rm -f $(BINARY) $(BINARY).exe apecli apecli.exe $(GUI_OUT)
	rm -f coverage.out coverage.html
	-cd $(APECLI_DIR) && cargo clean 2>/dev/null || true
	-cd $(GUI_DIR) && cargo clean 2>/dev/null || true

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
	@echo "APE (Monkey's Audio):"
	@echo "  apetools           Build apecli Rust helper (cargo required)"
	@echo "  apetools/debug     Build apecli in debug mode"
	@echo ""
	@echo "macOS App Bundle:"
	@echo "  build/osxappbundle    Build macOS .app bundle (macOS only)"
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
	@echo "  check              Run vet + lint + Rust checks + tests (CI gate)"
	@echo "  test/rust          Run Rust unit tests (GUI + apecli)"
	@echo "  lint/rust          Run cargo fmt --check + clippy -D warnings"
	@echo ""
	@echo "GUI (gpui-ce + yororen-ui):"
	@echo "  gui                Build neoviolet-gui (cargo required)"
	@echo "  gui/debug          Build neoviolet-gui in debug mode"
	@echo "  run/gui ARGS=...   Build GUI and run with optional arguments"
	@echo ""
	@echo "Other:"
	@echo "  clean              Remove build artifacts"
	@echo "  install            Install binary to GOPATH/bin"
