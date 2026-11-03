# GUI dependency sources

The GUI uses Iced with iced_term and a small macOS winit fix. These are independent
upstream components; no code comes from the former GPL GUI.

`sources.json` pins the exact source archives and their SHA-256 digests. The patches
in this directory are the complete local changes. `make gui/deps` verifies cached
archives, downloads a missing archive, and applies the patches into `.build/gui-deps`.
All GUI build, test and lint targets prepare these sources automatically. Sources and
archive caches stay outside Git. Git is required to apply the patches.

| Component | Upstream | License | Changes |
|---|---|---|---|
| iced_term | `230945e443c7bded8eac0786240210a51a0e8303` (PR 73) | MIT | Validated IME fixes, process ID and selection access, terminal query responses, bracketed paste, application and menu shortcuts, opacity and scroll zoom, modal input isolation, subscription shutdown |
| winit | 0.30.13 / `e9809ef54b18499bb4f2cac945719ecc2a61061b` | Apache-2.0 | Discard marked text when disabling IME; retain document/reopen/quit callbacks and preinstalled macOS menus; refresh the final size and frame after native fullscreen transitions |

Original license texts are retained under `licenses/` and shipped in macOS bundles.
iced_term's manifest is made standalone to avoid depending on example assets.

To change a dependency, update its source pin and digest, regenerate the corresponding
patch against that pristine source, then run `make gui/deps` and the relevant `make`
checks. Edit the tracked patches rather than committing generated source files.
The preparation stamp includes the patch contents, so changing a patch regenerates
its source directory. Archives are reusable across worktrees in the OS user cache.

Upstream references:

- https://github.com/kemokempo/iced_term/pull/73
- https://github.com/rust-windowing/winit/tree/v0.30.13
