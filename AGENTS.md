# AGENTS.md

For AI agents. Read [Keeping this file honest](#keeping-this-file-honest) before editing it.

## Working agreement

- **Think before coding** — state your interpretations and simpler alternatives; ask if unsure.
- **Simplicity first** — minimum code, no extras or abstractions unless requested.
- **Surgical changes** — edit only what's needed, match existing style, don't refactor working code.
- **Goal-driven** — task → testable goal → plan → verify checkpoints → iterate until pass.
- **No unsolicited commits** — leave changes in the working tree.
- **Keep working documents out of the repo, and never reference them from it.** Specs, plans
  and other engineering docs you produce are committed only if explicitly asked. Nothing
  tracked may refer to an untracked document in any form — not code comments, tracked docs,
  commit messages or PR descriptions.

## Hard constraints

- **`make` only** — never run `go build`/`go test`/`cargo build`/`cargo test` directly; you lose
  the build tags, `-ldflags` and test flags the Makefile adds.
- **CGo is required** — check `CGO_ENABLED=1` and a working `CC` before blaming a link error on source.
- **`make test` is not a Rust build** — it only runs `cargo test`; the release binaries the app
  spawns need `make apetools` / `make gui`.
- **Rust is optional at runtime** — `apecli` comes from PATH or beside the binary (ffmpeg/macOS
  fallback), the GUI is an optional wrapper. No cargo → `make build` warns and continues, by design.
- **Never commit build artifacts or local state** — root binaries, `coverage.*`, `config.json`,
  `history.txt`, `caches/`.

## License boundary — GPL must not reach the MIT side

| Path | License |
|---|---|
| root, `cmd/`, `internal/` (TUI) | MIT |
| `tools/apecli/` | MIT |
| `tools/neoviolet-gui/` (GUI) | GPL-3.0-or-later |

Copyleft must never flow into the MIT side: don't copy code, types, strings or comments out of the
GUI; don't link, embed, vendor or share sources across them; don't add copyleft dependencies to the
MIT side. MIT → GUI is compatible. Keep them **separate programs** — the PTY + IPC split is the
license boundary, so never merge them into one binary. Leave the license fields and the
per-component `LICENSE` files alone; the app bundle ships both.

## Quality gates

`make check` is the local aggregate; CI runs the same gates individually
(`.github/workflows/build.yml`, which also pins Go/Rust/linter versions). Thresholds live with
their gate, not here: `make lint` (`.golangci.yml`), `make lint/rust` (clippy `-D warnings`),
`make test` / `test/race` (`TEST_FLAGS`), `make test/coverage` (`COVERAGE_MIN`),
`make check/linelength`, `make tidy/check`, `make vet`.

## Architecture invariants

```
neoviolet (Go)        TUI — Bubble Tea, Elm-style model/view/update
neoviolet-gui (Rust)  optional native wrapper (gpui-ce + yororen-ui)
apecli (Rust)         optional APE decoder subprocess, stdin → PCM stdout
```

- The GUI holds no playback state: it spawns `neoviolet` in a PTY and drives it over TCP IPC
  (loopback, random port + token in a temp file). **The TUI must run standalone** — anything that
  works only with the GUI attached is a bug.
- IPC message types live in the `internal/ipc` package comment, the contract both sides are
  written against. Add the variant there first.
- OS integration sits behind per-OS build tags or platform modules; don't branch on
  `runtime.GOOS` in shared code.

### Extension points

Register in `init()` beside the existing entries:

| Concern | Mechanism | Check the current set |
|---|---|---|
| Audio format | `registerFormat` + `register*Probe`, `internal/audio/format` | `grep -rn 'registerFormat(' internal/audio` |
| Lyrics format | `LyricParser` + `RegisterParser`, `internal/lyrics` | `grep -rn 'RegisterParser(' internal/lyrics` |
| Lyric source | `internal/lyrics/fetch` (client, matching, cache, rate limit) | package comment |
| Keybinding | `KeyMap` + a handler beside the others, `internal/ui` | `grep -rn 'KeyMap' internal/ui` |
| TUI styling | Lipgloss styles beside the views, `internal/ui` | `grep -rn 'lipgloss.Style' internal/ui` |
| GUI menu action | action in `menus.rs` → `setup()` → handled in `neo_violet_app.rs` | read `menus.rs` |
| GUI dialog / theme | `AppState` flag + dialogs module; theme module + `GuiConfig` | module docs |

## Working in this repo

- **Discovery beats memory**: `make help` (targets), `go list ./...` (packages), `docs/BUILD.md`
  (prereqs, platforms, troubleshooting), `docs/ACKNOWLEDGEMENTS.md` (third-party code, assets,
  dependency licenses), `go doc ./internal/<pkg>` (subsystem intent).
- **Loop**: register beside the existing entries, keep the logic in its own file next to its
  peers, test in the same package, iterate with `make test/short`, finish with `make check`. If
  your change falsifies anything here — including the license table — fix it in the same change.

## Keeping this file honest

- Write invariants, contracts and workflows — things that hold as files move.
- Don't add directory trees, per-file inventories, version or threshold numbers, or the full
  `make` target list; each has a live home (`go list`, `.github/workflows/build.yml`, `Makefile`,
  `make help`). Copying them here is how this file rots.
- Don't restate what the code documents; point at the package. When a statement stops being true,
  fix or delete it — never append a correction.
