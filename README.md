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

| Format | Description | Note |
|--------|-------------|---------|
| `wav` | Wave |  |
| `mp3` | MPEG Layer 3 |  |
| `flac` | Free Lossless Audio Codec |  |
| `ogg`, `oga` | OGG Vorbis |  |
| `m4a/alac` | Apple Lossless |  |
| `opus` | Opus |  |
| `mp2` | MPEG-1 Audio Layer II |  |
| `ape` | Monkey's Audio | requires `apecil`, `ffmpeg` or `mac` |
| `mid`, `midi` | Musical Instrument Digital Interface | requires SoundFont |
| `mod` | Module Music |  |
| `xm` | Extended Module |  |
| `it` | Impulse Tracker |  |
| `s3m` | ScreamTracker 3 Module |  |
| [Tracker](https://openmpt.org/features#modules) | More Tracker Module | require `libopenmpt` |

#### Lyrics File

| Format | Description | Note |
|--------|-------------|---------|
| `lrc` | Standard Synced Lyrics File |  |
| `srt` | SubRip Text | |
| `ttml` | Timed Text Markup Language | EXPERIMENTAL |
| `yrc` | NetEase Cloud Music Word-for-Word Lyrics | EXPERIMENTAL |
| `qrc` | QQ Music Word-for-Word Lyrics | EXPERIMENTAL |
| `eslrc` | Enhanced Synced Lyrics File | EXPERIMENTAL |
| `lys` | LYS Lyrics File | EXPERIMENTAL |
| `smi` | Synchronized Accessible Media Interchange | EXPERIMENTAL |

## Usage

Simply run:
~~~bash
./neoviolet /path/to/audiofile
~~~

> [!TIP]
> **Modern terminal emulators like Windows Terminal, Konsole, iTerm2 or Ghostty is recommended!**
> Issues may occurred when using this program on outdated terminal emulators like xterm.

## Build

Checkout [`BUILD.md`](./docs/BUILD.md) for more infomation!

## License

This application is open source under the **MIT license**. For the licenses of its dependencies, please refer to [`ACKNOWLEDGEMENTS.md`](./docs/ACKNOWLEDGEMENTS.md).
