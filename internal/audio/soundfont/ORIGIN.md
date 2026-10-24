# Origin

This package is copied from the `meltysynth` package of
[github.com/jpodeszfa/go-meltysynth](https://github.com/jpodeszfa/go-meltysynth),
version v1.0.1. That library is a Go port of MeltySynth,
[github.com/sinshu/go-meltysynth](https://github.com/sinshu/go-meltysynth), by
Nobuaki Tanaka.

Module checksum: `github.com/jpodeszfa/go-meltysynth v1.0.1 h1:Pn0zC0e/5/vjew4F8ChfhDWd4ZSUdfWt/IxJK5Ws308=`

## Scope

The copy is limited to SoundFont 2 (SF2) parsing and the synthesis DSP. The
upstream MIDI file reader and sequencer (`midifile.go`,
`midifile_sequencer.go`) were not copied; MIDI scheduling is implemented
independently elsewhere in NeoViolet.

## Changes

NeoViolet adapts the parser for bounded RIFF/table reads and streaming PCM,
retains empty instruments, validates effective sample/loop bounds, and shares
immutable font data across synthesizers. It adds SF2 modulators, controller
sources, explicit preset selection and voice statistics. Internal names and
unused inherited accessors were cleaned up for the project quality gates.
Tests use generated fixtures; upstream asset-dependent tests are not included.

Modulator records, source curves, defaults and hierarchy follow the SoundFont
2.04 specification, sections 7.4/7.8, 8.1–8.4 and 9.5:
https://www.polyphone.io/doc/files/sf_specifications_v2.04.pdf
The published document contains legacy 2.01 page footers and known prose/table
inconsistencies: CC7's source is 0x0587 (CC index 7), CC10 targets pan and uses
500 to span its defined -500..500 range. MIDI 7-bit endpoints reach full scale
while bipolar value 64 is exactly centered; the pitch wheel keeps its native
8192 center. This is an explicit endpoint convention. Envelope phase origins
are latched at note-on; modulation changes rates and sustain without restarting
an attack. Sample addresses, root key and scale tuning are not live targets.
Linked/reserved modulators are diagnosed and ignored.

## License

MIT, reproduced verbatim in [LICENSE](LICENSE). The upstream copyright notices
(Copyright (C) 2021 Nobuaki Tanaka for MeltySynth, Copyright (C) 2022 Nobuaki
Tanaka for Go-MeltySynth) are retained there.
