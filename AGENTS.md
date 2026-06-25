# AGENTS.md

## Principle

- **Think before coding**: List interpretations, propose simpler alternatives, ask if unsure.
- **Simplicity first**: Minimum code, no extras/abstractions unless requested.
- **Surgical changes**: Edit only what's needed, don't refactor what isn't broken, match existing style.
- **Goal-driven**: Task → testable goal → plan → verify checkpoints → iterate until pass.

## Build & Test

All operations go through `make` — never call `go build`/`go test`/`cargo build` directly:

```bash
make build               # Go TUI + apecli + neoviolet-gui (auto-detects libopenmpt)
make build/race          # Race detector
make build/debug         # Debug symbols (dlv-compatible)
make build/noopenmpt     # Without libopenmpt support

make gui                 # Release (LTO) → ./neoviolet-gui
make gui/debug           # Debug build
make run/gui ARGS=...    # Build GUI + launch
make build/osxappbundle  # macOS .app bundle (macOS only)

make apetools            # cargo build --release in tools/apecli/
make apetools/debug      # cargo build (debug)

make run ARGS="<file>"   # Build TUI + run
make install             # → $GOPATH/bin

make test                # All tests
make test/verbose        # -v
make test/race           # Race detector
make test/cover          # → coverage.html
make test/short          # Skip integration tests

make vet                 # go vet
make lint                # golangci-lint (falls back to go vet)
make tidy                # go mod tidy
make clean               # Go + Rust artifacts
```

### Prerequisites

- **Go 1.26+** — [Download](https://go.dev/dl/). Windows: use MSYS2.
- **Rust + cargo** — builds `apecli` and `neoviolet-gui`. Missing → warns + exits cleanly. Install via [rustup](https://rustup.rs/).
- **C compiler** — CGo links C libs. `CGO_ENABLED=1`; `CC=clang` recommended.
- **`make`**, **`pkg-config`**
- **`libopenmpt`** (optional) — tracker playback (`.mptm`). Most formats work via `gotracker/playback`.
- **Linux GUI**: `libxcb1-dev libxkbcommon-dev libxkbcommon-x11-dev libwayland-dev` (see [`docs/BUILD.md#gui-prerequisites`](docs/BUILD.md#gui-prerequisites)).
- **macOS GUI**: no extra deps; Makefile uses `runtime_shaders` to skip Metal Toolchain.

**For AI agents:** Always verify `CGO_ENABLED=1` and `CC`. `make test` does NOT rebuild Rust — run `make apetools`/`make gui` first after Rust changes.

---

## Architecture

```
cmd/neoviolet/              # Go entry → TUI (Bubble Tea, Elm architecture)
internal/
  audio/format/             # Format decoders (registerFormat + register*Probe in init)
  audio/synth/              # MIDI SoundFont synthesis
  config/ cover/ lyrics/    # Config, album art, pluggable lyric parsers (RegisterParser)
  ui/                       # model/view/update, keyboard, audio_state
  mediactl/                 # OS media control (per-OS build tags)
  accent/ logger/

tools/neoviolet-gui/        # Native GUI (Rust, gpui-ce + yororen-ui)
  src/
    main.rs neo_violet_app.rs  # Entry, app shell, IPC dispatch
    app.rs backend.rs           # PTY terminal + neoviolet child process
    ipc.rs state.rs config.rs   # TCP IPC, shared state, GuiConfig
    menus.rs desktop_lyrics.rs  # Actions, keybindings, lyrics overlay
    components/ terminal/       # Dialogs, alacritty_terminal rendering
    platform.rs dracula_theme.rs util.rs

tools/apecli/               # APE decoder (Rust, stdin→PCM stdout)
```

**GUI ↔ TUI:** GUI spawns `neoviolet` in a PTY. Two channels: PTY for terminal I/O, TCP IPC (temp-file address + token auth) for structured control. GUI is optional — TUI runs standalone.

---

## Common Tasks

### TUI (Go)

| Task | Where |
|---|---|
| Add lyrics format | New file in `internal/lyrics/`, `LyricParser` + `RegisterParser()` in `init()` |
| Add audio format | New file in `internal/audio/format/`, `registerFormat()` + appropriate `register*Probe()` in `init()` |
| Change shortcuts | `KeyMap` in `internal/ui/types.go` + handler in `internal/ui/update_keyboard.go` |
| Tweak UI | Lipgloss styles in `internal/ui/view.go` and `internal/ui/types.go` |

### GUI (Rust)

| Task | Where |
|---|---|
| Add menu action / keybinding | `menus.rs`: define action → `setup()` → handle in `neo_violet_app.rs` |
| Add IPC message | `ipc.rs`: add variant → dispatch in `neo_violet_app.rs` → send from Go `internal/ui/` |
| Add dialog | `components/dialogs.rs`: render fn → trigger via `AppState` flag |
| Change theme / font | `dracula_theme.rs` / `platform.rs` + `config.rs` |
| Tweak desktop lyrics | `desktop_lyrics.rs` |
