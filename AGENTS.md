# AGENTS.md

Rules for AI agents working in this repo. Read [Keeping this file honest](#keeping-this-file-honest)
before editing it.

## Working agreement

- **Think first** — state your reading of the task and any simpler alternative; ask when unsure.
- **Minimal, surgical changes** — no extras, no abstractions, no refactors of working code.
- **Prove it** — turn the task into a checkable goal, verify each step, iterate until it passes.
- **No unsolicited commits** — leave the work in the tree.
- **Docs are output, not a scratchpad** — touch `README.md` or `docs/` only on request or to fix
  a stale statement, never to log what you did.
- **`docs/BUILD.md` covers building only** — prereqs, commands, platforms, troubleshooting. A
  section that would still make sense to someone who never compiles anything does not belong there.
- **Local-only material lives in `.dev/`** — specs, plans, verification output and other scratch
  assets are gitignored there and committed only when asked. Nothing tracked may reference an
  untracked document.

## Hard constraints

- **`make` only** — never run `go build`/`go test`/`cargo build`/`cargo test` directly; you lose
  the flags the Makefile sets.
- **No CGo** — load native libraries at runtime with purego/dlopen (`internal/power`,
  `internal/mediactl`, `internal/audio/synth`).
- **`CGO_ENABLED=1` still needs a working `CC`** — the race tests compile the cgo shim, so check
  the toolchain before blaming the source for a link error.
- **`make test` does not rebuild the Rust binaries the app spawns** — `make gui` and
  `make apetools` do.
- **TUI work does not build the GUI** — use `make run` or `make build/tui`. `make build` prepares
  every component; `make run/gui` builds before launching.
- **Rust is optional for the TUI, required for the GUI** — without cargo, TUI builds warn and skip
  the Rust parts while `run/gui` and `build/macos` fail. A missing apecli falls back to ffmpeg or
  the Monkey's Audio `mac` CLI. Rust compile or copy failures are always errors.
- **libopenmpt is loaded at runtime** — its version lives in `deps/libopenmpt.lock`, not `go.mod`;
  `deps/` plus `.github/workflows/deps-libopenmpt.yml` build it, `internal/audio/synth` finds it.
- **Never commit build output or local state** — binaries, `coverage.*`, `dist/`, `config.json`,
  `history.txt`, `caches/`.

## Licenses

| Path | License |
| --- | --- |
| root, `cmd/`, `internal/` (TUI) | MIT |
| `tools/apecli/` | MIT |
| `tools/neoviolet-gui/` (GUI, excluding third-party sources) | MIT |
| `assets/icons/`, `tools/neoviolet-gui/assets` | (c) AuroraStudio, all rights reserved |

Third-party code keeps its license and upstream origin: no copyleft dependencies, and every
release ships the notices in `docs/ACKNOWLEDGEMENTS.md`. The icons sit outside the MIT grant — free
to use for any legitimate purpose without asking, but do not relicense them.

## Quality gates

`make check` runs the gates locally; CI runs them separately (`.github/workflows/build.yml`, which
pins the tool versions). Thresholds live in their gate (`.golangci.yml`, `Makefile`), not here.
Without cargo the Rust gates warn and skip, so a passing `make check` says nothing about Rust. CI
also runs `govulncheck`, which has no local target.

## Architecture

    neoviolet (Go)        TUI — Bubble Tea, Elm-style model/view/update
    neoviolet-gui (Rust)  optional wrapper (Iced + iced_term)
    apecli (Rust)         optional APE decoder subprocess, stdin → PCM stdout

- **Two separate programs** joined by PTY + TCP IPC (loopback, random port, token in a temp file).
  The GUI owns no playback decisions; IPC snapshots are read-only projections of Go state.
  Go alone saves user preferences, including GUI and desktop lyrics settings; the GUI saves
  window geometry separately in `gui-state.json` and reads `config.json` only for bootstrap.
  The TUI must run standalone: anything that needs the GUI attached is a bug.
- IPC message types are defined in the `internal/ipc` package comment — that is the contract; add
  a variant there first.
- OS APIs live behind per-OS build tags or files (`internal/mediactl/controller_stub.go` covers the
  rest). `runtime.GOOS` is for capability detection (font lookup in `internal/ui/wizard`), not for
  OS APIs.
- GUI: English controls, dark gray, no light mode.

### Extension points

| Concern | Add it via | Current set |
| --- | --- | --- |
| Audio format | `registerFormat` + `register*Probe` in `internal/audio/format` | `grep -rn 'registerFormat(' internal/audio` |
| Lyrics format | `LyricParser` + `RegisterParser` in `internal/lyrics` | `grep -rn 'RegisterParser(' internal/lyrics` |
| Lyric source | one provider in `internal/lyrics/fetch`, no registry | `ls internal/lyrics/fetch` |
| Keybinding | `KeyMap` entry plus a handler in `internal/ui` | `grep -rn 'KeyMap' internal/ui` |
| TUI styling | Lipgloss style beside its peers in `internal/ui` | `grep -rn 'lipgloss.Style' internal/ui` |
| GUI action or dialog | Iced message → update → view | `tools/neoviolet-gui` |
| GUI terminal | patched `iced_term` | `tools/neoviolet-gui/deps` |

## Working in this repo

- **Discover, don't recall**: `make help`, `go list ./...`, `go doc ./internal/<pkg>`,
  `docs/BUILD.md`, `docs/ACKNOWLEDGEMENTS.md` (licenses: one row each, texts in the appendix).
- **Loop**: put new logic in its own file beside its peers, test in the same package, iterate with
  `make test/short`, finish with `make check`. If your change falsifies this file, fix it here too.
- **Keep caches**: `make clean` drops root binaries and coverage; only `make clean/all` clears the
  Rust cache.

## Keeping this file honest

- Write invariants, contracts and workflows — what stays true as files move.
- No directory trees, file inventories, version or threshold numbers, or full `make` target lists;
  each has a live home (`go list`, `.github/workflows/build.yml`, `Makefile`, `make help`).
- Don't restate what the code documents — point at the package. When something stops being true,
  fix or delete it; never append a correction.
