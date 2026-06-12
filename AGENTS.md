# AGENTS.md

## Build & Test

All operations go through `make` — never call `go build`/`go test` directly:

```bash
# Build (also builds apecli via make apetools dependency)
make build               # → ./neoviolet (auto-detects libopenmpt)
make build/race          # Race detector
make build/debug         # Debug symbols (dlv-compatible)

# APE toolchain (built automatically by `make build`)
make apetools            # cargo build --release in tools/apecli/
make apetools/debug      # cargo build (debug)

# Run / Install
make run ARGS="<file>"   # Build + run with optional args
make install             # → $GOPATH/bin
./neoviolet              # Start player

# Test
make test                # All tests (incl. APE if apecli present)
make test/verbose        # -v
make test/race           # Race detector
make test/cover          # Coverage report → coverage.html
make test/short          # Skip integration tests

# Code quality
make vet                 # go vet
make lint                # golangci-lint (falls back to go vet)
make tidy                # go mod tidy
make clean               # Go + apecli artifacts
```

## Prerequisites

- **Go 1.26+** — [Download](https://go.dev/dl/). Windows: use MSYS2 build.
- **Rust + cargo** — builds `apecli` (APE decoder). Without it, `make build` warns and continues; APE falls back to ffmpeg/macOS `afconvert`. Install via [rustup](https://rustup.rs/).
- **C compiler** — CGo links against C libs. `CGO_ENABLED=1` must be set; `CC=clang` recommended.
- **`make`** — entry point for all targets.
- **`pkg-config`** — auto-detects `libopenmpt`.
- **`libopenmpt`** (optional) — enhances tracker module playback. Without it, `gotracker/playback` covers most formats (MOD/XM/IT/S3M) but not `.mptm`.

**For AI agents:** Always verify `CGO_ENABLED=1` and `CC` before building — CGo is non-negotiable. If `libopenmpt` is needed but missing, ask the user before installing.

## APE (Monkey's Audio) Toolchain

Uses a Rust helper (`apecli`) built by the `apetools` Makefile target. `make build` depends on it.

### Flow

1. `make build` → `apetools` → checks for `cargo` (warns + exits 0 if missing) → `cargo build --release` in `tools/apecli/` → copies `apecli` binary to repo root
2. Go decoder (`internal/audio/format/format_ape.go`) probes for `apecli` at runtime (binary dir → $PATH → common locations)
3. If `apecli` unavailable at runtime: falls back to ffmpeg → `afconvert` → error

### Targets

| Target | Action | Use case |
|---|---|---|
| `make build` | `apetools` → `go build` | Every production build |
| `make apetools` | `cargo build --release` + copy | Rebuild after Rust changes |
| `make apetools/debug` | `cargo build` (debug) | Debugging Rust decoder |
| `make clean` | rm `apecli` + `cargo clean` | Full clean |

### Layout

```
tools/apecli/
  Cargo.toml     # ape-decoder crate, clap CLI
  src/main.rs    # stdin → decode → raw PCM stdout
```

**For AI agents:** After changing Rust source, run `make apetools` before `make test` — `make test` does NOT rebuild `apecli`.

## Project Overview

Terminal music player (Bubble Tea TUI) in Go 1.26+. Audio playback, album art, lyrics syncing, media controls.

## Architecture

```
cmd/neoviolet/          # Entry point: init logging → load config → launch TUI
internal/
  audio/                # Playback engine + format decoders + MIDI/tracker synths
    format/             # Per-format: alac/, mp4/, alacstream/, opusstream/, plus individual .go files
    synth/              # MIDI SoundFont synthesis
  config/               # JSON config with defaults, first-run detection
  cover/                # Album art from audio tags
  lyrics/               # Pluggable parser registry: lrc/ttml/qrc/yrc/eslrc/lys/embedded
  ui/                   # Bubble Tea model.go/types.go/update.go/update_keyboard.go/view.go/audio_state.go
  mediactl/             # OS media control: Linux (MPRIS/D-Bus) or no-op stub
  accent/               # K-means color extraction from cover art
  logger/               # Structured logging
docs/                   # Documentation
testdata/               # Test fixtures (*.mp3, *.flac, *.m4a, *.lrc, ...)
.github/workflows/      # CI: multi-platform (linux/mac/windows × amd64/arm64)
```

### Design Decisions

- **Elm architecture:** Model → Update (`tea.Msg`) → View. Mutate state only in `Update()`.
- **Pluggable lyrics:** `LyricParser` interface + `RegisterParser()` — prioritized by registration order. TTML/LYS support overlapping multi-agent rendering.
- **Platform build tags:** Media control via `controller_linux.go` (MPRIS/D-Bus) vs `controller_stub.go`. Add `controller_<os>.go` for new platforms.
- **Config-first startup:** First run → setup wizard → persist JSON config next to binary.
- **Cross-platform audio:** `gopxl/beep` core loop. Format decoders for WAV/MP3/FLAC/OGG/Opus/ALAC/APE. MIDI needs `.sf2`. Trackers via `gotracker/playback` + optional `libopenmpt`.

## Common Tasks

| Task | Where |
|---|---|
| Add lyrics format | New file in `internal/lyrics/`, implement `LyricParser`, `RegisterParser()` in `init()` |
| Add audio format | Decoder in `internal/audio/format/`, magic-byte detection in `Decode()` switch in `internal/audio/format/decoder.go`, add extension to `SupportedFormats()` |
| Change shortcuts | `KeyMap` in `internal/ui/types.go` + handler in `internal/ui/update_keyboard.go` |
| Tweak UI | Lipgloss styles in `internal/ui/view.go` and `internal/ui/types.go` |

## Dependencies

| Dependency | Usage |
|---|---|
| charm.land/bubbletea/v2 | TUI framework |
| charm.land/lipgloss/v2 | Terminal styling |
| github.com/gopxl/beep/v2 | Audio playback loop |
| github.com/dhowden/tag | Metadata / cover art |
| github.com/jpodeszfa/go-meltysynth | MIDI SoundFont synthesis |
| github.com/gotracker/playback | MOD/XM/IT/S3M playback |
| github.com/godbus/dbus/v5 | Linux MPRIS |
| github.com/alicebob/alac (forked, internal) | ALAC decoder for M4A |
| github.com/pion/opus | Opus decoder for OGG |