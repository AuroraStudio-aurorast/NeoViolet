~~~
███╗   ██╗███████╗ ██████╗
████╗  ██║██╔════╝██╔═══██╗
██╔██╗ ██║█████╗  ██║   ██║
██║╚██╗██║██╔══╝  ██║   ██║
██║ ╚████║███████╗╚██████╔╝
╚═╝  ╚═══╝╚══════╝ ╚═════╝

██╗   ██╗██╗ ██████╗ ██╗     ███████╗████████╗
██║   ██║██║██╔═══██╗██║     ██╔════╝╚══██╔══╝
██║   ██║██║██║   ██║██║     █████╗     ██║
╚██╗ ██╔╝██║██║   ██║██║     ██╔══╝     ██║
 ╚████╔╝ ██║╚██████╔╝███████╗███████╗   ██║
  ╚═══╝  ╚═╝ ╚═════╝ ╚══════╝╚══════╝   ╚═╝
~~~

# NeoViolet - A Terminal Music Player

> [!IMPORTANT]
> This program is work in progress!

## Features

### Supported Formats

#### Audio File

| Format | Description | Backend |
|--------|-------------|---------|
| `wav` | Wave | beep |
| `mp3` | MPEG Layer 3 | beep + go-mp3 |
| `flac` | Free Lossless Audio Codec | beep + mewkiz/flac |
| `ogg`, `oga` | OGG Vorbis | beep + jfreymuth/oggvorbis |
| `mid` | Musical Instrument Digital Interface | meltysynth (requires .sf2) |
| `mod` | Module Music | gotracker / libopenmpt |
| `xm` | Extended Module | gotracker / libopenmpt |
| `it` | Impulse Tracker | gotracker / libopenmpt |
| `s3m` | ScreamTracker 3 Module | gotracker / libopenmpt |
| `mptm` | OpenMPT Module Music | libopenmpt only |

#### Lyrics File

- `lrc`: Standard synced lyrics file
- `ttml`: Timed Text Markup Language (W3C standard)

## Build

### Pre-requirements

- **Go 1.26+** (see `go.mod` for exact version)
- **Optional**: `libopenmpt` (for `mptm` format and enhanced tracker playback)
  - macOS: `brew install libopenmpt`
  - Linux: `apt install libopenmpt-dev` (Debian/Ubuntu) / `dnf install libopenmpt-devel` (Fedora) / `pacman -S libopenmpt` (Arch Linux)
  - Windows: download from [libopenmpt.org](https://lib.openmpt.org/)
- **Optional**: SoundFont file (`.sf2`) for MIDI playback

### Compile

```bash
# Build production binary
make build

# Build with race detector (for debugging)
make build/race

# Build with debug symbols (dlv compatible)
make build/debug

# Build and run (with optional arguments)
make run ARGS="/path/to/audio.mp3"

# Or use Go directly
go build -o neoviolet ./cmd/neoviolet
```

The Makefile automatically detects `libopenmpt` via `pkg-config` and adds the `openmpt` build tag if available.

### Install

```bash
make install
```

Installs the binary to `$GOPATH/bin`.

### Test

```bash
# Run all tests
make test

# Run tests with race detector
make test/race

# Run tests verbosely
make test/verbose

# Run short tests (skips integration)
make test/short

# Run with coverage report
make test/cover
```

### Code Quality

```bash
make vet        # go vet
make lint       # golangci-lint (fallback: go vet)
make tidy       # go mod tidy
```

### Clean

```bash
make clean
```

## License

This application is open source under the **MIT license**. For the licenses of its dependencies, please refer to `ACKNOWLEDGEMENTS.md`.
