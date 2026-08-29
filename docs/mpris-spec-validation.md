# MPRIS 规范合规验证

对应提交 `a3f5061`（fix: bring Linux MPRIS implementation closer to spec）及后续测试拆分。
验证环境为 **macOS 客户机**（无 D-Bus 会话总线），因此分为两层：可在 macOS 实测的部分
（纯逻辑单元测试、交叉编译、vet），以及必须依赖真实 D-Bus 总线、仅能理论推演的部分
（dbus-send / playerctl 交互）。

## 1. 验证环境与结论

| 验证项 | 方法 | 结果 |
|---|---|---|
| MPRIS 纯逻辑（playbackStatus / rootProps / playerProps / buildMetadata / validSetPosition） | macOS `go test ./internal/mediactl/` 实测 | ✅ 7 个测试全部通过 |
| macOS 全量回归 | `make test` | ✅ 全部包通过 |
| macOS 静态检查 | `make vet`、`gofmt -l` | ✅ 干净 |
| Linux 交叉编译 | `GOOS=linux go build ./internal/mediactl/...` | ✅ 通过 |
| Linux vet | `GOOS=linux go vet ./internal/mediactl/` | ✅ 干净（Seek 误报见 §6） |
| Linux 测试二进制 | `GOOS=linux go test -c` | ✅ 编译通过（aarch64 ELF，macOS 无法执行） |
| 真实 D-Bus 总线交互 | 需 Linux 机器 | ⏳ 见 §7 实测脚本 |

## 2. Root 接口属性可读（原缺陷：全部属性读取失败）

**规范**：`org.mpris.MediaPlayer2`（Root 接口）定义 9 个属性，客户端必须能通过
`org.freedesktop.DBus.Properties.Get/GetAll` 读取。

**修复前**：`mprisRoot` 用 `CanQuit() bool` 等方法签名暴露属性。godbus v5.2.2 的
`getMethods()`（export.go:84-89）只导出"最后一个返回值是 `*dbus.Error`"的方法，
这些返回裸值的方法被过滤；且 `rootObj` 从未 export `propsIface`，导致
`Properties.Get("org.mpris.MediaPlayer2", "Identity")` 路由到 playerObj 的 Get 后
返回 "unknown interface" 错误——**7 个属性全部不可读**，`CanSetFullscreen`/`Fullscreen`
甚至从未实现。

**修复后**：`rootProps()` 数据表（mpris.go）+ `mprisPlayerObj.Get/GetAll` 按
`iface` 分发到 `mprisRootIface` / `mprisPlayerIface` 两张属性表；`propsIface`
仍由 playerObj 导出（同一 path 同一 iface 只能有一个 handler）。

**验证**：`TestRootProps` 断言 9 个必需属性全部存在、能力旗标值正确（macOS 实测通过）。

## 3. SetPosition 参数校验

**规范**：`SetPosition(o: TrackId, x: Position)` —— TrackId 与当前 `mpris:trackid`
不匹配时 "ignored as stale"；Position < 0 或 > 曲长时 do nothing。

**修复后**：校验提取为纯函数 `validSetPosition(trackID, curID, pos, dur)`，仅在
通过时向 TUI 发送 `CmdSetPosition`。

**验证**：`TestValidSetPosition` 覆盖 6 种场景：范围内、恰在曲长、stale trackid、
负值、越界、未知曲长（允许任意非负值）（macOS 实测通过）。

## 4. Volume 属性端到端

**规范**：`Volume` 为 Read/Write double，0-1；`CanControl=true` 时 Set 应生效。

**修复前**：属性恒为硬编码 `1.0`，Set 一律 `PropertyReadOnly` —— `playerctl volume`
报错，控制中心音量条恒 100%。

**修复后**（完整链路）：

```
Properties.Set(Volume, 0.5)
  → mprisPlayerObj.Set 解析 double
  → Command{Type: CmdSetVolume, Volume: 0.5}（cmdChan）
  → root.go 转发 MediaCtlMsg
  → update.go handleMediaCtlCmd → m.Audio.SetVolume(0.5)
  → 下个 tick Update() 携带 Volume=0.5
  → PropertiesChanged(Volume) 广播
```

`PlayState.Volume` 默认零值即静音，不回落 1.0（`TestPlayerPropsVolumeZero` 守护）。

**验证**：`TestPlayerProps` / `TestPlayerPropsVolumeZero`（macOS 实测通过）；
跨进程链路需 Linux 实测。

## 5. Stopped 播放状态

**规范**：`PlaybackStatus` 枚举为 Playing / Paused / **Stopped**。

**修复前**：`!playing` 恒报 "Paused"，无曲目时（每 tick 仍会 Update）语义错误。

**修复后**：`PlayState.HasTrack`（TUI `buildPlayState` 填 `m.Audio.Player != nil`），
`playbackStatus(playing, hasTrack)` 三态；`Update()` 在 HasTrack 翻转时也发
`PropertiesChanged(PlaybackStatus)`。

**验证**：`TestPlaybackStatus` 4 用例（macOS 实测通过）。

## 6. 能力旗标诚实化 + vet 误报

| 项目 | 修复 | 依据 |
|---|---|---|
| `CanQuit` | `false`（无退出路径，Quit 保持 no-op） | 规范：客户端仅在 true 时调用 |
| `SupportedUriSchemes` | 空数组，`OpenUri` 返回 `NotSupported` | 规范：schemes 为空时可不必实现 |
| `CanSetFullscreen` / `Fullscreen` | `false` | Root 接口必需属性，此前缺失 |
| go vet Seek 警告 | 误报，已加 `//nolint:stdmethods` 注释 | vet stdmethods 分析器把 D-Bus 方法 `Seek(offset int64) *dbus.Error` 误匹配为 `io.Seeker` 的 `(int64, int) (int64, error)`；godbus 要求 D-Bus 方法末参返回 `*dbus.Error` |

## 7. Linux 实测脚本（提交前建议跑一遍）

```bash
# 属性读取（修复前 Identity 读取失败，修复后返回 "NeoViolet"）
dbus-send --session --print-reply --dest=org.mpris.MediaPlayer2.neoviolet \
  /org/mpris/MediaPlayer2 org.freedesktop.DBus.Properties.Get \
  string:"org.mpris.MediaPlayer2" string:"Identity"

# 音量写入（修复前 PropertyReadOnly，修复后调整 TUI 音量）
dbus-send --session --print-reply --dest=org.mpris.MediaPlayer2.neoviolet \
  /org/mpris/MediaPlayer2 org.freedesktop.DBus.Properties.Set \
  string:"org.mpris.MediaPlayer2.Player" string:"Volume" variant:double:0.5

# 播放状态三态：播放中=Playing、暂停=Paused、无曲目=Stopped
dbus-send --session --print-reply --dest=org.mpris.MediaPlayer2.neoviolet \
  /org/mpris/MediaPlayer2 org.freedesktop.DBus.Properties.Get \
  string:"org.mpris.MediaPlayer2.Player" string:"PlaybackStatus"

# SetPosition：正确 trackid（换曲后旧 trackid 应被忽略）
dbus-send --session --print-reply --dest=org.mpris.MediaPlayer2.neoviolet \
  /org/mpris/MediaPlayer2 org.mpris.MediaPlayer2.Player.SetPosition \
  objpath:"/neoviolet/track/1" int64:30000000

# OpenUri 应返回 NotSupported
dbus-send --session --print-reply --dest=org.mpris.MediaPlayer2.neoviolet \
  /org/mpris/MediaPlayer2 org.mpris.MediaPlayer2.Player.OpenUri \
  string:"file:///tmp/test.mp3"

# playerctl 冒烟：status / metadata / identity / volume
playerctl status && playerctl metadata && playerctl identity && playerctl volume 0.5
```
