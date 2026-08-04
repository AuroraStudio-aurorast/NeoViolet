# 网络获取歌词（LRCLIB）功能实现计划

> **面向 AI 代理的工作者：** 此计划处于 review 状态，尚未批准实现。带 `[待定]` 标记的决策点需项目所有者确认后方可开始编码。批准后按任务分解顺序实现，每任务完成后运行对应测试。

**目标：** 本地无歌词时，从可配置的 LRCLIB 兼容服务在线获取歌词，接入现有歌词显示/滚动/IPC 管线；在线歌词作为 `FormatPriority` 的最后一个 fallback；遵守 LRCLIB 限流契约；严格验证候选匹配度；抑制重复请求。

**架构：** 新增 `internal/lyrics/fetch/` 包（HTTP 客户端 + 内容安全校验 + 匹配验证 + 会话级缓存 + 高层获取逻辑），在 `handleAudioLoaded` 本地查找失败后作为异步回退接入；`"online"` 作为 `FormatPriority` 伪条目；429 冷却语义由缓存条目承载。

**技术栈：** Go 标准库 `net/http`、`encoding/json`、`golang.org/x/text/unicode/norm`（新增依赖，Unicode 规范化）；Bubble Tea `tea.Cmd` 异步；`internal/version` 包承载构建期版本号（Makefile ldflags 注入）。

---

## 0. 研究结论摘要

**项目现状（歌词管线）**

- `internal/lyrics/`：`LyricParser` 接口（`FindSidecar` + `Parse`），全部本地文件导向（embedded/lrc/ttml/qrc/yrc/eslrc/lys/srt/smi）；`FindAndParse`/`FindAndParsePreferred` 按 `FormatPriority` 遍历，**未知格式名安全跳过**（`parserMap[name]` 不存在 → `continue`）。
- `handleAudioLoaded`（`internal/ui/update.go:265`）：曲目加载后 `FindAndParsePreferred`，无本地歌词静默跳过（不报错、无回退）。
- 命令：`:lrc on/off/switch <fmt>/agent/desktop`（`internal/ui/update_keyboard.go:460`）。
- 配置：`LyricsConfig{Enabled, ScrollSpeed, FormatPriority}`；`Load()` 内联校验 + `Normalize()` 兜底（返回 `true` 触发 `Save()`）。
- `syntheticFormats`（`internal/audio/player.go:671`，**未导出**）：`.mid/.midi/.mod/.xm/.s3m/.it/.stm/.nst/.wow/.ult/.669/.mtm/.mdl/.far/.ptm/.okt/.dmf/.dbm/.digi/.imf/.j2b/.mo3/.umx/.gdm` + openmpt 扩展格式。
- 元数据：`Player.Title()/Artist()/Album()/Duration()`；net/http 先例 `internal/audio/remote.go`（30s timeout client）。
- 版本号：`cmd.Version`，Makefile ldflags 注入（`git describe`，默认 `"dev"`）。
- GUI 桌面歌词走同一 `LyricsData` → IPC 流，**TUI 侧获取后 GUI 自动受益，无需改 Rust 侧**。
- **项目无现成字符串匹配/规范化工具**（需新建）。

**LRCLIB API 契约**（官方文档 + 服务端源码核验）

- 免 API key；**必须设置 `User-Agent`**（应用名 + 版本 + 主页链接）。
- **429 限流**：服务端返回 `429` + **`Retry-After` header（秒）**；客户端**必须**遵守，忽略继续请求可能被临时封禁。429 响应体：`{"code":429,"name":"TooManyRequests","message":"Rate limit exceeded"}`。
- **请求节流**：请求应顺序发送，相邻请求间隔 200–500ms。
- `GET /api/get?track_name=&artist_name=&album_name=&duration=`：精确匹配，返回单条；404 = 无匹配（`{"code":404,"name":"TrackNotFound"}`）；**duration 是硬条件**（±2s 内才给结果）。
- `GET /api/search?q=...` 或 `track_name/artist_name/album_name` 组合：数组，最多 20 条，无分页，**候选需自行验证**。
- `GET /api/get/{id}`：按 ID 取记录。
- 响应字段：`id, name, trackName, artistName, albumName, duration, instrumental, plainLyrics, syncedLyrics, lyricsfile`；**`syncedLyrics` 即标准 LRC 文本**。
- **文档明示**：404 的曲目可能被后台服务补录、后续请求可能变得可用 → negative cache 只做会话级。
- 服务端开源（axum + tower-http，无限流中间件）→ **可自建兼容实例**，Base URL 可配置有现实依据。

---

## 1. 目标与范围

**目标：** 本地无歌词时，从可配置的 LRCLIB 兼容服务获取歌词并接入现有歌词管线；在线歌词为优先级列表最后一个 fallback；遵守限流契约 + 分级安全防护 + 严格候选验证 + 重复请求抑制；提供手动命令；可配置。

**范围内：**

- 新包 `internal/lyrics/fetch/`：HTTP 客户端 + 内容安全校验 + 匹配验证 + 双层缓存（会话级内存 + 持久化磁盘）+ 高层获取逻辑（纯函数、可单测）。
- `internal/version` 包 + Makefile 版本注入，供 UA 使用。
- 配置：`LyricsConfig.Fetch` 子结构（BaseURL/开关/超时/安全档位/TLS 豁免）+ `FormatPriority` 支持 `"online"` 伪条目（默认尾位）。
- TUI：自动获取（异步 `tea.Cmd`）+ `:lrc switch online` 手动获取（复用现有 switch 语法）；**全部 UI 文案英语**（v0.14 用户指示）
- 会话级请求缓存（positive + negative + 429 冷却，内存层）。
- **持久化磁盘缓存（歌词本地保存，在线获取优先取缓存，§2.11）**。
- 合成格式（MIDI/Tracker）自动排除。
- 严格匹配验证（歌曲名 + 歌手，多歌手拆分，编码规范化）。
- plain-only（无同步歌词）候选自动放弃。
- **竞态防护（v0.17）**：异步 fetch 结果携带曲目签名，切歌后旧结果不覆盖当前歌词（§2.12）。
- **磁盘过期清理（v0.17）**：启动时扫描 `CacheDir()` 删除已过期缓存文件（§2.11 ⑤）。
- **fetching 反馈（v0.17）**：获取中显示 `[Fetching lyrics...]`（§2.12）。
- **SSRF 私网警告（v0.17）**：`base_url` 指向私网/本地地址时 Normalize 仅警告不阻止（§2.3）。

**范围外（v1 不做）：** 发布歌词、flag 举报、多结果选择 UI、GUI 侧修改、缓存容量上限管理（LRU/配额）、**翻译歌词**（非标准标签解析时忽略不报错）、**下一首预取**、**多 provider 故障转移**（单 base_url）、**缓存清理命令**（指令集已精简 v0.16，清理靠启动扫描）。

---

## 2. 架构设计

### 2.1 包结构

```
internal/version/
  version.go          # var Version = "dev"（ldflags 注入）+ UserAgent()

internal/lyrics/fetch/
  client.go           # HTTP 客户端：Get/Search/GetByID
                      # URL 构造、User-Agent、超时、跳转限制、429 重试（Retry-After）、前置节流、传输/响应层安全
  validate.go         # 内容层安全校验（strict/basic 档，纯函数）
  match.go            # Normalize / SplitArtists / MatchTrack / MatchArtist（纯函数）
  types.go            # Track 响应结构体 + APIError{Code,Name,Message}
  cache.go            # 会话级请求缓存（positive + negative + 429 冷却，并发安全，内存层）
  cachefile.go        # 持久化磁盘缓存（读写 JSON 缓存文件 + TTL 判定，§2.11）
  ratelimit.go        # provider 级 429 冷却持久化（读写 ratelimit.json，§2.7b）
  fetch.go            # FetchLyrics(ctx, meta, cfg)：磁盘缓存查 → Get → Search → 匹配验证 → 解析 → 写回缓存
  *_test.go

internal/audio/       # 修改：导出合成格式判断（见 §2.8）
internal/config/      # 修改：LyricsConfig.Fetch 子结构 + "online" 默认尾位（见 §2.3）
internal/ui/          # 修改：自动获取接入 + 指令（见 §2.5）
Makefile              # 修改：version 包 ldflags 注入（见 §2.4）
go.mod                # 新增 golang.org/x/text（norm 子包）
```

### 2.2 与现有体系的关系

不实现 `LyricParser` 接口（文件导向，`FindSidecar` 语义不匹配网络来源）。`"online"` 是 `FormatPriority` 中的伪条目：`FindAndParsePreferred` 遍历时自动跳过（未知名安全），UI 层在本地遍历前过滤它、本地失败后再走在线路径。

`LyricsData` 复用现有类型：`Format: "lrclib"`，`Path: "lrclib://<id>"`，Lines/时间戳语义与本地解析一致 → 现有显示/滚动/IPC 逻辑零改动。

### 2.3 配置设计（Base URL 可配置 + D8/D9 可配置化）

```go
// internal/config/config.go

type LyricsConfig struct {
 Enabled        bool     `json:"enabled"`
 ScrollSpeed    int      `json:"scroll_speed"`
 FormatPriority []string `json:"format_priority"` // 可含 "online"（默认尾位）
 Fetch          LyricsFetchConfig `json:"fetch"`
}

// LyricsFetchConfig 控制在线歌词获取（LRCLIB 兼容 API）。
type LyricsFetchConfig struct {
 Enabled     bool   `json:"enabled"`      // 在线获取总开关（自动获取必需）
 BaseURL     string `json:"base_url"`     // API 根地址；空 = 默认 https://lrclib.net
 Timeout     int    `json:"timeout"`      // 单请求超时（秒）；默认 10
 Security    string `json:"security"`     // "strict" | "basic"；默认 "strict"
 InsecureTLS bool   `json:"insecure_tls"` // 跳过 TLS 证书校验；默认 false
}

const (
 DefaultBaseURL = "https://lrclib.net"
 DefaultFetchTimeout = 10
 DefaultSecurity = "strict"
)
```

**默认值**（`DefaultConfig()`）：

```go
Lyrics: LyricsConfig{
 Enabled:        true,
 ScrollSpeed:    6,
 FormatPriority: []string{"embedded", "lrc", "ttml", "qrc", "yrc", "eslrc", "lys", "online"},
 Fetch: LyricsFetchConfig{
  Enabled:  true,
  Timeout:  DefaultFetchTimeout,
  Security: DefaultSecurity,
 },
},
```

- `BaseURL` 留空 = 使用代码常量（默认指向官方，不持久化进配置；用户显式配置才写入）。
- **旧配置兼容**：`json.Unmarshal` 到 `DefaultConfig()` 上，旧文件无 `"fetch"` 字段 → 保持默认值，零迁移成本。

**Normalize 校验**（合并进现有 `Normalize()`，返回 `true` 触发 Save）：

```go
func (c *Config) Normalize() bool {
 orig := *c
 // ...existing volume 逻辑不变...

 f := &c.Lyrics.Fetch

 // base_url: 仅接受 http/https + 非空 host；去尾斜杠；非法 → 回退默认（空）
 if f.BaseURL != "" {
  u, err := url.Parse(f.BaseURL)
  if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
   logger.Warn("invalid lyrics fetch base_url, falling back to default", "url", f.BaseURL)
   f.BaseURL = ""
  } else {
   f.BaseURL = strings.TrimRight(f.BaseURL, "/")
  }
 }

 // timeout: <=0 → 默认
 if f.Timeout <= 0 {
  f.Timeout = DefaultFetchTimeout
 }

 // security: 仅 "strict"/"basic"
 if f.Security != "strict" && f.Security != "basic" {
  f.Security = DefaultSecurity
 }

 // insecure_tls: bool 无需校验，但开启时告警（审计层）
 if f.InsecureTLS {
  logger.Warn("lyrics fetch: TLS certificate verification disabled")
 }

 // http（非 localhost）明文传输告警
 if u, err := url.Parse(f.BaseURL); err == nil && u.Scheme == "http" && !isLocalHost(u.Hostname()) {
  logger.Warn("lyrics fetch over plain http (not recommended)", "url", f.BaseURL)
 }

 // 私网/本地地址告警（v0.17，SSRF 面）：误配指向内网服务时曲目元数据会发送过去；
 // 不阻止（自建实例可能就在内网），仅日志暴露问题。
 if u, err := url.Parse(f.BaseURL); err == nil && isPrivateHost(u.Hostname()) {
  logger.Warn("lyrics fetch base_url points to private/local address", "url", f.BaseURL)
 }

 volumeChanged := c.DefaultVolume != orig.DefaultVolume
 fetchChanged := f.Enabled != orig.Lyrics.Fetch.Enabled ||
  f.BaseURL != orig.Lyrics.Fetch.BaseURL ||
  f.Timeout != orig.Lyrics.Fetch.Timeout ||
  f.Security != orig.Lyrics.Fetch.Security ||
  f.InsecureTLS != orig.Lyrics.Fetch.InsecureTLS
 return volumeChanged || fetchChanged
}

// isLocalHost 判断 hostname 是否为 localhost/127.x/::1（自建实例允许明文 http）
func isLocalHost(host string) bool {
 return host == "localhost" || strings.HasPrefix(host, "127.") || host == "::1"
}

// isPrivateHost 判断 hostname 是否为私网/回环/链路本地 IP 或本地域名后缀（v0.17）。
// 仅告警用，不阻止请求；回环地址在 isLocalHost 中也已涵盖。
func isPrivateHost(host string) bool {
 if ip := net.ParseIP(host); ip != nil {
  return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
 }
 host = strings.ToLower(strings.TrimSuffix(host, "."))
 for _, suf := range []string{".local", ".internal", ".lan", ".home", ".localhost"} {
  if strings.HasSuffix(host, suf) {
   return true
  }
 }
 return host == "localhost"
}
```

**JSON 形态**（用户可见）：

```json
{
  "lyrics": {
    "enabled": true,
    "scroll_speed": 6,
    "format_priority": ["embedded", "lrc", "ttml", "qrc", "yrc", "eslrc", "lys", "online"],
    "fetch": {
      "enabled": true,
      "base_url": "https://lrclib.net",
      "timeout": 10,
      "security": "strict",
      "insecure_tls": false
    }
  }
}
```

### 2.4 User-Agent 与版本注入

**UA 值**（已确认）：

```
NEOVIOLET v1.2.3 (https://github.com/AuroraStudio-aurorast/NeoViolet)
```

dev 时：`NEOVIOLET vdev (https://github.com/AuroraStudio-aurorast/NeoViolet)`。

```go
// internal/version/version.go
package version

// Version is set via ldflags at build time; "dev" when built without ldflags.
var Version = "dev"

// UserAgent returns the LRCLIB-compliant User-Agent string.
func UserAgent() string {
 return "NEOVIOLET v" + Version + " (https://github.com/AuroraStudio-aurorast/NeoViolet)"
}
```

**Makefile 双注入**（`cmd.Version` 兼容现有 `version` 子命令；`internal/version.Version` 供 fetch 使用；两处同一 `$(VERSION)`）：

```makefile
LDFLAGS = -s -w \
  -X github.com/AuroraStudio-aurorast/neoviolet/cmd/neoviolet/cmd.Version=$(VERSION) \
  -X github.com/AuroraStudio-aurorast/neoviolet/internal/version.Version=$(VERSION)
```

### 2.5 数据流与 TUI 集成

```
handleAudioLoaded
  ├─ localPriority = FormatPriority 去掉 "online"
  ├─ FindAndParsePreferred(path, localPriority, preferred) 命中? ──是──→ 挂载 Lyrics，结束
  │
  └─ 否 → 自动在线获取前置条件（全部满足才继续）：
       ① Fetch.Enabled == true
       ② FormatPriority 含 "online"
       ③ Title、Artist 非空（URL 流/无 tag 跳过）
       ④ 非合成格式（MIDI/Tracker，见 §2.8）
       │ 是
       ▼
  provider 冷却检查 rateLimit.Blocked(baseURL)（持久化，§2.7b）
    ├─ 冷却中 → 静默跳过（0 请求，无论哪首歌，重启后仍生效）
    └─ 可请求 → 继续 ↓
  fetchCache.Lookup(sig)     // 内存层；sig = title|artist|album|duration 归一化
    ├─ CacheFound       → 直接复用挂载（0 请求）
    ├─ CacheNotFound    → 静默跳过（0 请求）
    ├─ CacheRateLimited → （会话内残留）冷却未到 → 静默跳过；已过 → 视为 miss 继续
    └─ miss → 磁盘缓存 cachefile.Load(sig)（§2.11）:
        ├─ hit（found，未过期）→ 载入内存 CacheFound → 挂载（0 请求）
        ├─ hit（notfound，未过期）→ 载入内存 CacheNotFound → 静默（0 请求）
        └─ miss / 过期 / 损坏 → 置 CachePending → tea.Cmd → FetchLyrics(ctx, meta, cfg):
            ├─ client.Get() 精确匹配:
            │   ├─ 200 → 内容校验 → 解析 → 写回内存+磁盘 → FetchLyricsResultMsg → 挂载
            │   ├─ 404 → 降级 /api/search（§2.6 匹配验证）
            │   └─ 429 → Retry-After 冷却重试（§2.7）
            └─ client.Search() 兜底:
            ├─ 候选逐个 MatchTrack + MatchArtist 严格验证（§2.6）
            │   ├─ 首个通过且 syncedLyrics 非空 → 采用 → cache.Store(CacheFound)
            │   └─ 全部未通过 / 仅 plain → 放弃 → cache.Store(CacheNotFound)
            └─ 空 / 请求失败 → cache.Store(CacheNotFound)
                          ▼
  FetchLyricsResultMsg → 挂载 m.Audio.Lyrics（成功）或 logger 记录（失败，UI 静默）
```

### 2.6 严格匹配验证（D2 决策）

**目的**：`/api/search` 候选不能自动取第一条——必须确认"候选 = 本曲"才采用。

```go
// internal/lyrics/fetch/match.go（纯函数，无 IO）

// Normalize 编码规范化：大小写、全角/半角、Unicode 兼容字符、变音符号剥离、
// 标点→空格、撇号移除、空白折叠、剥离括号块与 "feat." 后缀。
func Normalize(s string) string

// SplitArtists 多歌手拆分：按分隔符拆分并规范化、去重、去空。
// 分隔符：, ; 、 / \x00 & feat. featuring ft. with（& 拆分理由见 §2.6b 真实数据验证）
// 注意：and 不拆分（"Simon and Garfunkel" 保持整体，"/" 已覆盖多数 case）
func SplitArtists(s string) []string

// MatchTrack 歌曲名严格匹配：Normalize 后精确相等。
func MatchTrack(reqTitle, candTitle string) bool

// MatchArtist 歌手匹配：
//   strict=true（默认）：SplitArtists(req) ⊆ SplitArtists(cand)
//   strict=false（平衡档）：两集合交集非空
func MatchArtist(reqArtists, candArtists string, strict bool) bool
```

**① 编码规范化（Normalize）**——解决编码问题（借鉴 lrcget `prepare_input`/`prepare_search_input`，§2.6b）：

| 处理 | 说明 |
|---|---|
| `unicode.ToLower`（逐 rune） | 全 Unicode 大小写 |
| `norm.NFKC`（`golang.org/x/text/unicode/norm`） | 全角→半角（`Ｆｅｅｌ`→`Feel`）、兼容字符、组合字符统一 |
| **变音符号剥离**（NFKD + 过滤 `unicode.Mn`） | `Café`→`cafe`、`Für Elise`→`fur elise`（lrcget 用 `secular::lower_lay_string`；Go 侧用 `norm.NFKD` 分解后丢弃组合标记，等价） |
| **标点→空格** | 替换 `[\`~!@#$%^&*()_|+=?;:.,<>{}[\]\\/]` 为空格（lrcget `prepare_input` 同款字符集） |
| **撇号移除** | `'`/`’` 直接删除（不换空格）：`don't`→`dont`、`I’m`→`im` |
| `strings.Fields` 重连 | 折叠空白（多空格/tab/NBSP → 单空格） |
| 剥离括号块 | `(feat. X)`、`(Live)`、`(Remastered)`、`[xx]` → 去掉（lrcget `prepare_search_input` 移除**全部**括号内容；本项目剥离括号块，见 §2.6b） |
| 剥离 `feat./ft./featuring` 后缀 | "Song feat. Artist" → "Song"（歌手已单独比较） |
| Trim 首尾空白 | 兜底 |

**② 多歌手拆分（SplitArtists）**——解决多歌手问题：

| 分隔符 | 例 |
|---|---|
| `,` / `;` / `、` / `/` | "Taylor Swift; Bon Iver" → [taylor swift, bon iver] |
| `feat.` / `featuring` / `ft.` / `with` | "Taylor Swift feat. Bon Iver" → [taylor swift, bon iver] |
| `&` | **拆**（"Simon & Garfunkel" → [simon, garfunkel]；"Porter Robinson & Madeon" → [porter robinson, madeon]）——D13 已定，理由见 §2.6b 真实数据验证 |
| `\x00`（NUL） | 拆（LRCLIB 真实数据出现 `'Porter Robinson\x00Madeon'`） |
| `and` | **不拆**（"Simon and Garfunkel" 保持整体） |

**③ 匹配规则**：

```
MatchTrack(req, cand)  = Normalize(req) == Normalize(cand)
MatchArtist(req, cand) = strict ? (SplitArtists(req) ⊆ SplitArtists(cand))
                                : (交集非空)
```

- **歌曲名：严格相等**（规范化后）。不做模糊/包含——同名歌靠 artist 区分。
- **歌手（严格档，默认）**：请求的全部歌手必须在候选歌手集中出现（候选可有额外歌手，如合辑）。"Taylor Swift; Bon Iver" 只匹配到同时标注两位的候选；候选只标 "Taylor Swift" → **拒绝**。
- **歌手（平衡档）**：交集非空——适配 `Security=basic` 或用户放宽时（决策点 D14）。
- **候选选取（多条通过时）**：通过 MatchTrack+MatchArtist 的候选按 `|candDuration - reqDuration|` 升序取**最小者**（实测：`Porter Robinson/Madeon - Shelter` 通过验证的候选有 Δ0s/Δ2s/Δ29s 多条，见 §2.6b；album 不参与排序）。

**④ plain-only 放弃（D2 决策）**：

- **自动获取路径：候选只有 `plainLyrics`（无 `syncedLyrics`）→ 一律放弃**（不采用、不展示、不计入挂载）。
- 理由：在线歌词主要用途是同步滚动显示；纯文本无时间轴无法联动播放进度。
- 放弃计入 negative cache（`CacheNotFound`），避免反复请求同一条 plain-only 记录。
- 手动命令例外见决策点 D15（默认：也放弃，保持一致性）。

### 2.6b 参考 lrcget 的匹配思路（研究结论）

lrcget（LRCLIB 官方客户端，`github.com/tranxuanthang/lrcget`）源码核验，其匹配思路与本方案的对齐与差异：

| lrcget 做法 | 本项目采用 | 说明 |
|---|---|---|
| `prepare_input`：变音剥离 + 标点→空格 + 撇号移除 + 小写 + 折叠空白 | **采用**（纳入 §2.6 ① Normalize） | Go 侧用 `norm.NFKD` + 过滤 `unicode.Mn` 等价实现 `secular::lower_lay_string`；标点字符集与 lrcget 完全一致 |
| `prepare_search_input`：移除**全部**括号内容再规范化 | **部分采用**：剥离括号块（含非尾部），但保留括号内信息不用于匹配 | lrcget 用于生成 FTS 查询词（信息丢失可接受）；本项目做候选验证需保留区分度 |
| `/api/get` 精确匹配：直接信任服务端结果，无二次验证 | **不采用**：命中后仍走内容层校验（§2.9 层 3）+ 时长偏差检查 | 本项目安全五层防护更强（§2.9） |
| `/api/search` 结果：**无自动匹配算法，人工选择** | **部分采用**：自动路径仍严格验证（§2.6），但验证失败时**不自动降级采用**，与 lrcget 一致保持保守 | 本项目无选择 UI（范围外），保守拒绝是安全的默认 |
| `plain_lyrics` 缺失时从 `synced_lyrics` 剥离时间戳生成 | **采用**：解析后若 plain 为空，可用 strip-timestamp 从 synced 派生（未来展示静态歌词用，v1 仅存储） | lrcget `strip_timestamp` 用正则 `(?m)^\[[^\]]*\]\s*` 逐行去时间戳 |
| 下载结果构建成 **lyricsfile**（LRCLIB 原生 YAML）存入本地库 | **v1 不采用**：本项目缓存存 JSON（§2.11），字段对齐即可 | lrcget 是歌词库管理器需持久化 lyricsfile；本项目缓存仅服务本机播放，JSON 足够 |

**对本方案决策点的影响**：

- **D13（`&`/`and` 拆分）已变更——拆 `&`**（理由见下「真实数据验证」）。lrcget 将 `&` 替换为空格（实质参与分隔），本项目同步。
- **D14（歌手匹配档位）**：lrcget 不做歌手集比较（仅依赖服务端 /api/get 的元组匹配）。本项目保留严格档默认；无新证据改变该决策。
- **变音剥离默认开启**：lrcget 证实这是官方客户端默认行为，本项目同步默认开启（无需新决策点）。

**真实数据验证（Porter Robinson/Madeon - Shelter，2026-08-04 实测）**：

本地文件元数据（dhowden/tag 解析，与播放器一致）：`Title="Shelter"`、`Artist="Porter Robinson/Madeon"`（**`/` 分隔**）、`Album="Shelter"`、`Duration=219s`。

- **`/api/get` 用原样 artist（带 `/`）直接命中** → track **id 887633**（服务端记录 artist=`"Porter Robinson & Madeon"`，`&` 分隔）；单独传 `Madeon` → 404。
- **`/api/search` 20 个候选中不含 887633**（两索引独立）；artist 写法 7 种：`&`×11、`,`×4、`;`×2、`/`、**NUL（`\x00`）分隔**、小写空格版。
- **规范化模拟（本项目 §2.6 Normalize + lrcget 字符集）**：拆 `&` 时 `"Porter Robinson & Madeon"` 候选 → `[porter robinson, madeon]` ✓ 通过；不拆时 → `'porter robinson madeon'` 整体 ✗ 拒绝。**不拆 `&` 会拒绝主候选写法（11/20）**，/api/get 404 降级时必然误拒。
- **duration 二次筛选必要**：通过验证的候选可能多条（`id=34503480` Δ0s 但 album 不同；`id=20845212` album=Shelter Δ2s；`id=36849125` album=Shelter Δ29s）——需按 `|Δduration|` 最小者选取，而非首个通过者。

**由此新增两项规则**：

1. `SplitArtists` 分隔符增加 `\x00`（NUL）——真实数据出现 `'Porter Robinson\x00Madeon'`。
2. **候选选取加 duration 二级排序**：通过 MatchTrack+MatchArtist 的候选按 `|candDuration - reqDuration|` 升序取最小者（与 album 无关；仍不采用失败候选）。

### 2.7 429 限流与冷却（用户指示）

**契约**：429 的 `Retry-After`（秒）**必须遵守**；请求间隔 200–500ms 前置节流。

**算法（`client.Get` 内部）**：

```go
// Client 是单个 provider（baseURL）的 HTTP 客户端。
// 429 冷却与前置节流均绑定本 client 的 baseURL——不同 provider 各自独立，互不影响。
type Client struct {
 baseURL   string            // 有效 BaseURL（配置或默认，已规范化，§2.3）
 httpClient *http.Client
 rateLimit *RateLimit        // provider 级冷却（§2.7b）
 lastReqAt time.Time         // 前置节流：本 provider 上一次请求时间
}

const maxRateLimitRetries = 1 // 429 重试上限：1 次额外尝试

func (c *Client) do(req) (*http.Response, error) {
 // 前置节流：与上一次请求至少间隔 200ms（文档 Request Throttling）；
 // 每 client 独立计时 → 换 provider 不互相拖慢
 c.throttle()

 resp, err := c.httpClient.Do(req)
 if err != nil { return resp, err }

 if resp.StatusCode == http.StatusTooManyRequests {
  wait := parseRetryAfter(resp.Header.Get("Retry-After")) // 秒；非法/缺失 → 兜底 2s
  logger.Warn("lrclib rate limited", "base_url", c.baseURL, "retry_after", wait, "track", trackName)
  resp.Body.Close()

  // 等待后重试一次（可被 ctx 取消）
  select {
  case <-time.After(time.Duration(wait) * time.Second):
  case <-ctx.Done(): return nil, ctx.Err()
  }

  resp2, err2 := c.httpClient.Do(req)
  if err2 != nil { return nil, err2 }
  if resp2.StatusCode == http.StatusTooManyRequests {
   wait2 := parseRetryAfter(resp2.Header.Get("Retry-After")) // 秒
   resp2.Body.Close()
   // 静默保留新倒计时并持久化——冷却 key 绑定本 client 的 baseURL，
   // 不写默认/全局值（§2.7b）：仅本 provider 被抑制，其他实例不受影响
   c.rateLimit.SetRateLimited(c.baseURL, time.Duration(wait2)*time.Second)
   return nil, &RateLimitError{RetryAfter: time.Duration(wait2) * time.Second}
  }
  return resp2, nil
 }
 return resp, nil
}
```

**接线保证（v0.13 强化，用户指示）**：

- **冷却 key 恒等于实际发请求的 baseURL**：`Client` 构造时绑定 `baseURL`（`NewClient(effectiveBaseURL, cfg)`），429 落库键即 `c.baseURL`；绝不使用默认值/全局变量替代。
- **切 provider 零误伤**：用户把 `base_url` 从 A 改为 B 后，A 的冷却条目仍在 `ratelimit.json` 但 `Blocked(B)` 不受影响；B 首次请求走完整流程。
- **换回旧 provider 继续冷却**：改回 A 时 `Blocked(A)` 正确返回剩余冷却（持久化仍在该键下）。
- **前置节流 per-provider**：`lastReqAt` 在 client 实例上，A/B 各自计时，避免互相拖慢。

**冷却语义（用户指示要点 + 持久化修订）**：

| 用户指示 | 实现 |
|---|---|
| 遵守 Retry-After（秒） | 每次 429 都用最新 `Retry-After` 值作为等待/冷却时长 |
| 重试一次；失败后不再自动尝试 | `maxRateLimitRetries = 1`；仍 429 → 返回 `RateLimitError`，**不**再次自动重试 |
| "除非用户再次尝试启用在线歌词，再尝试" | 自动路径冷却中直接跳过（provider 级）；用户手动触发（`:lrc online on` / `:lrc switch online`）才重新走请求流程 |
| 再次请求后也是 429 → 静默保留倒计时 | `rateLimit.SetRateLimited(baseURL, retryAfter)` **持久化到磁盘**（§2.7b）；不向 UI 报错（仅 logger） |
| 用户再次加载在线歌词时，需等重试时间到后再请求 | 请求前查 provider 冷却：未到 → **不发起请求**，提示剩余秒数（或静默，决策点 D16）；已过 → 正常请求 |
| **（v0.12 新增）重启后冷却信息保留** | provider 级冷却持久化于 `ratelimit.json`（§2.7b），重启加载后仍能判断是否可请求 |

```go
// internal/lyrics/fetch/cache.go（会话级内存缓存，per-sig）
// 注意：429 冷却已从 per-sig 会话级升级为 provider 级持久化（§2.7b）。
// 本结构保留 CacheRateLimited 仅用于：同一会话内、冷却期间的快速标记与 UI 提示；
// 权威冷却判断一律走 rateLimit.Blocked(baseURL)（§2.7b），内存态只是它的镜像。

type CacheState int

const (
 CacheFound CacheState = iota
 CacheNotFound
 CacheRateLimited // 会话内镜像：rateLimit.Blocked(baseURL) 的本地副本
 CachePending     // 请求进行中：防并发重复
)

type cacheEntry struct {
 state            CacheState
 data             *lyrics.LyricsData // CacheFound 时有效
 at               time.Time          // 写入时间
 rateLimitedUntil time.Time          // CacheRateLimited 时：镜像 provider 冷却截止
}

type Cache struct {
 mu    sync.Mutex
 items map[string]cacheEntry
}

// Lookup 返回状态与数据。
// 冷却判定不依赖 per-sig：先查 rateLimit.Blocked(baseURL)，再查本缓存。
func (c *Cache) Lookup(sig string) (CacheState, *lyrics.LyricsData, bool)

func (c *Cache) Store(sig string, state CacheState, data *lyrics.LyricsData)
func (c *Cache) StoreRateLimited(sig string, retryAfter time.Duration) // 镜像写入
func (c *Cache) Clear(sig string) // 手动强制时清 pending/negative，允许重试
```

**`RateLimitError` 错误语义**（`fetch.go` 对外）：

```go
var ErrNotFound    = errors.New("lrclib: track not found")     // 404 / 验证未通过 → 静默
var ErrNoMatch     = errors.New("lrclib: no matching candidate") // Search 全部错配
var ErrRateLimited = errors.New("lrclib: rate limited")         // 重试耗尽 → 仅 logger
var ErrOffline     = errors.New("lrclib: no internet connection") // 离线 → 手动提示，自动静默（v0.15）

type RateLimitError struct {
 RetryAfter time.Duration // 服务端指示的冷却时长
}
func (e *RateLimitError) Error() string { return "lrclib: rate limited, retry in " + e.RetryAfter.String() }
func (e *RateLimitError) Unwrap() error { return ErrRateLimited }
```

**离线检测（v0.15 用户指示）**：

- **目标**：用户离线时，`:lrc switch online` 等请求**不等待超时**，直接提示。
- **识别**：连接层错误即时归类为 `ErrOffline`——`*net.DNSError`（域名解析失败=无网络/DNS 故障）、`errors.Is(err, syscall.ENETUNREACH)`（网络不可达）、`EHOSTUNREACH`（主机不可达）。此类错误由 `http.Client.Do` 在连接阶段**立即返回**（毫秒级），天然不等整体超时。
- **兜底**：网络黑洞场景（DNS 通但 TCP 挂起）不立即失败——通过 **Dialer 短超时**（`net.Dialer{Timeout: 3s}`）兜底，整体 `FetchTimeout`（10s）保留为最终上限；连接超时（`*net.OpError` + `os.IsTimeout`）也归为 `ErrOffline`（连不上=无法获取）。
- **缓存纪律**：`ErrOffline` **不入 negative cache、不写 ratelimit**——它不是"无歌词"也不是限流；下次播放/手动触发仍会重试（网络恢复后立即可用）。
- **UI**：手动路径（`:lrc switch online`）→ 直接提示 "No internet connection"；自动路径 → 静默（仅 logger.Debug）。

### 2.7b provider 级持久化冷却（v0.12 新增，用户指示）

**问题**：429 冷却此前仅存于会话级内存（`rateLimitedUntil`），重启即丢失——用户关闭软件后立即重新请求，无法判断是否仍在冷却期，可能再次 429 甚至触发临时封禁。

**关键认知**：LRCLIB 限流是 **per-IP / per-服务端** 维度的（同一 BaseURL 的所有请求共享配额），**不是 per-歌曲**。因此冷却必须挂在 **provider（BaseURL）** 上持久化，而不是 per-sig 会话缓存。

**方案**：新增 `ratelimit.go`，provider 级冷却持久化到磁盘。

**① 持久化文件**

```
<config.CacheDir()>/ratelimit.json
```

与歌词缓存同目录（§2.11 ③），独立文件便于审计与清理。内容：

```json
{
  "version": 1,
  "providers": {
    "https://lrclib.net": "2026-08-04T12:05:00Z",
    "http://localhost:8380": "2026-08-04T12:30:00Z"
  }
}
```

- key = 有效 BaseURL；value = 冷却截止时间（UTC RFC3339）。
- 多个自建实例各自独立冷却（换 provider 不误伤）。

**② 接口**

```go
// internal/lyrics/fetch/ratelimit.go

// RateLimit 持久化 provider 级冷却状态。
type RateLimit struct {
 mu      sync.Mutex
 file    string               // ratelimit.json 路径
 until   map[string]time.Time // baseURL → 冷却截止
}

// LoadRateLimit 启动时加载：读文件 + 清理已过期条目；
// 文件不存在 / 损坏 → 空状态（不报错）。
func LoadRateLimit(dir string) (*RateLimit, error)

// Blocked 报告 baseURL 是否仍在冷却期（权威判断）。
func (r *RateLimit) Blocked(baseURL string) bool

// Remaining 返回剩余冷却时长；<=0 表示可请求。
func (r *RateLimit) Remaining(baseURL string) time.Duration

// SetRateLimited 记录 429 冷却并立即持久化（重试耗尽后调用）。
func (r *RateLimit) SetRateLimited(baseURL string, retryAfter time.Duration) error

// Clear 清除指定 provider 冷却（测试/手动重置用）。
func (r *RateLimit) Clear(baseURL string) error
```

**③ 接入点**

| 时机 | 动作 |
|---|---|
| 启动（TUI 初始化） | `LoadRateLimit(cacheDir)` → 挂到 `Model` |
| 构造 `fetch.NewClient` | `NewClient(effectiveBaseURL, cfg)` 绑定 baseURL（配置或默认常量，§2.3）——后续 `Blocked`/`SetRateLimited` 恒用该 key |
| 任何请求前（自动 + 手动） | `rateLimit.Blocked(c.baseURL)` → 冷却中直接跳过（0 请求）；UI 手动路径提示剩余秒数（D16） |
| 429 重试耗尽（`do` 内） | `rateLimit.SetRateLimited(c.baseURL, retryAfter)` → 立即写盘（原子写）；key 恒等于实际请求的 baseURL（v0.13 强化） |
| 冷却自然过期 | `Blocked`/`Remaining` 惰性判断过期（时间对比，不主动删文件条目；下次 `SetRateLimited` 写盘时顺带清理过期键） |

**④ 语义**

- **重启后仍生效**：`ratelimit.json` 保留冷却截止，启动加载后 `Blocked()` 正确返回 → 用户重启软件后冷却期内请求被抑制。
- **冷却与 base_url 强绑定（v0.13，用户指示）**：落库键恒为实际请求的 BaseURL——A 实例被限流不抑制 B 实例；用户切 base_url 后新 provider 立即恢复可请求，旧 provider 条目保留（换回时继续生效）。
- **手动不绕过**：`:lrc switch online` 清 per-sig 缓存（`cache.Clear(sig)`）但 **不** 清除 provider 冷却——避免用户手动触发绕过限流（D16 提示剩余秒数）。
- **非 429 错误不写冷却**：网络错误/5xx/404 不设冷却（只有服务端明确 429 才写）。
- **缓存目录不可写**：`SetRateLimited` 失败 → logger.Warn，内存态仍生效（本次会话有效），下次写盘重试。

### 2.8 MIDI / Tracker 处理

MIDI、Tracker（.mod/.xm/.s3m/.it 等）几乎无歌词，自动请求必然 404 → 浪费请求。

**修改**：`internal/audio/player.go` 导出合成格式判断（最小改动）：

```go
// player.go
// IsSyntheticFormat reports whether the file extension is a synthetic
// (MIDI/tracker) format that has no meaningful lyrics.
func IsSyntheticFormat(ext string) bool { return isSyntheticFormat(ext) }
```

**UI 使用**：`handleAudioLoaded` 前置条件 ④ `ext := filepath.Ext(msg.Path); audio.IsSyntheticFormat(ext)` → 跳过在线获取（不进缓存、不请求）。

- MIDI：`Player.Title()/Artist()` 来自 synthCtrl，LRCLIB 上 MIDI 曲目基本无条目。
- Tracker：常只有标题无艺术家 → 前置条件 ③ 已拦一部分；④ 是确定性兜底（某些模块可能带 artist tag）。
- openmpt 扩展格式（`.mptm` 等）经 `isSyntheticFormat` 的 openmpt 分支覆盖。
- **手动 `:lrc switch online` 不拦截**（用户显式意图优先）。

### 2.9 安全机制（五层防护）

**威胁模型**：① 错误/恶意 URL → 伪造或超大响应；② 自建实例返回损坏数据；③ 中间人伪造；④ 误配内网地址。五层防护，强度按 `Security` 档位裁剪。

**层 1 — 传输层（始终开启）**

| 检查 | 实现 |
|---|---|
| scheme 白名单 | 仅 `http`/`https`（Normalize 校验 + client 二次防御） |
| 证书校验 | Go 默认验证 TLS，**不关闭**（`InsecureTLS=true` 时经 `tls.Config{InsecureSkipVerify: true}` 显式豁免，默认关） |
| 超时 | 连接 + 整体超时（`FetchTimeout`，默认 10s）；**Dialer 短超时 3s 兜底黑洞场景**（v0.15） |
| 跳转限制 | `CheckRedirect` 限 5 次 + 拒绝跳转到非 http(s) |
| ctx 取消 | 全部请求绑定 ctx（切歌/退出即中断，与 429 等待共用） |

**层 2 — 响应层（始终开启）**

| 检查 | 实现 |
|---|---|
| 状态码白名单 | 仅接受 200（+ 429 走重试）；意外状态 → `ErrUnexpectedResponse`（仅日志） |
| Content-Type | 期望 `application/json`（宽松匹配）；`text/html` 等 → 拒绝（防代理错误页） |
| 响应大小上限 | `io.LimitReader` 截断至 `maxFetchResponse = 2MB`（复用 `maxLyricSize` 1MB 思路），超限 → `ErrResponseTooLarge` |
| JSON 严格解码 | 字段类型强校验（层 3） |

**层 3 — 内容层（核心，strict/basic 档差异）**

| 检查 | strict | basic |
|---|---|---|
| 必填字段：`id` 正整数、`trackName`、`artistName` 非空 | ✓ | ✓ |
| `duration` 在 1–3600；与请求时长偏差 > 5s → 拒绝 | ✓ | ✗ |
| `syncedLyrics` 必须能解析出 ≥1 行有效歌词；0 行/纯垃圾 → 拒绝 | ✓ | 仅非空 |
| 歌词末行时间 ≤ duration + 10s 容差 | ✓ | ✗ |
| ≤ 2000 行，单行 ≤ 500 字符 | ✓ | ✗ |
| `instrumental=true` 且无歌词 → 视为无歌词（ErrNotFound 语义） | ✓ | ✓ |
| 文本清洗（去 NUL 等控制字符） | ✓ | ✓ |

**层 4 — 请求层（始终开启）**

| 检查 | 实现 |
|---|---|
| 参数钳制 | duration 钳制 1–3600（文档约束）；`url.Values` 编码 |
| UA 标识 | `NEOVIOLET vX (https://github.com/AuroraStudio-aurorast/NeoViolet)` |
| 前置节流 | 相邻请求 ≥200ms |
| 敏感数据 | 仅发送 title/artist/album/duration，不含音频内容/文件路径 |

**层 5 — 审计（始终开启）**

| 项 | 实现 |
|---|---|
| provider 可见性 | 启动 `logger.Info("lyrics fetch provider", "url", effectiveURL)`；非默认 URL 额外 Warn |
| 请求日志 | `logger.Debug("lrclib fetch", "base", base, "track", title)`；429/失败 `logger.Warn` |
| insecure_tls 告警 | Normalize 中（§2.3） |

**档位语义**：

| 档位 | 内容层校验 | 适用 |
|---|---|---|
| `strict`（默认） | 层 1/2/4/5 + 层 3 全量 | 官方 lrclib.net 或未知自建实例 |
| `basic` | 层 1/2/4/5 + 层 3 仅必填字段 + 非空歌词 | 内部可信自建实例 |

**D8 × D9 交互矩阵**（`insecure_tls` 与 `security` 正交，独立配置）：

| insecure_tls | security | 含义 |
|---|---|---|
| false | strict | 证书校验 + 全量内容校验（默认） |
| false | basic | 证书校验 + 宽松内容校验 |
| true | strict | 跳过证书但仍严格校验内容 |
| true | basic | 完全信任 provider |

### 2.10 优先级列表与 vim-like 指令

**语言约束（用户指示，v0.14）**：**所有 UI 显示使用英语**——包括命令输出、错误提示、状态栏文案；代码注释、日志消息、文档正文不强制（本项目现有风格混用），但用户可见文本一律英文。

**语义（D10 已定，v0.16 用户指示）**：

- **在线歌词与其他格式完全一致**：`"online"` 是 `FormatPriority` 的一个普通条目，与 `embedded/lrc/ttml/...` 同等地由配置 `format_priority` 控制（§2.3），默认放最后；调整列表即调整在线 fallback 次序。
- **从列表删除 `"online"` = 禁止在自动加载优先级中考虑**（自动获取不触发）；与 `Fetch.Enabled` 是 AND 关系。
- 删除列表条目**不**禁用手动命令（`:lrc switch online` 始终可用）。
- **无运行时列表修改指令，也无在线专用开关指令**（D10 定案：仅 config.json）：`Fetch.Enabled` 与 `format_priority` 均由 config.json 管理（§2.3），编辑后重启生效；保持现有指令集精简。

**指令设计（v0.16 精简，复用现有 `:lrc` 语法；UI 文案全部英文）**：

| 指令 | 行为（UI 英文） |
|---|---|
| `:lrc on/off` | Toggle lyrics display (existing; unchanged) |
| `:lrc switch online` | Fetch current track from network now (bypass negative cache; **does not bypass provider cooldown §2.7b**; if cooling, show remaining time) |

> **不再新增 `:lrc online` / `:lrc online on/off` 子命令**（v0.16 用户指示）：在线获取开关（`Fetch.Enabled`）与优先级列表统一由 config.json 管理，与本地格式的开关方式完全一致——不引入在线专用的运行时命令。
> `:lrc switch online` 复用现有 `switch` 子命令；对 `"online"` 特判走 fetch 路径并 `cache.Clear(sig)` 绕过 negative 缓存。与 `:lrc switch lrc` 切换本地格式同构——`online` 作为伪格式名参与同一语法。

---

### 2.11 持久化磁盘缓存（歌词本地保存）

**目的**：在线获取的歌词详细信息保存到本地，后续轮到在线获取时**优先命中本地缓存**（0 请求、离线可用）。与内存层（§2.7 `Cache`）构成双层缓存：内存管会话内复用与 per-sig 镜像，磁盘管跨会话持久化（歌词缓存 + provider 级 429 冷却 §2.7b）。

**① 缓存文件命名**

```
<sha256(sig) 前 32 位 hex>.json
```

- `sig` 与内存缓存键完全一致：`title|artist|album|duration`（归一化，duration 取整秒）。
- **理由**：确定性（同一歌曲总是同一文件，可重定位）；无非法字符（不依赖 tag 内容，规避路径注入）；长度固定（32 hex 字符，碰撞概率可忽略）；不包含可读曲名（避免隐私泄露与文件名超长问题）。
- 扩展名 `.json`（存结构化元数据 + 歌词原文）。

**② 缓存文件内容**（JSON，详细保存）

```json
{
  "version": 1,
  "sig": "i want to live|borislav slavov|baldur's gate 3 (original game soundtrack)|233",
  "provider": "https://lrclib.net",
  "state": "found",
  "track_id": 3396226,
  "track_name": "I Want to Live",
  "artist_name": "Borislav Slavov",
  "album_name": "Baldur's Gate 3 (Original Game Soundtrack)",
  "duration": 233,
  "fetched_at": "2026-08-04T12:00:00Z",
  "synced_lyrics": "[00:17.12] I feel your breath...",
  "plain_lyrics": ""
}
```

```go
// internal/lyrics/fetch/cachefile.go

const cacheFileVersion = 1

// CacheFile 是单个歌曲的持久化缓存记录。
type CacheFile struct {
 Version      int       `json:"version"`
 Sig          string    `json:"sig"`
 Provider     string    `json:"provider"`       // 来源 BaseURL（校验：换 provider 后旧缓存作废）
 State        string    `json:"state"`          // "found" | "notfound" | "instrumental"
 TrackID      int64     `json:"track_id"`
 TrackName    string    `json:"track_name"`
 ArtistName   string    `json:"artist_name"`
 AlbumName    string    `json:"album_name"`
 Duration     int       `json:"duration"`
 FetchedAt    time.Time `json:"fetched_at"`
 SyncedLyrics string    `json:"synced_lyrics"`
 PlainLyrics  string    `json:"plain_lyrics"`
}
```

- `state=found`：缓存完整歌词（`synced_lyrics` 原文，读回时复用 LRC 解析，§D4）。
- `state=notfound`：仅存 `sig/provider/state/fetched_at`，歌词字段空（**负缓存**——记录"这歌没歌词"，避免反复请求）。
- `state=instrumental`：同 notfound 语义（无歌词），单独标记以便未来展示"纯音乐"。
- **429 冷却不写入歌词缓存文件**：冷却单独持久化于 `ratelimit.json`（provider 级，§2.7b），避免污染歌曲缓存键空间。

**③ 存储路径**（用户已定）

```
XDG 模式：    <XDG_CACHE_HOME>/neoviolet/lyrics/<hash>.json
               （XDG_CACHE_HOME 未设置时回退 ~/.cache/neoviolet/lyrics/）
非 XDG 模式： <ConfigDir()>/caches/lyrics/<hash>.json
```

- **XDG 模式**：`config.CacheDir()`（新增，见下）→ `$XDG_CACHE_HOME/neoviolet/lyrics/`，未设置时回退 `~/.cache/neoviolet/lyrics/`（XDG 规范：缓存与配置分离）。
- **非 XDG 模式**：`<ConfigDir()>/caches/lyrics/`——在配置目录（即 exe 目录）下建 `caches/lyrics` 子路径，与 `history.txt`、`config.json` 分离。
- 模式判断与 `config.ConfigDir()` 完全一致（同一 `useXDG` 标志），新增函数：

```go
// internal/config/config.go（新增）

// CacheDir returns the directory for cached data (e.g. fetched lyrics).
// XDG mode:  $XDG_CACHE_HOME/neoviolet/lyrics（回退 ~/.cache/neoviolet/lyrics）
// Non-XDG:   <ConfigDir()>/caches/lyrics
func CacheDir() (string, error) {
 if useXDG.Load() {
  xdgCache := os.Getenv("XDG_CACHE_HOME")
  if xdgCache == "" || !filepath.IsAbs(xdgCache) {
   home, err := os.UserHomeDir()
   if err != nil {
    return "", fmt.Errorf("get home dir for XDG cache: %w", err)
   }
   xdgCache = filepath.Join(home, ".cache")
  }
  return filepath.Join(xdgCache, "neoviolet", "lyrics"), nil
 }
 dir, err := ConfigDir()
 if err != nil {
  return "", err
 }
 return filepath.Join(dir, "caches", "lyrics"), nil
}
```

- 首次写前 `os.MkdirAll(dir, 0755)`。
- 与配置、历史文件分离，便于整体清理。

**④ 刷新时机（TTL 策略）**

| 状态 | TTL | 依据 |
|---|---|---|
| `found` | **30 天** | 歌词内容稳定，长缓存合理；超过 30 天后允许重新获取以捕获服务端修订 |
| `notfound` | **7 天** | LRCLIB 文档明示"缺的曲目可能被后台补录、稍后可能可用"——负缓存必须短 TTL，过期后重新尝试 |
| `instrumental` | 7 天 | 同上（可能被补录为带歌词版本） |

- **读取时惰性判定**：`Load()` 内比较 `FetchedAt + TTL` 与当前时间；过期 → 视为 miss 并删除文件（写回时自然重建）。
- **手动强制刷新**：`:lrc switch online` 强制忽略缓存（`cache.Clear(sig)` + `cachefile.Remove(sig)`）→ 重新请求并覆盖缓存。
- **provider 变更失效**：`Load()` 校验 `Provider == 当前 BaseURL`，不一致 → 视为 miss（不同源的数据不混用）。
- **损坏容错**：JSON 解析失败 / `version` 不符 / `sig` 不匹配文件名 → 视为 miss 并删除（静默，不报错）。
- **不自动后台刷新**：无后台定时任务；TTL 过期由下一次请求触发（惰性）。

```go
// cachefile.go 接口
func cacheDir() (string, error)          // config.CacheDir()
func cacheFileName(sig string) string   // sha256(sig)[:32] + ".json"
func (cf CacheFile) expired() bool      // FetchedAt + ttl(state) < now
func Load(sig, provider string) (*CacheFile, bool)  // 命中且未过期且 provider 匹配 → (file, true)
func Save(cf CacheFile) error           // MkdirAll + 写 JSON（原子：先写 .tmp 再 rename）
func Remove(sig string) error           // 手动刷新时删除
func CleanupExpired(dir string) (int, error) // 启动清理（v0.17）：扫描 *.json，仅处理 ^[0-9a-f]{32}\.json$ 文件名（自动排除 ratelimit.json），过期或损坏 → 删除；未过期/非本格式 → 保留
```

**⑤ 数据流接线**（已在 §2.5 反映）：内存 miss → `cachefile.Load` → 磁盘 hit 载入内存 + 挂载；磁盘 miss → 网络请求 → 成功后 `cachefile.Save` + 内存 Store。**读/写均收敛在 `FetchLyrics` 内**，UI 层只持有内存 `Cache` 引用，不直接触碰磁盘（可测性）。

**⑥ 启动清理（v0.17，缺口 2）**：惰性过期（`Load` 时检查）只清理"再次播放"的文件——长期不播的歌曲缓存永留磁盘。UI 初始化时（`root.go runRoot`，config.Load 之后）异步调用 `CleanupExpired(CacheDir())` 一次，`removed>0` 时 `logger.Info("lyrics cache cleanup", "removed", n)`。

- 文件名白名单 `^[0-9a-f]{32}\.json$` 天然隔离 `ratelimit.json`（§2.7b）与任何非本格式文件，不误删。
- 损坏文件（JSON 解析失败）也删除——与 `Load` 的损坏容错语义一致。
- 不做后台定时/容量配额（范围外）。

---

### 2.12 竞态防护与获取中反馈（v0.17，缺口 1/3）

**问题 1（竞态）**：`FetchLyrics` 以 `tea.Cmd` 异步运行，结果经 `FetchLyricsResultMsg` 回挂。快速切歌 A→B 时，A 的请求可能晚于 B 的结果到达——无校验时旧结果会**覆盖当前歌词**（或失败结果清掉新歌词）。

**设计**：

- `FetchLyricsResultMsg` 携带 `Sig string`（请求发起时计算的归一化签名，与缓存 key 同源，§2.6）。
- 挂载处理校验：用**当前曲目元数据**现算 `cur := fetch.Sign(title, artist, album, duration)`，`msg.Sig != cur` → **丢弃 UI 展示**（缓存写入已在 `FetchLyrics` 内完成，不受影响）。

```go
// 任务 9 挂载处理（示意）
cur := fetch.Sign(m.Audio.Title, m.Audio.Artist, m.Audio.Album, m.Audio.Duration)
if msg.Sig != cur { return m, nil } // 旧结果：仅丢弃挂载，缓存已正确写入
```

- **缓存正确性不依赖该校验**：磁盘/内存写入收敛在 `FetchLyrics` 内部（§2.11 ⑤），竞态只影响"挂载"一步。

**问题 2（无反馈）**：自动路径完全静默、fetch 异步，慢网时用户无感知。

**设计**：

- `Model` 增加 `LyricsFetching bool`：发起请求（置 `CachePending`）时 true；结果到达且 `msg.Sig == cur` 时 false（**切歌后旧结果不清除新 pending**——旧结果 sig 不匹配，不触碰标记）。
- 显示：歌词区无歌词时显示 `[Fetching lyrics...]`（英文，§2.10 语言约束）。
- 失败结果同样清除标记（fetch 完成即退出 fetching 态）。

**测试**（并入任务 9）：

- A 请求未归 → 切 B → A 结果晚到 → B 歌词不被覆盖；B 结果正常挂载。
- 旧结果不清除新 pending（LyricsFetching 保持 true 直到 B 结果）。
- 成功/失败/离线结果均清除 fetching 标记。

---

## 3. 错误处理

| 场景 | 处理 | UI |
|---|---|---|
| 网络错误 / 超时 / 证书错误 | logger | 极简或静默 |
| **离线**（DNS 失败 / 网络不可达 / 连接超时） | `ErrOffline`——不入 negative cache、不写 ratelimit；下次重试（v0.15） | **手动路径直接提示 "No internet connection"；自动路径静默** |
| 404 无匹配 | `ErrNotFound` → `cache.Store(CacheNotFound)` | 静默 |
| Search 候选全部未通过验证（错配） | `ErrNoMatch` → `cache.Store(CacheNotFound)` | 静默 |
| 候选仅 plainLyrics | 放弃 → `cache.Store(CacheNotFound)` | 静默 |
| 429 限流 | Retry-After 等待 → 重试 1 次 → 仍 429：`RateLimitError{RetryAfter}` → `rateLimit.SetRateLimited(baseURL, retryAfter)` 持久化（§2.7b） | **不显示，仅 logger.Warn** |
| 冷却中（自动路径） | provider 冷却未到 → 静默跳过（0 请求） | 静默 |
| 冷却中（手动触发） | provider 冷却未到 → **不发起请求** | 提示剩余秒数（决策点 D16）或静默 |
| 冷却文件损坏 / 不可写 | `LoadRateLimit` 空态 / `SetRateLimited` 失败 → logger.Warn，内存态仍生效 | 静默 |
| 手动触发时冷却未到 | 不发起请求 | 提示剩余秒数（决策点 D16）或静默 |
| 非预期状态码 / Content-Type 不符 / 响应过大 / JSON 字段缺失 / 歌词校验失败 | `ErrUnexpectedResponse` / `ErrResponseTooLarge` / `ErrInvalidPayload` | 仅 logger |
| 前置条件不满足（开关关 / 无条目 / 无元数据 / 合成格式） | 跳过自动获取 | 静默 |
| 缓存命中 notFound / rateLimited（冷却未到） | 0 请求 | 静默 |
| 磁盘缓存命中（未过期） | 载入内存复用（0 请求） | 静默 |
| 磁盘缓存过期 / 损坏 / provider 不符 | 视为 miss，删除旧文件，正常请求 | 静默（仅 Debug 日志） |
| 缓存目录不可写（权限/只读介质） | logger.Warn 一次，降级为仅内存缓存 | 静默 |
| 无 Title/Artist（URL 流等） | 跳过自动获取 | 手动命令时提示 |
| instrumental / 空歌词 | 视为无歌词（ErrNotFound 语义） | 静默 |

---

## 4. 测试策略

### match_test.go（D2 核心）

- Normalize：大小写、全角→半角、空白折叠、剥离 `(feat. X)`/`(Live)`/`feat.` 后缀、Unicode 组合字符。
- **变音剥离**（lrcget 对齐）：`Café`→`cafe`、`Für Elise`→`fur elise`、`Beyoncé`→`beyonce`（§2.6b）。
- **标点/撇号**（lrcget 对齐）：`don't`→`dont`、`I’m`→`im`、`A&W`→`a w`、`x*5`→`x 5`（§2.6b 字符集）。
- **括号块**：`Song (Live)`、`Song [Remastered]` → `song`。
- SplitArtists：`;`/`,`/`feat.`/`ft.`/`with`/`/` 拆分；**`&` 拆分**（`Porter Robinson & Madeon` → [porter robinson, madeon]，D13 已定）；**NUL 拆分**（`Porter Robinson\x00Madeon` → [porter robinson, madeon]）；`and` 保持整体；空/单歌手边界。
- MatchTrack：相等/不等/带括号版本 vs 原版。
- MatchArtist strict：`req ⊆ cand` 通过；缺一歌手拒绝；候选多歌手通过。
- MatchArtist 平衡档：交集非空通过。
- **候选选取**：多条通过时按 `|Δduration|` 最小者选取（实测用例：Δ0s 优先于 Δ2s/Δ29s）；无通过 → 拒绝。
- **端到端真实用例**（Porter Robinson/Madeon - Shelter）：`req[porter robinson, madeon]` 对 `Porter Robinson & Madeon`/`, Madeon`/`; Madeon`/`,` 全部通过；对 `Simon & Garfunkel` 类整体歌手名需 req 侧也拆才通过（见 D13 语义）。

### cache_test.go（429 冷却核心）

- Lookup/Store 各状态；pending 合并（并发同 sig）。
- **notFound 不重复请求**；found 复用。
- **RateLimited 冷却未到 → 返回 RateLimited**；**冷却已过 → miss 语义允许重试**。
- StoreRateLimited 记录 `rateLimitedUntil`；`Clear(sig)` 清 negative/pending。
- sig 归一化（大小写/trim/duration 取整秒）。
- **注意**：冷却权威判断在 `rateLimit.Blocked`（§2.7b）；本测试验证镜像一致性。

### ratelimit_test.go（provider 级持久化冷却，§2.7b）

- **加载**：文件不存在 → 空态不报错；损坏 JSON → 空态不报错；合法文件 → 正确加载。
- **过期清理**：启动加载时剔除已过期条目；`Blocked` 惰性判定过期（时间对比）。
- **SetRateLimited 持久化**：写入后重读文件字段正确；跨实例重建（模拟重启 `LoadRateLimit`）后 `Blocked` 仍为 true、`Remaining` 正确。
- **多 provider 隔离**：A 冷却不影响 B；`Clear(A)` 后 A 可请求、B 仍冷却。
- **key = 实际请求 baseURL**：`SetRateLimited("https://a.example", ...)` 后仅该 key 出现；`Blocked("https://lrclib.net")` 不受影响（v0.13）。
- **Remaining 语义**：冷却中返回剩余时长；过期/无记录返回 <=0。
- **原子写**：无 .tmp 残留；目录不可写 → 返回错误不 panic。

### cachefile_test.go（持久化缓存核心，§2.11）

- **文件名**：`sha256(sig)[:32] + ".json"` 确定性；同一 sig 同一文件；不同 sig 不同文件；特殊字符（中文/emoji/路径注入字符）安全。
- **读写往返**：Save → Load 字段完整（含歌词原文）；原子写（无 .tmp 残留）。
- **TTL**：found 未过期命中；found 过期 → miss + 文件删除；notfound 7 天过期后重新请求；instrumental 同 notfound。
- **provider 校验**：Save 于 provider A → Load(provider B) → miss。
- **损坏容错**：非法 JSON / version 不符 / sig 不匹配 → miss + 删除。
- **目录不可写**：Save 失败返回错误，不 panic。

### client_test.go

- URL 构造（自定义 BaseURL + 默认）、UA 头断言、JSON 解码。
- **429 重试**：首请求 429 + `Retry-After: 1` → 第二次请求发出、间隔 ≥1s、最终成功。
- **Retry-After 缺失 → 兜底 2s**；连续 429 → 最多 2 次请求 → `RateLimitError{RetryAfter}`。
- **冷却 key 绑定 baseURL（v0.13）**：对 client A（`https://a.example`）触发 429 耗尽 → `ratelimit.json` 中出现 key `https://a.example`（非默认值/非 B）；`Blocked("https://b.example")` 为 false。
- 等待期间 ctx 取消 → 立即返回。
- **离线识别（v0.15）**：mock DNS 失败（`*net.DNSError`）/ `ENETUNREACH` / 连接超时 → 返回 `ErrOffline`（不等整体超时）；离线结果不入 negative cache、不写 ratelimit。
- 前置节流：同一 client 连续两次请求间隔 ≥ 阈值；**两个 client 实例互不拖慢**；跳转限制。

### validate_test.go

- scheme 拒绝（`file://` 等）；Content-Type 不符；响应超限（>2MB）；JSON 字段缺失。
- strict 档：duration 偏差 >5s 拒绝；syncedLyrics 解析 0 行拒绝；时间戳越界拒绝；行数/行长超限拒绝。
- basic 档：上述内容校验跳过，仅非空校验；instrumental 语义。

### fetch_test.go

- Get 命中 → LyricsData 正确（Lines/时间戳/Format="lrclib"）。
- Get 404 → Search 兜底 → 首个通过验证候选采用；全部错配 → ErrNoMatch。
- **仅 plain 候选 → 放弃** → CacheNotFound。
- 429 耗尽 → RateLimitError + 缓存冷却。

### config_test.go

- 默认 `FormatPriority` 含尾位 `"online"`；用户配置不含时不被强加。
- base_url 规范化（去尾斜杠）/非法 scheme 回退/超时/安全档位/insecure_tls 不丢值。
- Normalize 返回 true 触发 Save（fetch 字段变更时 fetchChanged=true）。

### internal/version 测试

- `UserAgent()` 格式（dev 与注入版本两态）。

### UI 测试

- 合成格式（.mid/.mod）不产生 FetchLyricsCmd。
- 无 `"online"` 条目 / `Fetch.Enabled=false` / 无 Title+Artist → 不产生 FetchLyricsCmd。
- 缓存命中 found → 复用；命中 notFound/rateLimited（冷却未到）→ 静默。
- result 消息挂载正确；429 结果不产生用户提示。
- `:lrc switch online` 命令行为（含冷却未到时不请求）。

---

## 5. 任务分解

### 任务 1：`internal/version` 包 + Makefile 注入

**文件：**

- 创建：`internal/version/version.go`
- 修改：`Makefile`（LDFLAGS）
- 测试：`internal/version/version_test.go`

- [ ] 步骤 1：创建 `internal/version/version.go`（§2.4 代码）+ `version_test.go`（UA 格式断言：注入版与 dev 版）。
- [ ] 步骤 2：`make test`（或 `go test ./internal/version/`）确认通过。
- [ ] 步骤 3：Makefile LDFLAGS 增加 `-X .../internal/version.Version=$(VERSION)`（保留原 cmd 注入）。
- [ ] 步骤 4：`make build` 确认编译通过；`./neoviolet version` 输出不变。

### 任务 2：配置扩展

**文件：**

- 修改：`internal/config/config.go`、`internal/config/config_test.go`

- [ ] 步骤 1：`LyricsConfig` 增加 `Fetch LyricsFetchConfig` 字段；新增 `LyricsFetchConfig` 结构 + `DefaultBaseURL/DefaultFetchTimeout/DefaultSecurity` 常量；`DefaultConfig()` 设默认（含 `FormatPriority` 尾位 `"online"`）。
- [ ] 步骤 2：`Normalize()` 增加 fetch 字段校验（§2.3 代码）+ `isLocalHost`/`isPrivateHost`（**SSRF 私网警告 v0.17**）帮助函数；返回 `volumeChanged || fetchChanged`。
- [ ] 步骤 3：新增 `CacheDir()`（§2.11 代码，与 `useXDG` 一致：XDG → `$XDG_CACHE_HOME/neoviolet/lyrics` 回退 `~/.cache/...`；非 XDG → `<ConfigDir()>/caches/lyrics`）。
- [ ] 步骤 4：写测试：默认值、base_url 规范化/非法回退、timeout/security 兜底、fetchChanged 触发 Save 语义、旧配置兼容（无 fetch 字段 → 默认值）、`CacheDir()` 两模式路径。
- [ ] 步骤 5：`make test` 确认通过。

### 任务 3：导出合成格式判断

**文件：**

- 修改：`internal/audio/player.go`、`internal/audio/player_test.go`（如存在）

- [ ] 步骤 1：`player.go` 增加导出包装 `func IsSyntheticFormat(ext string) bool { return isSyntheticFormat(ext) }`。
- [ ] 步骤 2：写测试：`.mid/.mod/.xm/.mptm` 等返回 true；`.mp3/.flac` 返回 false。
- [ ] 步骤 3：`make test` 确认通过。

### 任务 3b：导出 LRC 解析帮助函数（D4 已确认）

**文件：**

- 修改：`internal/lyrics/lrc.go`、`internal/lyrics/lrc_test.go`（或新建）

- [ ] 步骤 1：`lrc.go` 新增导出包装 `func ParseLRC(r io.Reader) (*LyricsData, error)`——委托给 `lrcParser{}.Parse` 内部逻辑（不改既有解析行为，仅导出）。
- [ ] 步骤 2：写测试：`ParseLRC` 与 `lrcParser.Parse` 结果一致；空输入/纯注释行 → 空 LyricsData 不报错。
- [ ] 步骤 3：`make test` 确认通过。

### 任务 4：fetch 基础类型 + 内容安全校验

**文件：**

- 创建：`internal/lyrics/fetch/types.go`、`internal/lyrics/fetch/validate.go`
- 测试：`internal/lyrics/fetch/validate_test.go`

- [ ] 步骤 1：`types.go`：`Track` 响应结构体（ID/TrackName/ArtistName/AlbumName/Duration/Instrumental/PlainLyrics/SyncedLyrics）+ `APIError{Code,Name,Message}` + JSON tag。
- [ ] 步骤 2：`validate.go`：层 2（大小限制/Content-Type）与层 3（strict/basic 内容校验）纯函数——`ValidateTrack(t, opts) error`。
- [ ] 步骤 3：写 validate_test.go（§4 测试策略列出的校验矩阵）。
- [ ] 步骤 4：`make test` 确认通过。

### 任务 5：匹配验证模块

**文件：**

- 创建：`internal/lyrics/fetch/match.go`
- 测试：`internal/lyrics/fetch/match_test.go`
- 修改：`go.mod`（新增 `golang.org/x/text`）

- [ ] 步骤 1：`go get golang.org/x/text`（norm 子包）。
- [ ] 步骤 2：`match.go`：`Normalize`/`SplitArtists`/`MatchTrack`/`MatchArtist`（§2.6 代码含 §2.6b 变音剥离/标点/撇号处理，按 D13/D14 决策落地）+ **`Sign(title, artist, album string, duration float64) string`（缓存 key 与竞态校验共用：归一化拼接 + 取整秒，§2.12）**。
- [ ] 步骤 3：写 match_test.go（§4 测试策略矩阵）。
- [ ] 步骤 4：`make test` 确认通过。

### 任务 6：会话级请求缓存

**文件：**

- 创建：`internal/lyrics/fetch/cache.go`
- 测试：`internal/lyrics/fetch/cache_test.go`

- [ ] 步骤 1：`cache.go`：`Cache`/`CacheState`/`Lookup`/`Store`/`StoreRateLimited`/`Clear`（§2.7 代码）。
- [ ] 步骤 2：写 cache_test.go（found/notFound/rateLimited 冷却过期语义/pending 合并/Clear/sig 归一化）。
- [ ] 步骤 3：`make test` 确认通过。

### 任务 6b：持久化磁盘缓存（§2.11）

**文件：**

- 创建：`internal/lyrics/fetch/cachefile.go`
- 测试：`internal/lyrics/fetch/cachefile_test.go`

- [ ] 步骤 1：`cachefile.go`：`CacheFile` 结构 + `cacheDir`（调 `config.CacheDir()`）/`cacheFileName`/`expired`/`Load`/`Save`（原子写）/`Remove`（§2.11 代码，TTL 常量：found 30 天、notfound/instrumental 7 天；provider 校验；损坏容错）+ **`CleanupExpired`（v0.17 缺口 2：文件名白名单 `^[0-9a-f]{32}\.json$`，过期/损坏删除，ratelimit.json 与非法文件保留）**。
- [ ] 步骤 2：写 cachefile_test.go（§4 测试策略：文件名确定性/读写往返/TTL 过期/provider 不符/损坏容错/目录不可写）+ **CleanupExpired 用例（过期/未过期/损坏/32hex 非本格式/ratelimit.json）**。
- [ ] 步骤 3：`make test` 确认通过。

### 任务 6c：provider 级持久化冷却（§2.7b）

**文件：**

- 创建：`internal/lyrics/fetch/ratelimit.go`
- 测试：`internal/lyrics/fetch/ratelimit_test.go`

- [ ] 步骤 1：`ratelimit.go`：`RateLimit`/`LoadRateLimit`/`Blocked`/`Remaining`/`SetRateLimited`/`Clear`（§2.7b 代码，原子写、过期惰性清理、多 provider 隔离、key 恒为传入 baseURL）。
- [ ] 步骤 2：写 ratelimit_test.go（§4 测试策略：加载/损坏容错/过期清理/持久化跨实例/多 provider/Remaining 语义）。
- [ ] 步骤 3：`make test` 确认通过。

### 任务 7：HTTP 客户端

**文件：**

- 创建：`internal/lyrics/fetch/client.go`
- 测试：`internal/lyrics/fetch/client_test.go`

- [ ] 步骤 1：`client.go`：`Client` 结构（**持有 `baseURL` 字段**，§2.7）、`NewClient(effectiveBaseURL, cfg)`（BaseURL 默认/超时/**Dialer 3s**/TLS 豁免/跳转限制）、`Get(track)`/`Search(...)`/`GetByID(id)`、前置节流 `throttle()`（per-client 计时）、429 重试逻辑（`SetRateLimited(c.baseURL, ...)` 落库）、**离线识别**（`isOfflineErr`：DNSError/ENETUNREACH/EHOSTUNREACH/连接超时 → `ErrOffline`）、UA 头（`version.UserAgent()`）。
- [ ] 步骤 2：写 client_test.go（httptest mock：URL 构造/UA/429 重试/兜底/上限/ctx 取消/节流/跳转）。
- [ ] 步骤 3：`make test` 确认通过。

### 任务 8：高层获取逻辑

**文件：**

- 创建：`internal/lyrics/fetch/fetch.go`
- 测试：`internal/lyrics/fetch/fetch_test.go`

- [ ] 步骤 1：`fetch.go`：`FetchLyrics(ctx, meta, opts) (*lyrics.LyricsData, error)`——**磁盘缓存 Load 优先**（命中直接返回）→ Get 精确 → 404 降级 Search → 匹配验证（MatchTrack+MatchArtist）→ plain 放弃 → 解析（**`lyrics.ParseLRC` 复用**，任务 3b 已导出）→ 成功写回磁盘+内存缓存 → 错误语义（ErrNotFound/ErrNoMatch/RateLimitError 等）。
- [ ] 步骤 2：写 fetch_test.go（Get 命中/404 降级/错配/plain 放弃/429 缓存接线）。
- [ ] 步骤 3：`make test` 确认通过。

### 任务 9：UI 自动获取接入

**文件：**

- 修改：`internal/ui/update.go`、`internal/ui/types.go`、`internal/ui/ui_test.go`

- [ ] 步骤 1：`types.go`：`Model` 增加 `fetchCache *fetch.Cache` + `LyricsFetching bool`（v0.17 缺口 3）；新增消息类型 `FetchLyricsMsg`（meta）/`FetchLyricsResultMsg`（data/err/**sig**）。
- [ ] 步骤 2：`update.go`：`handleAudioLoaded` 本地失败后走前置条件 ①②③④ → 缓存检查 → 置 pending + `LyricsFetching=true` → `tea.Cmd` 启动 `FetchLyrics`；result 处理（**sig 校验：不匹配丢弃挂载 §2.12**，匹配则挂载 + 清除 fetching 标记；缓存写入 / logger；**ErrOffline 静默**）。
- [ ] 步骤 3：写 UI 测试（合成格式/开关/无条目/无元数据不触发；缓存命中；result 挂载；429 静默；**离线自动路径静默**；**竞态：旧结果不覆盖新歌词 §2.12；fetching 标记清除语义**）。
- [ ] 步骤 4：`make test` 确认通过。

### 任务 10：vim-like 指令

**文件：**

- 修改：`internal/ui/update_keyboard.go`、`internal/ui/update_keyboard_test.go`

- [ ] 步骤 1：`:lrc switch online` 特判子命令（§2.10；复用现有 `switch` 语法，`online` 作为伪格式名；含冷却未到时提示剩余秒数；**UI 文案英文**）。
- [ ] 步骤 2：`switch online` 特判：`cache.Clear(sig)` 绕过 negative 缓存 + 冷却检查（不绕过 429 冷却）；**ErrOffline → 提示 "No internet connection"**（v0.15）。
- [ ] 步骤 3：写命令测试（`switch online` 行为 + 冷却未到不请求 + 离线提示 + 与 `switch lrc` 等本地格式同构不回归）。
- [ ] 步骤 4：`make test` 确认通过。

### 任务 11：全量验证

- [ ] `make test`（全量，含 race：`make test/race`）
- [ ] `make vet`
- [ ] `make lint`
- [ ] `make build` + 手动冒烟：无本地歌词的 mp3 → 自动获取；断网 → 静默；`:lrc switch online` 手动获取（**英文 UI**）

---

### 5b. 实施与提交计划（v0.17，用户指示：commit 用英文；能不复杂就不复杂、无 slop、无垃圾注释、鲁棒性优先）

依赖链：`version/配置/导出`（独立）→ `fetch 包`（串行）→ `UI 集成` → `收尾`。**红线：每个 commit 独立编译 + `make test` 通过**。分支：`feat/online-lyrics`，合并回 main 用 `--no-ff`。

| # | Commit（英文 message） | 内容 | 任务 |
|---|---|---|---|
| C1 | `docs: add online lyrics implementation plan` | 计划文档定稿 v0.17 | — |
| C2 | `feat: add internal/version package and Makefile injection` | version 包 + 双注入 | 1 |
| C3 | `feat: add online fetch config with normalization and SSRF warning` | 配置子结构 + Normalize + isPrivateHost 警告 | 2a |
| C4 | `feat: add XDG-aware cache directory` | CacheDir() 两模式 | 2b |
| C5 | `refactor: export audio.IsSyntheticFormat` | 导出包装 | 3 |
| C6 | `refactor: export lyrics.ParseLRC helper` | 导出包装 | 3b |
| C7 | `feat: add fetch types and content validation` | types/validate | 4 |
| C8 | `feat: add lyric matching and normalization` | match + Sign + go.mod | 5 |
| C9 | `feat: add session lyric cache` | cache.go | 6 |
| C10 | `feat: add persistent disk cache with expired cleanup` | cachefile + CleanupExpired | 6b |
| C11 | `feat: add provider-level persistent rate limit` | ratelimit.go | 6c |
| C12 | `feat: add LRCLIB HTTP client` | client（UA/429/离线/节流） | 7 |
| C13 | `feat: add FetchLyrics high-level fetch logic` | fetch.go | 8 |
| C14 | `feat: auto-fetch lyrics with race guard and fetching indicator` | UI 接入 + 竞态 + 反馈 | 9 |
| C15 | `feat: add :lrc switch online command` | 手动指令 | 10 |
| C16 | `test: run full verification` | race/vet/lint + 冒烟 | 11 |

C2–C6 相互独立可并行；C10 依赖 C4；C12 依赖 C2/C11；C13 依赖 C7–C12/C6；C14 依赖 C13；C15 依赖 C14。

---

## 6. 决策点清单（待确认）

| # | 决策点 | 现状 | 建议 |
|---|---|---|---|
| D1 | 自动获取触发条件 | 已确认 | `Fetch.Enabled` AND 列表含 `"online"` AND Title+Artist 非空 AND 非合成格式 |
| D2 | Search 兜底验证 | 已确认 | 严格验证（MatchTrack 相等 + MatchArtist）+ plain-only 放弃 |
| D3 | 缓存写盘 | 已确认 | 持久化磁盘缓存（§2.11）：found 30 天 / notfound 7 天 TTL，路径见 D18（XDG `~/.cache/neoviolet/lyrics` / 非 XDG `<ConfigDir()>/caches/lyrics`） |
| D4 | 解析复用 | **已确认** | `lyrics` 包导出 `ParseLRC` 帮助函数（DRY，复用既有 `bracketRe`/`wordTagRe` 解析；fetch 与缓存读回共用） |
| D5 | plainLyrics 展示 | 已定（自动路径） | 自动路径一律放弃；手动例外见 D15 |
| D6 | 版本注入 | 已确认 | 双注入（cmd + internal/version） |
| D7 | 429 重试上限 | 已确认 | 重试 1 次；仍 429 静默保留倒计时并持久化（provider 级 `ratelimit.json` §2.7b），用户再触发时遵守冷却 |
| D8 | 安全档位 | 已确认 | `security: "strict"\|"basic"` 可配置，默认 strict |
| D9 | TLS 豁免 | 已确认 | `insecure_tls: bool` 可配置，默认 false |
| D10 | 指令命名 | **已确认** | **精简：仅 `:lrc on/off`（现有）+ `:lrc switch online`（复用现有 switch 语法）**；不新增 `:lrc online`/`:lrc online on|off`——在线获取开关与优先级统一由 config.json 管理（v0.16 用户指示） |
| D11 | negative cache 粒度 | 已确认 | 内存层会话级 + 磁盘层 7 天 TTL（LRCLIB 可能补录，过期重新尝试） |
| D12 | 合成格式排除范围 | **已确认** | 自动获取排除全部 syntheticFormats（含 openmpt 扩展）；手动 `:lrc switch online` 不拦截 |
| D13 | 多歌手 `&` 是否拆分 | **已确认（拆）** | 拆 `&`（Porter Robinson/Madeon - Shelter 实测：不拆会拒绝 11/20 主候选，§2.6b）；`and` 不拆；新增 NUL 分隔符 |
| D14 | 歌手匹配档位默认值 | **已确认** | 自动路径固定 strict（`req ⊆ cand`）；`basic` 档只放宽内容层校验，不降匹配严格度（两维正交，§2.9） |
| D15 | plain-only 手动例外 | **已确认** | `:lrc switch online` 遇 plain-only：也放弃（与自动路径一致；静态歌词无法滚动联动） |
| D16 | 手动触发时冷却未到的提示 | **已确认** | 提示剩余秒数（英文，如 "Rate limited, retry in 3m 12s"）；自动路径保持静默 |
| D17 | 磁盘缓存 TTL 值 | **已确认** | found 30 天 / notfound·instrumental 7 天（不对称：found 过期成本=1 请求，notfound 过期解锁补录） |
| D18 | 缓存目录位置 | 已确认 | XDG → `$XDG_CACHE_HOME/neoviolet/lyrics`（回退 `~/.cache/`）；非 XDG → `<ConfigDir()>/caches/lyrics`（用户已定，`config.CacheDir()`） |

---

## 7. Review 历史

- **v0.1**：初始计划（本地 lyrics 管线调研 + LRCLIB API 提取）。
- **v0.2**：UA 采用 `NEOVIOLET v<version> (https://github.com/AuroraStudio-aurorast/NeoViolet)`；`internal/version` 包 + Makefile 注入。
- **v0.3**：429 不显示用户提示（仅 logger）；按 Retry-After 重试；区分 404（无匹配）与 429（限流）。
- **v0.4**：Base URL 可配置（默认 lrclib.net）；安全五层防护 + strict/basic 档位。
- **v0.5**：D8/D9 可配置化（嵌套 `LyricsFetchConfig`）；`"online"` 进 `FormatPriority`（默认尾位）+ vim-like 指令；会话级请求缓存；MIDI/Tracker 合成格式排除。
- **v0.6**：D2 严格匹配验证（Normalize/SplitArtists/MatchTrack/MatchArtist）+ plain-only 放弃。
- **v0.7**：429 冷却语义精化——遵守 Retry-After；重试 1 次后不再自动尝试；仍 429 静默保留倒计时（`CacheRateLimited` + `rateLimitedUntil`）；用户再触发时冷却未到不请求、到后放行；写入 `docs/plans/2026-08-04-online-lyrics.md`。
- **v0.8**：持久化磁盘缓存（§2.11 新增）——歌词详细信息本地保存；文件命名 `<sha256(sig)[:32]>.json`、内容 JSON 元数据+歌词原文、路径 `ConfigDir()/lyrics_cache/`、TTL 刷新（found 30 天 / notfound 7 天）；内存+磁盘双层缓存；D3/D11 状态确认，新增 D17（TTL 值）/D18（缓存目录）。
- **v0.9**：缓存路径改为用户方案——XDG 模式 `$XDG_CACHE_HOME/neoviolet/lyrics/`（回退 `~/.cache/`），非 XDG 模式 `<ConfigDir()>/caches/lyrics/`；新增 `config.CacheDir()`（与 `useXDG` 一致）；D18 已确认；任务 2 增补 CacheDir 实现与测试。
- **v0.10**：参考 lrcget 官方客户端源码（§2.6b 新增）——Normalize 纳入变音符号剥离（NFKD+Mn 过滤，对齐 `secular::lower_lay_string`）、标点→空格、撇号移除；明确 /api/get 信任边界（本项目仍走内容校验）与 /api/search 无自动匹配（保守拒绝）；plain 从 synced 剥离时间戳派生；D13 补充 lrcget 对照。
- **v0.11**：真实数据验证（Porter Robinson/Madeon - Shelter）——`/api/get` 原样 artist 直接命中 id 887633；`/api/search` 20 候选无 887633、artist 7 种写法；**D13 确认拆 `&`**（不拆会拒绝 11/20 主候选）；SplitArtists 新增 NUL 分隔符；候选选取新增 duration 二级排序（|Δ| 最小者）；测试策略补端到端用例。
- **v0.12**：**429 冷却持久化（§2.7b 新增）**——冷却从 per-sig 会话级升级为 **provider 级持久化**（LRCLIB 限流为 per-IP 维度）；`ratelimit.json` 存 BaseURL→冷却截止，启动 `LoadRateLimit` 加载，重启后仍能判断是否可请求；请求前 `Blocked(baseURL)` 权威判断；手动不绕过；非 429 不写冷却；新增 ratelimit.go 与任务 6c。
- **v0.13**：**冷却与 base_url 强绑定（用户指示）**——`Client` 构造绑定 `baseURL`（配置或默认），429 落库键恒为实际请求的 baseURL（绝不写默认/全局值）；切 provider 零误伤（A 限流不抑制 B，换回 A 冷却仍生效）；前置节流改 per-client 计时；client/ratelimit 测试补绑定断言。
- **v0.14**：**决策点批量确认 + UI 语言约束（用户指示）**——D4（导出 `ParseLRC`，任务 3b）、D12（合成格式全排除）、D14（匹配与内容两维正交，basic 不降匹配）、D15（plain-only 一致放弃）、D16（手动提示剩余秒数，英文文案）、D17（found 30 天 / notfound 7 天）均确认；**全部 UI 显示使用英语**（§2.10 语言约束）；**移除 `:lrc priority` 系列指令**（用户：暂无需求），列表条目由 config.json 管理，优先级条目运行时管理方式列为待探讨（D10）。
- **v0.15**：**D10 定案 + 离线检测（用户指示）**——① 在线歌词与其他格式完全一致：`"online"` 为 `FormatPriority` 普通条目，优先级由 config.json 控制（默认尾位），无运行时列表修改指令，`:lrc switch online` 手动获取（与 `:lrc switch lrc` 同构）；② **离线不等待超时直接提示**——新增 `ErrOffline`（DNS 失败/ENETUNREACH/EHOSTUNREACH/连接超时识别），连接层立即返回，Dialer 3s 兜底黑洞，不入 negative cache 不写 ratelimit；手动路径提示 "No internet connection"，自动路径静默。
- **v0.16**：**指令进一步精简（用户指示）**——移除 `:lrc online` / `:lrc online on|off`，**仅保留 `:lrc on/off`（现有歌词开关）与 `:lrc switch online`（复用现有 switch 语法）**；`Fetch.Enabled` 与优先级列表统一由 config.json 管理，不引入任何在线专用运行时命令（§2.10、D10、任务 10 同步）。
- **v0.17（本版，头脑风暴补漏）**：**三缺口 + SSRF 警告 + 范围外清单 + 提交计划**——① 竞态防护（§2.12：`FetchLyricsResultMsg` 携带 sig，挂载前与当前曲目比对，不匹配仅丢弃挂载、缓存不受影响）；② 磁盘过期清理（§2.11 ⑥：启动扫描 `CleanupExpired`，文件名白名单隔离 ratelimit.json）；③ fetching 反馈（§2.12：`[Fetching lyrics...]`，旧结果不清除新 pending）；④ SSRF 私网地址仅警告（§2.3 `isPrivateHost`，不阻止以支持内网自建实例）；⑤ 范围外明确：翻译歌词/下一首预取/多 provider 故障转移/缓存清理命令；⑥ §5b 实施与提交计划（16 commits，英文 message，每 commit 编译+测试通过，`feat/online-lyrics` 分支）。
