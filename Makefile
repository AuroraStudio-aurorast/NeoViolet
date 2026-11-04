BINARY  ?= neoviolet
GO      ?= go
WINDOWS_HOST := $(filter Windows_NT,$(OS))$(filter MSYS% MINGW% CYGWIN%,$(shell uname -s))
GOOS    ?= $(if $(WINDOWS_HOST)$(findstring -windows-,$(CARGO_BUILD_TARGET)),windows,$(shell $(GO) env GOOS))
GOARCH  ?= $(shell $(GO) env GOARCH)
export GOOS GOARCH
OUTPUT   = $(BINARY)
PREFIX  ?= /usr/local

ifeq ($(GOOS),windows)
	OUTPUT := $(OUTPUT).exe
endif

VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
VERSION_LDFLAGS = -X github.com/AuroraStudio-aurorast/neoviolet/internal/version.Version=$(VERSION)
LDFLAGS   = -s -w $(VERSION_LDFLAGS)
BUILD_FLAGS ?= -ldflags="$(LDFLAGS)"
TEST_FLAGS  ?= -count=1

# Oldest macOS the universal binaries claim to support. Exported as
# MACOSX_DEPLOYMENT_TARGET for both Rust slices (unset, the Intel slice would
# record a legacy minimum) and reused by the app bundle for its plist and icon
# asset. The Go slices carry the toolchain's own default instead, so this must
# not be set below it.
MACOS_MIN ?= 13.0

HAS_CARGO := $(shell command -v cargo >/dev/null 2>&1 && echo 1 || echo 0)
RUST_TARGET = $(if $(CARGO_BUILD_TARGET),$(CARGO_BUILD_TARGET),$(shell rustc -vV | sed -n 's/^host: //p'))
RUST_TARGET_SUBDIR = $(if $(CARGO_BUILD_TARGET),$(CARGO_BUILD_TARGET)/)

APECLI_DIR := tools/apecli
APECLI_BIN := $(APECLI_DIR)/target/$(RUST_TARGET_SUBDIR)release/apecli$(shell [ "$(GOOS)" = windows ] && echo ".exe")

GUI_DIR   := tools/neoviolet-gui
GUI_OUT   := neoviolet-gui$(shell [ "$(GOOS)" = windows ] && echo ".exe")
GUI_BIN   := $(GUI_DIR)/target/$(RUST_TARGET_SUBDIR)release/neoviolet-gui$(shell [ "$(GOOS)" = windows ] && echo ".exe")
GUI_CARGO := cargo build
GUI_LINK_FLAGS :=
GO_RESOURCE_DEPS :=
GO_DEBUG_RESOURCE_DEPS :=
GO_RESOURCE_COPY :=
GUI_ICON_DEPS :=
GUI_DEBUG_ICON_DEPS :=
APECLI_CARGO := cargo build
APECLI_LINK_FLAGS :=
APECLI_RESOURCE_DEPS :=
APECLI_DEBUG_RESOURCE_DEPS :=
DEBUG_VERSION_FLAGS :=
WINDOWS_PROFILE := release
ifeq ($(GOOS),windows)
  export WINDOWS_VERSION := $(VERSION)
  DEBUG_VERSION_FLAGS = -ldflags="$(VERSION_LDFLAGS)"
  LLVM_RC ?= llvm-rc
  LLVM_CVTRES ?= llvm-cvtres
  WINDRES ?= windres
  WINDOWS_MACHINE_386 := X86
  WINDOWS_MACHINE_amd64 := X64
  WINDOWS_MACHINE_arm64 := ARM64
  WINDOWS_FORMAT_386 := pe-i386
  WINDOWS_FORMAT_amd64 := pe-x86-64
  WINDOWS_RUST_ARCH_386 := i686
  WINDOWS_RUST_ARCH_amd64 := x86_64
  WINDOWS_RUST_ARCH_arm64 := aarch64
  GO_RESOURCE_DEPS := windows-resource/tui-release
  GO_DEBUG_RESOURCE_DEPS := windows-resource/tui-debug
  GO_RESOURCE_COPY = @obj=$$(cat .build/windows/tui_$(GOARCH)_$(WINDOWS_PROFILE).path); \
    syso=cmd/$(BINARY)/icon_windows_$(GOARCH).syso; cmp -s "$$obj" "$$syso" || cp "$$obj" "$$syso"
  GUI_CARGO := cargo rustc
  APECLI_CARGO := cargo rustc
  GUI_LINK_FLAGS = -- -C "link-arg=$$(cat '$(abspath .build/windows/gui_$(GOARCH)_$(WINDOWS_PROFILE).path)')"
  APECLI_LINK_FLAGS = -- -C "link-arg=$$(cat '$(abspath .build/windows/apecli_$(GOARCH)_$(WINDOWS_PROFILE).path)')"
  GUI_ICON_DEPS := $(if $(filter 1,$(HAS_CARGO)),check/windows-rust windows-resource/gui-release)
  GUI_DEBUG_ICON_DEPS := $(if $(filter 1,$(HAS_CARGO)),check/windows-rust windows-resource/gui-debug)
  APECLI_RESOURCE_DEPS := $(if $(filter 1,$(HAS_CARGO)),check/windows-rust windows-resource/apecli-release)
  APECLI_DEBUG_RESOURCE_DEPS := $(if $(filter 1,$(HAS_CARGO)),check/windows-rust windows-resource/apecli-debug)
endif
GUI_FEATURES :=

.PHONY: all build build/tui build/race build/debug build/macos build/osxappbundle
.PHONY: run run/gui gui gui/debug gui/deps apetools apetools/debug
.PHONY: test test/go test/race test/verbose test/short test/cover test/coverage test/rust test/gui-ipc
.PHONY: lint lint/rust check check/cgo check/linelength vet tidy tidy/check clean clean/all
.PHONY: install install/desktop install/icons install/all help
.PHONY: check/windows-rust windows-resource/tui-release windows-resource/tui-debug
.PHONY: windows-resource/gui-release windows-resource/gui-debug
.PHONY: windows-resource/apecli-release windows-resource/apecli-debug

all: build

build/debug gui/debug apetools/debug: WINDOWS_PROFILE = debug

# --- Build ---

build: build/tui gui

build/tui: apetools $(GO_RESOURCE_DEPS)
	$(GO_RESOURCE_COPY)
	$(GO) build $(BUILD_FLAGS) -o $(OUTPUT) ./cmd/$(BINARY)

build/race: check/cgo $(GO_RESOURCE_DEPS)
	$(GO_RESOURCE_COPY)
	$(GO) build -race $(BUILD_FLAGS) -o $(OUTPUT) ./cmd/$(BINARY)

build/debug: $(GO_DEBUG_RESOURCE_DEPS)
	$(GO_RESOURCE_COPY)
	$(GO) build -gcflags="all=-N -l" $(DEBUG_VERSION_FLAGS) -o $(OUTPUT) ./cmd/$(BINARY)

# Universal macOS build: every slice is compiled here and merged with lipo, so a
# single Apple silicon host produces one artifact that runs on both CPUs.
build/macos: gui/deps
	@if [ "$(GOOS)" != darwin ]; then \
		echo "error: build/macos is macOS only" >&2; exit 1; \
	fi
	@if [ $(HAS_CARGO) -eq 0 ]; then \
		echo "error: build/macos needs cargo for the GUI and apecli" >&2; exit 1; \
	fi
	@if [ ! -d "$$(rustc --print target-libdir --target x86_64-apple-darwin 2>/dev/null)" ]; then \
		echo "error: build/macos needs the Intel target: rustup target add x86_64-apple-darwin" >&2; exit 1; \
	fi
	@set -eu; tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT; \
		CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build $(BUILD_FLAGS) -o "$$tmp/arm64" ./cmd/$(BINARY); \
		CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GO) build $(BUILD_FLAGS) -o "$$tmp/amd64" ./cmd/$(BINARY); \
		lipo -create "$$tmp/arm64" "$$tmp/amd64" -output $(OUTPUT)
	@set -eu; export MACOSX_DEPLOYMENT_TARGET=$(MACOS_MIN); \
		( cd $(APECLI_DIR) && cargo build --release --target aarch64-apple-darwin && cargo build --release --target x86_64-apple-darwin ); \
		lipo -create $(APECLI_DIR)/target/aarch64-apple-darwin/release/apecli \
			$(APECLI_DIR)/target/x86_64-apple-darwin/release/apecli -output apecli
	@set -eu; export MACOSX_DEPLOYMENT_TARGET=$(MACOS_MIN); \
		( cd $(GUI_DIR) && cargo build --release --target aarch64-apple-darwin $(GUI_FEATURES) && cargo build --release --target x86_64-apple-darwin $(GUI_FEATURES) ); \
		lipo -create $(GUI_DIR)/target/aarch64-apple-darwin/release/neoviolet-gui \
			$(GUI_DIR)/target/x86_64-apple-darwin/release/neoviolet-gui -output $(GUI_OUT)
	@echo "Built $(OUTPUT), $(GUI_OUT) and apecli for arm64 + amd64"

# --- GUI (Rust + Iced) ---

# Replace the output inode so macOS does not reuse a previous Mach-O signature cache.
gui: gui/deps $(GUI_ICON_DEPS)
	@set -eu; if [ $(HAS_CARGO) -eq 0 ]; then \
		echo "Warning: cargo not found, GUI will not be built"; \
	else \
		( cd $(GUI_DIR) && $(GUI_CARGO) --release $(GUI_FEATURES) $(GUI_LINK_FLAGS) ) || exit 1; \
		gui_tmp="$(GUI_OUT).tmp.$$$$"; \
		trap 'rm -f "$$gui_tmp"' EXIT; \
		cp "$(GUI_BIN)" "$$gui_tmp"; \
		mv -f "$$gui_tmp" "$(GUI_OUT)"; \
	fi

gui/debug: gui/deps $(GUI_DEBUG_ICON_DEPS)
	@if [ $(HAS_CARGO) -eq 0 ]; then \
		echo "Warning: cargo not found, GUI will not be built"; \
	else \
		( cd $(GUI_DIR) && $(GUI_CARGO) $(GUI_FEATURES) $(GUI_LINK_FLAGS) ) || exit 1; \
	fi

gui/deps:
	@if [ $(HAS_CARGO) -eq 1 ]; then GOOS= GOARCH= $(GO) run ./tools/gui-deps -root "$(CURDIR)"; fi

# --- APE toolchain (Rust) ---

apetools: $(APECLI_RESOURCE_DEPS)
	@if [ $(HAS_CARGO) -eq 0 ]; then \
		echo "Warning: cargo not found, apecli will not be built (ffmpeg/mac fallback still works)"; \
	else \
		( cd $(APECLI_DIR) && $(APECLI_CARGO) --release $(APECLI_LINK_FLAGS) ) || exit 1; \
		cp "$(APECLI_BIN)" .; \
	fi

apetools/debug: $(APECLI_DEBUG_RESOURCE_DEPS)
	@if [ $(HAS_CARGO) -eq 0 ]; then \
		echo "Warning: cargo not found, apecli will not be built"; \
	else \
		( cd $(APECLI_DIR) && $(APECLI_CARGO) $(APECLI_LINK_FLAGS) ) || exit 1; \
	fi

build/osxappbundle:
	@tools/osx-appbundle-builder/build.sh

# --- Windows executable resources ---

ifeq ($(GOOS),windows)
windows-resource/tui-release windows-resource/tui-debug \
windows-resource/gui-release windows-resource/gui-debug \
windows-resource/apecli-release windows-resource/apecli-debug:
	@if [ -z "$(WINDOWS_MACHINE_$(GOARCH))" ]; then \
		echo "error: unsupported Windows resource architecture: $(GOARCH)" >&2; exit 1; \
	fi
	@mkdir -p .build/windows
	@set -eu; tmp=$$(mktemp -d .build/windows/tmp.XXXXXX); trap 'rm -rf "$$tmp"' EXIT; \
		component=$(firstword $(subst -, ,$(@F))); profile=$(lastword $(subst -, ,$(@F))); \
		rc=$$(GOOS= GOARCH= $(GO) run ./tools/windows-resources \
			-component "$$component" -arch $(GOARCH) -debug=$(if $(filter %-debug,$@),true,false)); \
		obj=$${rc%.rc}.o; \
		if [ ! -f "$$obj" ]; then \
			if command -v "$(LLVM_RC)" >/dev/null 2>&1 && command -v "$(LLVM_CVTRES)" >/dev/null 2>&1; then \
				"$(LLVM_RC)" -no-preprocess -FO "$$tmp/resource.res" "$$rc"; \
				"$(LLVM_CVTRES)" -machine:$(WINDOWS_MACHINE_$(GOARCH)) -timestamp:0 "-out:$$tmp/resource.o" "$$tmp/resource.res"; \
			elif command -v "$(WINDRES)" >/dev/null 2>&1; then \
				if [ -z "$(WINDOWS_FORMAT_$(GOARCH))" ]; then \
					echo "error: Windows $(GOARCH) resources need llvm-rc + llvm-cvtres" >&2; exit 1; \
				fi; \
				"$(WINDRES)" -F $(WINDOWS_FORMAT_$(GOARCH)) -i "$$rc" -O coff -o "$$tmp/resource.o"; \
			else \
				echo "error: Windows resources need llvm-rc + llvm-cvtres, or windres (override LLVM_RC/LLVM_CVTRES/WINDRES)" >&2; \
				exit 1; \
			fi; \
			mv "$$tmp/resource.o" "$$obj"; \
		fi; \
		link_path="$(CURDIR)/$$obj"; \
		if command -v cygpath >/dev/null 2>&1; then link_path=$$(cygpath -m "$$link_path"); fi; \
		printf '%s\n' "$$link_path" > "$$tmp/resource.path"; \
		mv "$$tmp/resource.path" ".build/windows/$${component}_$(GOARCH)_$${profile}.path"

check/windows-rust:
	@case "$(RUST_TARGET)" in \
		$(WINDOWS_RUST_ARCH_$(GOARCH))-pc-windows-*) ;; \
		*) echo "error: Windows Rust binaries need a target matching GOARCH=$(GOARCH); set CARGO_BUILD_TARGET" >&2; exit 1 ;; \
	esac
endif

# --- Run ---

run: build/tui
	./$(OUTPUT) $(ARGS)

ifeq ($(HAS_CARGO),1)
run/gui: build
	./$(GUI_OUT) $(ARGS)
else
run/gui:
	@echo "error: run/gui needs cargo to build the GUI" >&2; exit 1
endif

# --- Test ---

test: test/go test/rust

test/go:
	$(GO) test $(TEST_FLAGS) ./...

test/race: check/cgo
	$(GO) test -race $(TEST_FLAGS) ./...

test/verbose:
	$(GO) test -v $(TEST_FLAGS) ./...

test/short:
	$(GO) test -short $(TEST_FLAGS) ./...

test/rust: gui/deps
	@if [ $(HAS_CARGO) -eq 0 ]; then \
		echo "Warning: cargo not found, skipping Rust tests"; \
	else \
		( cd $(APECLI_DIR) && cargo test ) || exit 1; \
		( cd $(GUI_DIR) && cargo test ) || exit 1; \
		( cd $(GUI_DIR) && cargo test -p iced_wgpu --lib transparency_tests ) || exit 1; \
	fi

test/gui-ipc: build/tui gui/deps
	@if [ $(HAS_CARGO) -eq 0 ]; then echo "error: test/gui-ipc needs cargo" >&2; exit 1; fi
	cd $(GUI_DIR) && NEOVIOLET_TEST_PLAYER="$(abspath $(OUTPUT))" \
		cargo test live_tui_acknowledges_and_exits_on_gui_request -- --ignored

lint/rust: gui/deps
	@if [ $(HAS_CARGO) -eq 0 ]; then \
		echo "Warning: cargo not found, skipping Rust lint"; \
	else \
		( cd $(APECLI_DIR) && cargo fmt --check && cargo clippy --all-targets -- -D warnings ) || exit 1; \
		( cd $(GUI_DIR) && cargo fmt --check && cargo clippy --all-targets -- -D warnings ) || exit 1; \
	fi

test/cover:
	$(GO) test -coverprofile=coverage.out $(TEST_FLAGS) ./...
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
		echo "WARNING: golangci-lint not found, falling back to go vet + gofmt"; \
		$(MAKE) vet || exit 1; \
		unformatted="$$(find . -type d \( -name .git -o -name .worktrees -o -name target \) -prune \
			-o -type f -name '*.go' -exec gofmt -l {} +)" || exit 1; \
		if [ -n "$$unformatted" ]; then \
			echo "gofmt: these files are not formatted:"; \
			echo "$$unformatted"; \
			exit 1; \
		fi; \
	fi

# Local aggregate of the code quality gates CI runs individually.
check: vet lint lint/rust test/rust test/race test/coverage tidy/check check/linelength
	@echo "All checks passed."

check/cgo:
	@if [ "$$($(GO) env CGO_ENABLED)" != 1 ]; then \
		echo "error: race targets need CGO_ENABLED=1 and a working C compiler" >&2; exit 1; \
	fi
	@race_cc="$$($(GO) env CC)" && $$race_cc --version >/dev/null 2>&1 || { \
		echo "error: race targets need a working CC (see go env CC)" >&2; exit 1; \
	}

check/linelength:
	@fail=0; \
	for f in $$(find . -type d \( -name .git -o -name .worktrees -o -name target \) -prune \
		-o -type f -name "*.go" ! -name "*_test.go" -print); do \
		lines=$$(wc -l < "$$f"); \
		if [ $$lines -gt 500 ]; then echo "$$f: $$lines lines (>500)"; fail=1; fi; \
	done; \
	for f in $$(find . -type d \( -name .git -o -name .worktrees -o -name target \) -prune \
		-o -type f -name "*_test.go" -print); do \
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
	rm -f $(BINARY) $(BINARY).exe apecli apecli.exe neoviolet-gui neoviolet-gui.exe $(GUI_OUT)
	rm -f coverage.out coverage.html
	rm -f cmd/$(BINARY)/icon_windows_*.syso
	rm -rf .build/windows

clean/all: clean
	@if [ $(HAS_CARGO) -eq 0 ]; then \
		echo "Warning: cargo not found, skipping Rust cache cleanup"; \
	else \
		( cd $(APECLI_DIR) && cargo clean ) || exit 1; \
		( cd $(GUI_DIR) && cargo clean ) || exit 1; \
	fi

# --- Install ---

install: $(GO_RESOURCE_DEPS)
	$(GO_RESOURCE_COPY)
	$(GO) install $(BUILD_FLAGS) ./cmd/$(BINARY)

install/desktop:
	@install -d "$(DESTDIR)$(PREFIX)/share/applications"
	install -m 644 neoviolet.desktop "$(DESTDIR)$(PREFIX)/share/applications/"
	@echo "Install desktop file to $(DESTDIR)$(PREFIX)/share/applications/"

install/icons:
	@install -d "$(DESTDIR)$(PREFIX)/share/icons/hicolor/scalable/apps"
	@install -d "$(DESTDIR)$(PREFIX)/share/pixmaps"
	install -m 644 assets/icons/neoviolet.svg "$(DESTDIR)$(PREFIX)/share/icons/hicolor/scalable/apps/"
	install -m 644 assets/icons/neoviolet.png "$(DESTDIR)$(PREFIX)/share/pixmaps/"

install/all: install install/desktop install/icons

# --- Help ---

help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "Build:"
	@echo "  build              Build TUI, apecli and GUI (default; Rust components optional)"
	@echo "  build/tui          Build TUI and optional apecli, without GUI"
	@echo "  build/race         Build with race detector"
	@echo "  build/debug        Build without optimizations (dlv compatible)"
	@echo "  build/macos        Build universal macOS binaries (arm64 + amd64)"
	@echo ""
	@echo "APE (Monkey's Audio):"
	@echo "  apetools           Build apecli Rust helper (cargo required)"
	@echo "  apetools/debug     Build apecli in debug mode"
	@echo ""
	@echo "macOS App Bundle:"
	@echo "  build/osxappbundle    Build macOS .app bundle (macOS only)"
	@echo ""
	@echo "Run:"
	@echo "  run ARGS=...       Build TUI and optional apecli, then run TUI"
	@echo ""
	@echo "Test:"
	@echo "  test               Run Go and Rust tests (Rust skipped without cargo)"
	@echo "  test/go            Run Go tests only"
	@echo "  test/race          Run Go tests with race detector (requires cgo and CC)"
	@echo "  test/verbose       Run Go tests verbosely"
	@echo "  test/short         Run short Go tests (skips integration)"
	@echo "  test/cover         Run Go tests with coverage report"
	@echo "  test/coverage      Run Go tests and enforce the COVERAGE_MIN gate"
	@echo "  test/rust          Run Rust unit tests (GUI + apecli)"
	@echo ""
	@echo "Code quality:"
	@echo "  vet                Run go vet"
	@echo "  lint               Run golangci-lint (fallback: make vet + gofmt)"
	@echo "  lint/rust          Run cargo fmt --check + clippy -D warnings"
	@echo "  check              Run lint, vet, Rust tests, race, coverage, tidy and size gates"
	@echo "  check/cgo          Check cgo and CC prerequisites for race targets"
	@echo "  check/linelength   Enforce the per-file line limits"
	@echo "  tidy               Run go mod tidy"
	@echo "  tidy/check         Read-only go mod tidy -diff (what CI runs)"
	@echo ""
	@echo "GUI (Iced + iced_term):"
	@echo "  gui                Build neoviolet-gui (cargo required)"
	@echo "  gui/deps           Prepare checksum-pinned GUI sources and local patches"
	@echo "  test/gui-ipc       Verify a real TUI handshake and graceful quit without a GUI"
	@echo "  gui/debug          Build neoviolet-gui in debug mode"
	@echo "  run/gui ARGS=...   Build TUI, apecli and GUI, then run GUI (cargo required)"
	@echo ""
	@echo "Other:"
	@echo "  clean              Remove root binaries and coverage reports; keep Rust cache"
	@echo "  clean/all          Also remove Rust compilation caches (when cargo is available)"
	@echo "  install            Install binary to GOPATH/bin"
	@echo "  install/desktop    Install the .desktop entry"
	@echo "  install/icons      Install Linux SVG and PNG icons"
	@echo "  install/all        install + install/desktop + install/icons"
	@echo "  help               Show this message"
