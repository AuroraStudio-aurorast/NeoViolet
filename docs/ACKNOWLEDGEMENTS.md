# Acknowledgements

NeoViolet is MIT licensed. Its own three components come first; everything
after them is third-party code, grouped by the component that links or copies
it. Each entry names its license and copyright, and the verbatim text of every
license appears once in [Appendix A](#appendix-a-license-texts) — no license is
repeated per dependency.

Versions are the `go.mod` and `Cargo.lock` values as-is; `—` means the source
states nothing for that field.

## First-party components

| Component | Path | License | Notice file |
|---|---|---|---|
| `neoviolet` | / | [MIT](#mit) | [`LICENSE`](../LICENSE) |
| `apecli` | /tools/apecli | [MIT](#mit) | [`LICENSE`](../tools/apecli/LICENSE) |
| `neoviolet-gui` | /tools/neoviolet-gui | [MIT](#mit) | [`LICENSE`](../tools/neoviolet-gui/LICENSE) |

The macOS bundle ships these as `LICENSE-MIT` and `LICENSE-GUI-MIT`.

## neoviolet dependencies (Go)

| Dependency | Version | License | Copyright |
|---|---|---|---|
| `charm.land/bubbletea/v2` | v2.1.1 | [MIT](#mit) | Copyright (c) 2020-2026 Charmbracelet, Inc. |
| `charm.land/lipgloss/v2` | v2.0.6 | [MIT](#mit) | Copyright (c) 2021-2026 Charmbracelet, Inc. |
| `charm.land/bubbles/v2` | v2.2.1 | [MIT](#mit) | Copyright (c) 2020-2026 Charmbracelet, Inc. |
| `charm.land/log/v2` | v2.0.1 | [MIT](#mit) | Copyright (c) 2022-2023 Charmbracelet, Inc |
| `charm.land/huh/v2` | v2.0.3 | [MIT](#mit) | Copyright (c) 2023-2026 Charmbracelet, Inc. |
| `github.com/gopxl/beep/v2` | v2.1.1 | [MIT](#mit) | Copyright (c) 2017 Michal Štrba |
| `github.com/dhowden/tag` | v0.0.0-20240417053706-3d75831295e8 | [BSD-2-Clause](#bsd-2-clause) | Copyright 2015, David Howden |
| `github.com/jpodeszfa/go-meltysynth/meltysynth` | v1.0.1 | [MIT](#mit) | MeltySynth: Copyright (C) 2021 Nobuaki Tanaka; Go-MeltySynth: Copyright (C) 2022 Nobuaki Tanaka |
| `github.com/gotracker/playback` | v1.5.0 | [MIT](#mit) | Copyright (c) 2015 Jason Crawford |
| `github.com/WhatDamon/go-amll-ttml-parser` | v0.0.0-20260918094540-d8fe6ac26bd6 | [MIT](#mit) | Copyright (c) 2026 Damon Lu |
| `github.com/WhatDamon/go-nvaa-codec` | v0.0.0-20261009094822-0e5842a563d5 | [MIT](#mit) | Copyright (c) 2026 Damon Lu |
| `github.com/EdlinOrg/prominentcolor` | v1.0.0 | [BSD-3-Clause](#bsd-3-clause-prominentcolor) | Copyright (c) 2016 Carl Asman. |
| `github.com/lucasb-eyer/go-colorful` | v1.4.1 | [MIT](#mit) | Copyright (c) 2013 Lucas Beyer |
| `github.com/godbus/dbus/v5` | v5.2.4 | [BSD-2-Clause](#bsd-2-clause) | Copyright (c) 2013, Georg Reinke, Google |
| `github.com/ebitengine/oto/v3` | v3.5.0 | [Apache-2.0](#apache-20) | — |
| `github.com/ebitengine/purego` | v0.11.2 | [Apache-2.0](#apache-20) | — |
| `github.com/hajimehoshi/go-mp3` | v0.3.4 | [Apache-2.0](#apache-20) | — |
| `github.com/pion/opus` | v0.1.0 | [MIT](#mit) | Copyright (c) 2018 |
| `github.com/gen2brain/mpeg` | v0.6.2 | [MIT](#mit) | Copyright (c) 2022, Milan Nikolic <gen2brain> |
| `github.com/icza/bitio` | v1.1.0 | [Apache-2.0](#apache-20) | — |
| `github.com/jfreymuth/oggvorbis` | v1.0.5 | [MIT](#mit) | Copyright (c) 2016 Johann Freymuth |
| `github.com/jfreymuth/vorbis` | v1.0.2 | [MIT](#mit) | Copyright (c) 2016 Johann Freymuth |
| `github.com/mewkiz/flac` | v1.0.14 | [Unlicense](#unlicense) | — |
| `github.com/spf13/cobra` | v1.10.2 | [Apache-2.0](#apache-20) | — |
| `github.com/mewkiz/pkg` | v0.0.0-20260703220044-4fb89b18cc87 | [Unlicense](#unlicense) | — |
| `github.com/catppuccin/go` | v0.3.0 | [MIT](#mit) | Copyright (c) 2024 Catppuccin |
| `github.com/mitchellh/hashstructure/v2` | v2.0.2 | [MIT](#mit) | Copyright (c) 2016 Mitchell Hashimoto |
| `libopenmpt` (system library) | — | [BSD-3-Clause](#bsd-3-clause-libopenmpt)[^openmpt] | Copyright (c) 2004-2026, OpenMPT Project Developers and Contributors; Copyright (c) 1997-2003, Olivier Lapicque |

[^openmpt]: Loaded at run time, not linked. Windows release archives ship
    `libopenmpt.dll` next to the binary; other platforms open whichever
    `libopenmpt` the system provides (`openmptCandidates`), so their binaries
    contain none of this code.

### Code copied into this repository

Not dependencies: source was copied or adapted, so the upstream copyright and
license still apply to that code.

| Source | In tree | License | Copyright |
|---|---|---|---|
| `github.com/alicebob/alac` | `internal/audio/format/alac/` (modified) | [MIT](#mit) | Copyright (c) 2016 Harmen |
| `github.com/go-musicfox/go-musicfox` | `internal/mediactl/controller_darwin.go` (reference) | [MIT](#mit)[^musicfox] | Copyright (c) 2015 - present Microsoft Corporation |

[^musicfox]: Reproduced as upstream ships it. Upstream later declared GPL-3.0 in its
    changelog while its `LICENSE` file still carries this MIT text, so the notice
    here matches the file, not the declared license.

## apecli dependencies (Rust)

| Dependency | Version | License | Copyright |
|---|---|---|---|
| `ape-decoder` | 0.3.2 | [MIT](#mit) | Copyright (c) 2026 ombs.io |

## neoviolet-gui dependencies (Rust)

| Dependency | Version | License | Copyright |
|---|---|---|---|
| `iced` | 0.14.0 | [MIT](#mit) | Copyright 2019 Héctor Ramón, Iced contributors |
| `iced_term` | 0.8.0 | [MIT](#mit)[^patched] | Copyright (c) 2024 Ilia Shvyrialkin |
| `winit` | 0.30.13 | [Apache-2.0](#apache-20)[^patched] | — |
| `alacritty_terminal` | 0.25.1 | [Apache-2.0](#apache-20) | — |
| `iced_wgpu` | 0.14.0 | [MIT](#mit)[^patched] | Copyright 2019 Héctor Ramón, Iced contributors |
| `iced_tiny_skia` | 0.14.0 | [MIT](#mit) | Copyright 2019 Héctor Ramón, Iced contributors |
| `wgpu` | 27.0.1 | [MIT](#mit) | Copyright (c) 2025 The gfx-rs developers |
| `naga` | 27.0.3 | [MIT](#mit) | Copyright (c) 2025 The gfx-rs developers |
| `tiny-skia` | 0.11.4 | [BSD-3-Clause](#bsd-3-clause) | Copyright (c) 2011 Google Inc.; Copyright (c) 2020 Yevhenii Reizner |
| `softbuffer` | 0.4.8 | [MIT](#mit) | Copyright 2022 Kirill Chibisov |
| `cryoglyph` | 0.1.0 | [MIT](#mit) | — |
| `cosmic-text` | 0.15.0 | [MIT](#mit) | Copyright (c) 2022 System76 |
| `harfrust` | 0.3.2 | [MIT](#mit) | Copyright (c) HarfBuzz developers; Copyright (c) 2020 Yevhenii Reizner |
| `swash` | 0.2.10 | [MIT](#mit) | Copyright (c) 2020 Chad Brokaw |
| `fontdb` | 0.23.0 | [MIT](#mit) | Copyright (c) 2020 Yevhenii Reizner |
| `rfd` | 0.15.4 | [MIT](#mit) | Copyright (c) 2022 Bartłomiej Maryńczak |
| `ashpd` | 0.11.1 | [MIT](#mit) | Copyright (c) 2020 Bilal Elmoussaoui |
| `muda` | 0.17.2 | [MIT](#mit)[^patched] | Copyright (c) 2022-2022 Tauri Programme within The Commons Conservancy |
| `objc2` | 0.6.5 | [MIT](#objc2-crates) | — |
| `objc2-app-kit` | 0.3.2 | [MIT · Zlib · Apache-2.0](#objc2-crates) | — |
| `objc2-foundation` | 0.3.2 | [MIT](#objc2-crates) | — |
| `block2` | 0.6.2 | [MIT](#objc2-crates) | — |
| `windows-sys` | 0.61.2 | [MIT](#mit) | Copyright (c) Microsoft Corporation. |
| `x11rb` | 0.13.2 | [MIT](#mit) | Copyright 2019 x11rb Contributers |
| `serde` | 1.0.229 | [MIT](#mit) | — |
| `serde_json` | 1.0.154 | [MIT](#mit) | — |
| `tokio` | 1.54.1 | [MIT](#mit) | Copyright (c) Tokio Contributors |
| `toml` | 1.1.8+spec-1.1.0 | [MIT](#mit) | Copyright (c) Individual contributors |
| `unicode-segmentation` | 1.13.3 | [MIT](#mit) | Copyright (c) 2015 The Rust Project Developers |
| `open` | 5.4.5 | [MIT](#mit) | Copyright © 2015 Sebastian Thiel |

[^patched]: Local patches apply. The pin, digest and change description for each
    patched crate live in [`tools/neoviolet-gui/deps/README.md`](../tools/neoviolet-gui/deps/README.md);
    only the license is reproduced here.

## Appendix A: license texts

The copyright line for each entry stays in its table above; the text below
completes it. Where a project publishes its own wording of a standard license,
that wording is reproduced as published instead of the canonical SPDX text.

### MIT

~~~
Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
~~~

### Apache-2.0

~~~
                                 Apache License
                           Version 2.0, January 2004
                        http://www.apache.org/licenses/

   TERMS AND CONDITIONS FOR USE, REPRODUCTION, AND DISTRIBUTION

   1. Definitions.

      "License" shall mean the terms and conditions for use, reproduction,
      and distribution as defined by Sections 1 through 9 of this document.

      "Licensor" shall mean the copyright owner or entity authorized by
      the copyright owner that is granting the License.

      "Legal Entity" shall mean the union of the acting entity and all
      other entities that control, are controlled by, or are under common
      control with that entity. For the purposes of this definition,
      "control" means (i) the power, direct or indirect, to cause the
      direction or management of such entity, whether by contract or
      otherwise, or (ii) ownership of fifty percent (50%) or more of the
      outstanding shares, or (iii) beneficial ownership of such entity.

      "You" (or "Your") shall mean an individual or Legal Entity
      exercising permissions granted by this License.

      "Source" form shall mean the preferred form for making modifications,
      including but not limited to software source code, documentation
      source, and configuration files.

      "Object" form shall mean any form resulting from mechanical
      transformation or translation of a Source form, including but
      not limited to compiled object code, generated documentation,
      and conversions to other media types.

      "Work" shall mean the work of authorship, whether in Source or
      Object form, made available under the License, as indicated by a
      copyright notice that is included in or attached to the work
      (an example is provided in the Appendix below).

      "Derivative Works" shall mean any work, whether in Source or Object
      form, that is based on (or derived from) the Work and for which the
      editorial revisions, annotations, elaborations, or other modifications
      represent, as a whole, an original work of authorship. For the purposes
      of this License, Derivative Works shall not include works that remain
      separable from, or merely link (or bind by name) to the interfaces of,
      the Work and Derivative Works thereof.

      "Contribution" shall mean any work of authorship, including
      the original version of the Work and any modifications or additions
      to that Work or Derivative Works thereof, that is intentionally
      submitted to Licensor for inclusion in the Work by the copyright owner
      or by an individual or Legal Entity authorized to submit on behalf of
      the copyright owner. For the purposes of this definition, "submitted"
      means any form of electronic, verbal, or written communication sent
      to the Licensor or its representatives, including but not limited to
      communication on electronic mailing lists, source code control systems,
      and issue tracking systems that are managed by, or on behalf of, the
      Licensor for the purpose of discussing and improving the Work, but
      excluding communication that is conspicuously marked or otherwise
      designated in writing by the copyright owner as "Not a Contribution."

      "Contributor" shall mean Licensor and any individual or Legal Entity
      on behalf of whom a Contribution has been received by Licensor and
      subsequently incorporated within the Work.

   2. Grant of Copyright License. Subject to the terms and conditions of
      this License, each Contributor hereby grants to You a perpetual,
      worldwide, non-exclusive, no-charge, royalty-free, irrevocable
      copyright license to reproduce, prepare Derivative Works of,
      publicly display, publicly perform, sublicense, and distribute the
      Work and such Derivative Works in Source or Object form.

   3. Grant of Patent License. Subject to the terms and conditions of
      this License, each Contributor hereby grants to You a perpetual,
      worldwide, non-exclusive, no-charge, royalty-free, irrevocable
      (except as stated in this section) patent license to make, have made,
      use, offer to sell, sell, import, and otherwise transfer the Work,
      where such license applies only to those patent claims licensable
      by such Contributor that are necessarily infringed by their
      Contribution(s) alone or by combination of their Contribution(s)
      with the Work to which such Contribution(s) was submitted. If You
      institute patent litigation against any entity (including a
      cross-claim or counterclaim in a lawsuit) alleging that the Work
      or a Contribution incorporated within the Work constitutes direct
      or contributory patent infringement, then any patent licenses
      granted to You under this License for that Work shall terminate
      as of the date such litigation is filed.

   4. Redistribution. You may reproduce and distribute copies of the
      Work or Derivative Works thereof in any medium, with or without
      modifications, and in Source or Object form, provided that You
      meet the following conditions:

      (a) You must give any other recipients of the Work or
          Derivative Works a copy of this License; and

      (b) You must cause any modified files to carry prominent notices
          stating that You changed the files; and

      (c) You must retain, in the Source form of any Derivative Works
          that You distribute, all copyright, patent, trademark, and
          attribution notices from the Source form of the Work,
          excluding those notices that do not pertain to any part of
          the Derivative Works; and

      (d) If the Work includes a "NOTICE" text file as part of its
          distribution, then any Derivative Works that You distribute must
          include a readable copy of the attribution notices contained
          within such NOTICE file, excluding those notices that do not
          pertain to any part of the Derivative Works, in at least one
          of the following places: within a NOTICE text file distributed
          as part of the Derivative Works; within the Source form or
          documentation, if provided along with the Derivative Works; or,
          within a display generated by the Derivative Works, if and
          wherever such third-party notices normally appear. The contents
          of the NOTICE file are for informational purposes only and
          do not modify the License. You may add Your own attribution
          notices within Derivative Works that You distribute, alongside
          or as an addendum to the NOTICE text from the Work, provided
          that such additional attribution notices cannot be construed
          as modifying the License.

      You may add Your own copyright statement to Your modifications and
      may provide additional or different license terms and conditions
      for use, reproduction, or distribution of Your modifications, or
      for any such Derivative Works as a whole, provided Your use,
      reproduction, and distribution of the Work otherwise complies with
      the conditions stated in this License.

   5. Submission of Contributions. Unless You explicitly state otherwise,
      any Contribution intentionally submitted for inclusion in the Work
      by You to the Licensor shall be under the terms and conditions of
      this License, without any additional terms or conditions.
      Notwithstanding the above, nothing herein shall supersede or modify
      the terms of any separate license agreement you may have executed
      with Licensor regarding such Contributions.

   6. Trademarks. This License does not grant permission to use the trade
      names, trademarks, service marks, or product names of the Licensor,
      except as required for reasonable and customary use in describing the
      origin of the Work and reproducing the content of the NOTICE file.

   7. Disclaimer of Warranty. Unless required by applicable law or
      agreed to in writing, Licensor provides the Work (and each
      Contributor provides its Contributions) on an "AS IS" BASIS,
      WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
      implied, including, without limitation, any warranties or conditions
      of TITLE, NON-INFRINGEMENT, MERCHANTABILITY, or FITNESS FOR A
      PARTICULAR PURPOSE. You are solely responsible for determining the
      appropriateness of using or redistributing the Work and assume any
      risks associated with Your exercise of permissions under this License.

   8. Limitation of Liability. In no event and under no legal theory,
      whether in tort (including negligence), contract, or otherwise,
      unless required by applicable law (such as deliberate and grossly
      negligent acts) or agreed to in writing, shall any Contributor be
      liable to You for damages, including any direct, indirect, special,
      incidental, or consequential damages of any character arising as a
      result of this License or out of the use or inability to use the
      Work (including but not limited to damages for loss of goodwill,
      work stoppage, computer failure or malfunction, or any and all
      other commercial damages or losses), even if such Contributor
      has been advised of the possibility of such damages.

   9. Accepting Warranty or Additional Liability. While redistributing
      the Work or Derivative Works thereof, You may choose to offer,
      and charge a fee for, acceptance of support, warranty, indemnity,
      or other liability obligations and/or rights consistent with this
      License. However, in accepting such obligations, You may act only
      on Your own behalf and on Your sole responsibility, not on behalf
      of any other Contributor, and only if You agree to indemnify,
      defend, and hold each Contributor harmless for any liability
      incurred by, or claims asserted against, such Contributor by reason
      of your accepting any such warranty or additional liability.

   END OF TERMS AND CONDITIONS

   APPENDIX: How to apply the Apache License to your work.

      To apply the Apache License to your work, attach the following
      boilerplate notice, with the fields enclosed by brackets "{}"
      replaced with your own identifying information. (Don't include
      the brackets!)  The text should be enclosed in the appropriate
      comment syntax for the file format. We also recommend that a
      file or class name and description of purpose be included on the
      same "printed page" as the copyright notice for easier
      identification within third-party archives.

   Copyright {yyyy} {name of copyright owner}

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
~~~

### BSD-2-Clause

~~~
Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

  Redistributions of source code must retain the above copyright notice, this
  list of conditions and the following disclaimer.

  Redistributions in binary form must reproduce the above copyright notice, this
  list of conditions and the following disclaimer in the documentation and/or
  other materials provided with the distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR
ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
(INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON
ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
~~~

### BSD-3-Clause

~~~
Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

  * Redistributions of source code must retain the above copyright
    notice, this list of conditions and the following disclaimer.

  * Redistributions in binary form must reproduce the above copyright
    notice, this list of conditions and the following disclaimer in
    the documentation and/or other materials provided with the
    distribution.

  * Neither the name of the copyright holder nor the names of its
    contributors may be used to endorse or promote products derived
    from this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
~~~

### BSD-3-Clause (libopenmpt)

~~~
Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:
    * Redistributions of source code must retain the above copyright
      notice, this list of conditions and the following disclaimer.
    * Redistributions in binary form must reproduce the above copyright
      notice, this list of conditions and the following disclaimer in the
      documentation and/or other materials provided with the distribution.
    * Neither the name of the OpenMPT project nor the
      names of its contributors may be used to endorse or promote products
      derived from this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE
~~~

### BSD-3-Clause (prominentcolor)

~~~
Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
   * Neither the name of the copyright holder nor the names of its
contributors may be used to endorse or promote products derived from
this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
~~~

### Unlicense

~~~
This is free and unencumbered software released into the public domain.

Anyone is free to copy, modify, publish, use, compile, sell, or
distribute this software, either in source code form or as a compiled
binary, for any purpose, commercial or non-commercial, and by any
means.

In jurisdictions that recognize copyright laws, the author or authors
of this software dedicate any and all copyright interest in the
software to the public domain. We make this dedication for the benefit
of the public at large and to the detriment of our heirs and
successors. We intend this dedication to be an overt act of
relinquishment in perpetuity of all present and future rights to this
software under copyright law.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
IN NO EVENT SHALL THE AUTHORS BE LIABLE FOR ANY CLAIM, DAMAGES OR
OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
OTHER DEALINGS IN THE SOFTWARE.

For more information, please refer to <http://unlicense.org/>
~~~

### objc2 crates

`objc2`, `objc2-app-kit`, `objc2-foundation` and `block2` share one license file,
reproduced verbatim:

~~~
# License

The licensing of these crates is a bit complicated:
- The crates `objc2`, `block2`, `objc2-foundation` and `objc2-encode` are
  [currently][#23] licensed under [the MIT license][MIT].
- All other crates are trio-licensed under the [Zlib], [Apache-2.0] or [MIT]
  license, at your option.

Furthermore, the crates are (usually automatically) derived from Apple SDKs,
and that may have implications for licensing, see below for details.

[#23]: https://github.com/madsmtm/objc2/issues/23
[MIT]: https://opensource.org/license/MIT
[Zlib]: https://zlib.net/zlib_license.html
[Apache-2.0]: https://www.apache.org/licenses/LICENSE-2.0


## Apple SDKs

These crates are derived from Apple SDKs shipped with Xcode. You can obtain a
copy of the Xcode license at:

https://www.apple.com/legal/sla/docs/xcode.pdf

Or by typing `xcodebuild -license` in your terminal.

From reading the license, it is unclear whether distributing derived works
such as these crates are allowed?

But in any case, to practically use these crates, you will have to link, and
that only works when you have the correct Xcode SDK available to provide the
required `.tbd` files, which is why we choose to still use the normal SPDX
identifiers in the crates (Xcode is required to use the crates, and when using
Xcode you have already agreed to the Xcode license).
~~~

