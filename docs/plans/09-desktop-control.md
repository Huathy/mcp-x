# 09 Windows 桌面控制（鼠标/键盘/截图）

> 数据版本：2026-09-29
> 状态：设计定稿
> 关联：[06-tools-extension.md](06-tools-extension.md)、[08-doc-conversion.md](08-doc-conversion.md)
> 范围：为 mcp-x 新增 Windows 桌面控制类 MCP 工具，解决"鼠标点击/拖拽只能在 Chrome 里做"的限制

---

## 1. 背景与问题

当前 kilocode 的鼠标点击、拖拽能力全部依赖浏览器自动化（Chrome DevTools / Playwright），
**只能作用于 Chrome 页面内的元素**。遇到以下场景完全无能为力：

1. 操作原生 Windows 桌面程序（Excel、ERP 客户端、远程桌面窗口、文件管理器）
2. 操作非浏览器应用（QQ/微信客户端、Office 桌面版、VB/C# 写的内部工具）
3. 浏览器无法暴露的原生控件（下拉框、系统对话框、右键菜单）

**目标**：在 mcp-x 中新增 Windows 桌面控制工具，让 AI 能像操作 Chrome 一样操作整个桌面。

**核心诉求（来自用户）**：

> 为了防止乱点击，所以触发之前要先校验屏幕大小，同时提供获取屏幕大小的接口，
> 然后由 AI 计算窗口所在位置，AI 调用的时候传入参数（窗口坐标 X、窗口坐标 Y、
> 鼠标坐标 X、鼠标坐标 Y），要校验得严格一些。

---

## 2. 需求澄清与补全

用户原始描述中有些点未展开，本节逐条明确。**标注 `[补]` 的是我补的设计决策**。

### 2.1 坐标系定义 `[补·关键]`

用户给的四个参数需要明确定义，否则 AI 无法正确计算：

| 参数 | 含义 | 说明 |
|------|------|------|
| `window_x` / `window_y` | 目标窗口**左上角**的屏幕绝对坐标 | 必须与操作系统返回的真实窗口位置一致 |
| `mouse_x` / `mouse_y` | 目标点**相对窗口左上角**的偏移 | 与 `window_x/window_y` 同原点，非客户区、非屏幕绝对坐标 |

绝对坐标由服务端计算：`abs = window_x + mouse_x`，**AI 不允许直接传绝对坐标**。
这样做的目的：AI 传的是"我看到窗口在 (100,200)，我要点它里面 (30,40) 的位置"，
服务端可以拿真实窗口 rect 交叉验证 AI 的观察是否过期（窗口移动过 / 分辨率变过 / 窗口被最小化）。

### 2.2 为什么必须双重校验 `[补]`

只做"坐标是否在屏幕范围内"的校验是**不够的**，因为：

- 屏幕范围内有几十个窗口，AI 传的坐标可能落在另一个窗口上
- AI 观察之后窗口可能已被用户移动 / 关闭 / 最小化
- 多显示器场景下"屏幕范围"是虚拟桌面矩形，可能含负坐标

因此引入**三重交叉验证**：

1. **窗口身份验证**：按标题查找真实窗口，拿到 OS 返回的真实 rect
2. **位置一致性验证**：AI 传的 `window_x/window_y` 与真实 rect 偏差 ≤ `position_tolerance_px`（默认 2px）
3. **命中测试**：用 `WindowFromPoint(abs)` 反查该坐标实际命中的顶层窗口，必须与目标窗口一致（防遮挡误点）

### 2.3 焦点要求 `[补]`

点击一个失焦窗口上的坐标，实际生效的是当前前台窗口。
因此默认要求 `require_foreground: true`——目标窗口必须是前台窗口，否则拒绝并提示先激活。

### 2.4 范围决策

| 能力 | 工具 | 默认可用 | 说明 |
|------|------|---------|------|
| 屏幕信息 | `desktop_screen_info` | ✅ | 屏幕尺寸/虚拟桌面/显示器列表/DPI/当前鼠标位置/屏幕指纹 |
| 窗口枚举 | `desktop_list_windows` | ✅ | 可见顶层窗口（标题/类名/矩形/PID/进程名/前台标记） |
| 鼠标移动 | `desktop_mouse_move` | ✅ | 仅移动，不点击 |
| 鼠标点击 | `desktop_mouse_click` | ✅ | left/right/middle × 单击/双击 |
| 鼠标拖拽 | `desktop_mouse_drag` | ✅ | 起点→终点，插值移动模拟真实拖拽 |
| 滚轮 | `desktop_mouse_scroll` | ✅ | 垂直/水平，格数 |
| 键盘输入 | `desktop_type_text` / `desktop_key` | ❌ **需人工审核** | 默认关闭，开启后强制两阶段确认 |
| 截图 | `desktop_screenshot` | ✅ | 全屏/窗口/区域，PNG |

**键盘输入为什么默认关闭**：输入能力可直接触发"确定/删除/发送/付款"等不可逆操作，
风险等级显著高于鼠标点击。默认关闭 + 开启后强制 dry_run 确认，是刻意的摩擦设计。

### 2.5 明确不做（YAGNI）

| 不做 | 理由 |
|------|------|
| 键盘录制/宏/定时任务 | 与 MCP 单次调用模型不匹配，且放大误操作面 |
| OCR / 元素定位（UIA） | 需要截图 + 视觉模型协同，AI 已有视觉能力，直接用坐标 |
| 剪贴板读写 | 越权敏感，且 AI 已有文件工具可替代 |
| 任意组合键注入 | 用受控的 `with_ctrl/with_alt/with_shift` 布尔位代替，防止 AI 拼出任意热键 |
| 跨窗口拖拽（拖到另一个窗口） | 起点终点必须同窗口内，跨窗口语义歧义大、易误拖 |
| macOS/Linux 桌面控制 | 本期只做 Windows，跨平台抽象保留但不实现 |
| 后台窗口消息注入（PostMessage） | 绕过焦点校验，与"防乱点"目标冲突 |

---

## 3. 安全设计（本模块的核心）

防乱点击不是靠"提醒 AI 小心"，而是靠**服务端不可绕过的校验链**。任何一项不通过即拒绝执行。

### 3.1 校验链（按执行顺序）

```
MCP tool handler
  │
  ├─ C1  模块开关      desktop.enabled && desktop.<能力>.enabled
  ├─ C2  安全模式      safety.mode == read-write（复用 safety.CheckWrite）
  ├─ C3  参数完整性    四个坐标齐全；drag 需两对；数值类型合法
  ├─ C4  限流          max_actions_per_minute 滑动窗口
  ├─ C5  黑名单区域    目标点落在 blocked_regions → 拒绝
  │
  ├─ C6  窗口身份      按 window_title 查 OS 真实窗口
  │                   0 命中 / 多命中（歧义） → 拒绝
  ├─ C7  窗口状态      IsWindowVisible && !IsIconic && 宽高 > 0
  ├─ C8  位置一致性    |真实.left - window_x| <= position_tolerance_px
  │                   |真实.top  - window_y| <= position_tolerance_px
  │                   不一致 → 拒绝，提示重新调 desktop_list_windows
  │
  ├─ C9  相对边界      edge_margin <= mouse_x < 真实宽 - edge_margin
  │                   同理 mouse_y（防止点到窗口 resize 边框/最小化按钮）
  ├─ C10 屏幕边界      abs 落在虚拟桌面矩形内（支持负坐标多屏）
  ├─ C11 屏幕指纹      传入的 screen_token 与当前实时指纹一致（AI 观察未过期）
  ├─ C12 命中测试      WindowFromPoint(abs) 的根窗口 == 目标窗口（防遮挡）
  ├─ C13 焦点校验      require_foreground 时目标窗口须为前台窗口
  │
  ├─ C14 dry_run       true → 只回显计划，不执行
  ├─ C15 确认 token    confirm_required 时校验 plan_token（签名+TTL+参数绑定）
  │
  └─ 执行（Backend 串行化）→ 写审计日志
```

**关键点**：C6~C13 中任何一项失败都**不会移动鼠标**。AI 收到的是明确的中文错误 +
可操作的修复建议（"窗口已移动，请重新调用 desktop_list_windows"），而不是静默纠正。

### 3.2 拒绝时的错误信息设计 `[补]`

错误信息必须让 AI 能自我纠正，因此统一格式：

```
桌面操作被拒绝 [<CODE>]：<原因>
  目标窗口: "<title>"  传入位置: (wx, wy)  实际位置: (rx, ry)
  建议: <具体下一步>
```

拒绝码（C1~C15 各对应一个），便于 AI 与用户排查。

### 3.3 dry_run 与 plan_token 两阶段

- **dry_run**：所有写操作（move/click/drag/scroll/type/key）都接受 `dry_run: true`。
  走完整校验链（C1~C13），通过后**只回显**将要执行的绝对坐标与命中的窗口，不动鼠标。
- **plan_token**：`confirm_required: true` 时，仅 dry_run 有效，真实执行必须带 token。

token 是**无状态签名**（服务端内存随机密钥，HMAC-SHA256）：

```
payload = {"t": tool, "p": sha256(规范化参数), "s": screen_fingerprint, "e": unix过期时间}
token   = base64url(payload) + "." + base64url(HMAC-SHA256(secret, payload))
```

校验：签名正确 + 未过期 + `t` 与当前工具一致 + `p` 与本次参数一致 + `s` 与当前屏幕指纹一致。

参数绑定意味着 **token 不可挪用**（不能把 A 点击的 token 用在 B 拖拽上）；
屏幕指纹绑定意味着**换分辨率/拔显示器后 token 立即失效**，强制 AI 重新观察。

### 3.4 审计日志

每次调用（无论成功、拒绝、dry_run）都写一条 `slog` 结构化日志：

```
tool / action / window_title / passed_coords / real_coords / abs_point
/ screen_fingerprint / decision(allow|deny|preview) / deny_code / elapsed_ms
```

日志走 stderr（stdout 是 MCP 协议通道，不可污染），与项目现有 logger 一致。

### 3.5 串行化

`Controller` 持全局 `sync.Mutex`，所有 Backend 写操作串行执行。
防止 AI 并发发起多个点击导致鼠标位置竞态。

---

## 4. 架构设计

### 4.1 目录结构

```
internal/desktop/
├── desktop.go              # 类型、错误、Config、Controller、校验链、限流、token
├── desktop_test.go         # 校验链单元测试（fake backend，无需 Windows）
├── backend.go              # Backend 接口 + Available()
├── backend_windows.go      # //go:build windows —— user32/gdi32 实现
└── backend_other.go        # //go:build !windows —— stub，返回 ErrUnsupportedPlatform

internal/mcp/
└── tools_desktop.go        # MCP 工具注册 + handler
```

**分层理由**：校验链（C1~C15）是本模块的灵魂，必须**完全平台无关**才能被单测覆盖。
因此把"获取窗口/屏幕信息"抽象成 `Backend` 接口，Windows 实现细节全部下沉到
`backend_windows.go`，校验链本身跑在 `desktop.go` 里，用 fake backend 测。

### 4.2 Backend 接口

```go
type Backend interface {
    ScreenInfo() (ScreenInfo, error)                  // 屏幕/虚拟桌面/显示器/DPI
    ListWindows() ([]WindowInfo, error)               // 可见顶层窗口
    CursorPos() (int, int, error)                     // 当前鼠标位置
    WindowAtPoint(x, y int) (WindowInfo, bool, error) // 命中测试
    SetForeground(hwnd uintptr) error                 // 激活窗口
    MoveTo(x, y int) error
    MouseDown(btn string) error
    MouseUp(btn string) error
    MouseWheel(notches int) error
    TypeText(s string) error
    PressKey(name string, mod Mods) error
    Screenshot(rect Rect) (image.Image, error)        // 整屏或区域
}
```

### 4.3 平台隔离

沿用项目已有的 docconv 模式（`docx_com.go` + `docx_com_stub.go`）：

```go
// backend_windows.go
//go:build windows

// backend_other.go
//go:build !windows

func Available() bool { return false }
```

工具注册时 `if !desktop.Available() { return }`，非 Windows 平台完全看不到这些工具。

### 4.4 Win32 依赖

| DLL | 函数 | 用途 |
|-----|------|------|
| user32 | `SetProcessDpiAwarenessContext` | **必须先调**，否则高 DPI 下 rect 全部错位 |
| user32 | `GetSystemMetrics` / `SM_*` | 主屏、虚拟桌面 |
| user32 | `EnumWindows` / `GetWindowTextW` / `GetClassNameW` / `GetWindowRect` | 窗口枚举与定位 |
| user32 | `IsWindowVisible` / `IsIconic` / `GetForegroundWindow` | 窗口状态校验 |
| user32 | `WindowFromPoint` / `GetAncestor(GA_ROOT)` | 命中测试 |
| user32 | `SetCursorPos` / `GetCursorPos` | 鼠标移动 |
| user32 | `SendInput` | 点击/滚轮/按键（比 `mouse_event` 可靠，不受 UIPI 部分限制） |
| user32 | `GetDC` / `GetDpiForWindow` | 截图与 DPI |
| gdi32 | `CreateCompatibleDC` / `BitBlt(CAPTUREBLT)` / `GetDIBits` | 截图 |
| kernel32 | `GetModuleHandleW` | 解析自身模块名（SendInput 结构体对齐用） |

依赖库：`golang.org/x/sys/windows`（已是间接依赖，提升为直接依赖），纯 Go，无 cgo。

**DPI 感知是硬性前置**：进程若非 Per-Monitor DPI-Aware，`GetWindowRect` 返回的是
系统缩放后的虚拟化坐标，与 `SetCursorPos` 期望的物理坐标不一致，
在 125%/150% 缩放的屏幕上会导致**整体偏移**。必须在任何窗口 API 调用前设置。

### 4.5 截图实现

```
GetDC(NULL) → CreateCompatibleDC → CreateCompatibleBitmap(w,h)
  → SelectObject → BitBlt(SRCCOPY|CAPTUREBLT)  ← CAPTUREBLT 才能抓到分层窗口
  → GetDIBits → image.RGBA → 缩放(max_width) → png.Encode
```

返回两种形态：
- 默认：保存到 `desktop.workdir`，返回文件路径 + 尺寸
- `return_base64: true`：额外返回 MCP `ImageContent`（AI 可直接看图）

---

## 5. 工具规格

### 5.1 `desktop_screen_info`（只读）

**输入**：`{}`（无参数）

**输出**：Markdown 表格 + 机器可读字段

| 字段 | 说明 |
|------|------|
| `primary_width/height` | 主显示器分辨率（物理像素） |
| `virtual_x/y/width/height` | 虚拟桌面矩形，多屏时可能为负 |
| `dpi` / `scale` | 系统 DPI 与缩放比（1.0 = 100%） |
| `monitors[]` | 每屏：索引/主屏标记/矩形/DPI/设备名 |
| `cursor_x/y` | 当前鼠标位置 |
| `fingerprint` | 屏幕指纹，AI 后续调用须原样回传 |
| `foreground_window` | 当前前台窗口标题 |
| `desktop_enabled` | 各子能力开关状态 |

### 5.2 `desktop_list_windows`（只读）

**输入**：`filter`（标题子串过滤，可选）、`limit`（默认 50）、`include_offscreen`（默认 false）

**输出**：Markdown 表格

| 字段 | 说明 |
|------|------|
| `index` | 序号，供 AI 引用 |
| `title` | 窗口标题（**调用时须原样回传**） |
| `class` | 窗口类名 |
| `x/y/width/height` | 窗口矩形（= 调用时要传的 window_x/window_y/相对坐标基准） |
| `pid` / `process` | 进程 ID / 进程名 |
| `foreground` | 是否前台窗口 |
| `minimized` | 是否最小化（最小化窗口不可作为目标） |

### 5.3 `desktop_mouse_move`（写）

**输入**：`window_title`(必填)、`window_x`、`window_y`、`mouse_x`、`mouse_y`、`screen_token`(可选)、`dry_run`(可选)

**行为**：仅移动鼠标，不产生按键。

### 5.4 `desktop_mouse_click`（写）

**输入**：以上 + `button`（left/right/middle，默认 left）、`count`（1/2，默认 1）

**行为**：`SetCursorPos` → 短暂延迟 → `SendInput` 按下+抬起。`count=2` 发双击。

### 5.5 `desktop_mouse_drag`（写）

**输入**：`window_title`、`window_x`、`window_y`、`from_x`、`from_y`、`to_x`、`to_y`、`button`、`steps`(默认 20)、`duration_ms`(默认 300)、`screen_token`、`dry_run`

**行为**：`MoveTo(from)` → `MouseDown` → 插值 `steps` 步移动（每步 `duration/steps` 延时）→ `MouseUp`。

**限制**：起点与终点**必须都在同一目标窗口内**（各自独立走 C9/C10/C12），防止跨窗口误拖。

### 5.6 `desktop_mouse_scroll`（写）

**输入**：定位四参 + `direction`（up/down/left/right）、`notches`（默认 3，绝对值上限 50）

### 5.7 `desktop_type_text` / `desktop_key`（写，**默认关闭**）

- `desktop_type_text`：定位四参 + `text`（长度 ≤ `max_text_length`，默认 2000）
  - 走 Unicode 扫描码注入（`KEYEVENTF_UNICODE`），不依赖键盘布局，中文可直接输入
  - 4 字节以上字符按 UTF-16 代理对拆分
- `desktop_key`：定位四参 + `key`（固定白名单：`ENTER/TAB/ESC/BACKSPACE/DELETE/UP/DOWN/LEFT/RIGHT/HOME/END/PAGEUP/PAGEDOWN/F1~F12`）+ `with_ctrl` / `with_alt` / `with_shift` 三个布尔位
  - **不接受任意组合键字符串**，防止 AI 拼出 `Win+R` 之类高危热键

**人工审核要求**：`keyboard.enabled` 默认 `false`；开启后 `require_confirm` 强制为 `true`（dry_run + token 两阶段不可绕过）。

### 5.8 `desktop_screenshot`（只读）

**输入**：`scope`（`screen` / `window` / `region`，默认 screen）、`window_title`、`window_x`、`window_y`、`mouse_x`、`mouse_y`（region 与 window 需定位参数）、`max_width`（默认 1600）、`return_base64`（默认 false）

**安全**：`scope: screen` 全屏截图需 `allow_fullscreen: true`（默认 true，可关）。
`region` 同样走完整校验链，截不到窗口外的区域。

**输出**：文件路径（相对 workdir 的绝对路径）+ 尺寸 + 哈希；`return_base64` 时附 `ImageContent`。

---

## 6. 配置规格

```yaml
desktop:
  enabled: false                # 总开关，默认关闭；不开启则不注册任何工具

  # —— 通用安全参数 ——
  max_actions_per_minute: 60    # 限流；0 = 不限
  position_tolerance_px: 2      # AI 传的位置与 OS 真实位置的最大允许偏差
  require_foreground: true      # 目标窗口必须是前台窗口
  edge_margin: 1                # 距窗口边缘至少 N px，防误点 resize 边框/最小化按钮
  confirm_required: false       # true 时写操作必须先 dry_run 拿 plan_token
  confirm_ttl: 60s              # plan_token 有效期
  dpi_aware: true               # 进程 Per-Monitor DPI 感知（关闭会导致高 DPI 偏移）
  blocked_regions:              # 禁止操作的屏幕矩形（绝对坐标）
    - x: 0
      y: 1040
      width: 1920
      height: 40
      reason: "任务栏/系统托盘，误点会最小化全部窗口"

  # —— 分能力开关 ——
  mouse:
    enabled: true
    allow_buttons: ["left", "right", "middle"]
    allow_double_click: true
    allow_drag: true
    drag_steps: 20
    drag_duration_ms: 300
  keyboard:
    enabled: false              # 需人工审核后手动开启
    require_confirm: true       # 开启后强制两阶段，不可通过配置绕过
    max_text_length: 2000
  screenshot:
    enabled: true
    workdir: "./data/desktop"
    max_width: 1600
    allow_fullscreen: true
```

配置解析挂到 `Config.Desktop DesktopConfig`（`yaml: "desktop"`），
`applyDefaults()` 补默认值，**默认全部关闭**（`Enabled: false`）。

---

## 7. 测试计划

### 7.1 单元测试（`internal/desktop/desktop_test.go`，平台无关）

用 fake backend 覆盖校验链全部拒绝路径：

| ID | 场景 | 期望 |
|----|------|------|
| T-1201 | 参数缺失（window_title 空） | DENY_PARAM |
| T-1202 | 坐标负数/超出 int 范围 | DENY_PARAM |
| T-1203 | 限流超限 | DENY_RATE_LIMIT |
| T-1204 | 目标点落在 blocked_regions | DENY_BLOCKED_REGION |
| T-1205 | 窗口标题 0 命中 | DENY_WINDOW_NOT_FOUND |
| T-1206 | 窗口标题多命中（歧义） | DENY_WINDOW_AMBIGUOUS |
| T-1207 | 窗口已最小化 | DENY_WINDOW_MINIMIZED |
| T-1208 | 窗口不可见 | DENY_WINDOW_INVISIBLE |
| T-1209 | window_x 与真实位置差 3px（tol=2） | DENY_WINDOW_MOVED |
| T-1210 | window_x 差 1px（tol=2） | 放行 |
| T-1211 | mouse_x 超出窗口宽 | DENY_OUT_OF_WINDOW |
| T-1212 | mouse_x == edge_margin-1 | DENY_OUT_OF_WINDOW |
| T-1213 | abs 落在虚拟桌面外（多屏负坐标） | DENY_OUT_OF_SCREEN |
| T-1214 | screen_token 与实时指纹不符 | DENY_SCREEN_CHANGED |
| T-1215 | 命中测试返回另一窗口（被遮挡） | DENY_OCCLUDED |
| T-1216 | 非前台窗口 + require_foreground | DENY_NOT_FOREGROUND |
| T-1217 | 正常点击 | ALLOW，且 Backend 收到正确绝对坐标 |
| T-1218 | dry_run=true | DECISION_PREVIEW，Backend 零调用 |
| T-1219 | confirm_required 无 token | DENY_CONFIRM_REQUIRED |
| T-1220 | token 参数被篡改（挪用） | DENY_TOKEN_INVALID |
| T-1221 | token 过期 | DENY_TOKEN_EXPIRED |
| T-1222 | token 屏幕指纹过期 | DENY_TOKEN_SCREEN_STALE |
| T-1223 | 合法 token | ALLOW |
| T-1224 | drag 起点在窗口内、终点在窗口外 | DENY_OUT_OF_WINDOW |
| T-1225 | 键盘未启用 | DENY_DISABLED |
| T-1226 | 截图 region 越界 | DENY_OUT_OF_SCREEN |
| T-1227 | 拒绝时 Backend 零副作用 | 断言所有 Backend 方法未被调用 |

### 7.2 集成测试（需 Windows 桌面环境，手工）

| ID | 场景 | 期望 |
|----|------|------|
| T-1250 | 非 Windows 平台 | 工具完全不注册 |
| T-1251 | Windows + `desktop.enabled: false` | 工具完全不注册 |
| T-1252 | 真实窗口（记事本）点击 | 光标落到预期位置 |
| T-1253 | 125% 缩放屏幕 | 坐标不偏移（验证 DPI 感知） |
| T-1254 | 双显示器含负坐标 | 跨屏定位正确 |
| T-1255 | 拖拽文件/滑块 | 真实拖拽生效 |
| T-1256 | 截图 | PNG 可打开，分辨率正确，能抓到分层窗口 |
| T-1257 | 中文输入 | `desktop_type_text` 正确输入中文 |
| T-1258 | mcp-x 运行中用户移动目标窗口 | 下一次点击被 C8 拒绝 |

---

## 8. 交付物清单

- [ ] `internal/desktop/desktop.go` — 类型、错误、Config、Controller、校验链、限流、token
- [ ] `internal/desktop/backend.go` — `Backend` 接口 + `Available()`
- [ ] `internal/desktop/backend_windows.go` — `//go:build windows`
- [ ] `internal/desktop/backend_other.go` — `//go:build !windows`
- [ ] `internal/desktop/desktop_test.go` — 27 条单元测试
- [ ] `internal/mcp/tools_desktop.go` — 9 个 MCP 工具注册 + handler
- [ ] `internal/config/config.go` — 新增 `DesktopConfig` + 默认值
- [ ] `internal/config/config_test.go` — 补 desktop 配置解析用例
- [ ] `internal/mcp/server.go` — `registerTools()` 增加 `registerDesktopTools()`
- [ ] `examples/mcp_x.yaml.example` — desktop 配置示例（默认注释掉）
- [ ] `README.md` / `README_EN.md` — 工具列表与配置说明
- [ ] `cmd/mcp-x/main.go` — helpText 补充桌面控制说明
- [ ] `go.mod` — `golang.org/x/sys` 提升为直接依赖

---

## 9. 实施顺序与工时

| 阶段 | 内容 | 工时 | 前置 |
|------|------|------|------|
| D1 | `desktop.go` 类型/错误/Config/校验链 + fake backend 测试 | 1 天 | - |
| D2 | 限流 + plan_token + 审计日志 | 0.5 天 | D1 |
| D3 | `backend_windows.go`（屏幕/窗口枚举/命中测试/DPI） | 1 天 | D1 |
| D4 | `backend_windows.go`（SendInput 鼠标/键盘 + 截图） | 1 天 | D3 |
| D5 | `backend_other.go` stub + 跨平台编译验证 | 0.5 天 | D3 |
| D6 | `tools_desktop.go` 工具注册 + handler | 0.5 天 | D2,D4 |
| D7 | config + examples + README + helpText | 0.5 天 | D6 |
| **合计** | | **5 天** | |

D1 先行的理由：校验链是核心资产，先用 fake backend 测透，
再填 Windows 实现，避免在 Win32 调试过程中反复怀疑是校验逻辑还是平台调用的问题。

---

## 10. 已知限制与诚实声明

1. **需要交互桌面会话**：MCP Server 跑在用户会话（Session 1）才有效。
   作为 Windows 服务（Session 0）运行无法操作桌面，会返回空窗口列表。
2. **UIPI 权限边界**：若目标窗口以管理员权限运行，而 mcp-x 不是管理员，
   Windows 的 UI 隔离会阻止输入注入。表现为"校验通过但点击无反应"。
   需让 mcp-x 与目标程序同权限级别运行。
3. **坐标是物理像素**：高 DPI 多屏 + 不同缩放比例的显示器混排时，
   各屏坐标系不同（虚拟桌面统一为物理像素，但各屏缩放比不同）。
   本模块按虚拟桌面物理像素工作，工具描述中已标注。
4. **拖拽目标不透明**：本模块只负责"按下-移动-抬起"，不保证目标应用接受拖拽语义。
   有些应用需要更慢的移动或先 hover 生效，通过调大 `steps` / `duration_ms` 缓解。
5. **无元素语义**：只能坐标操作，无法确认"点到了按钮"还是"点到了旁边的空白"。
   截图 + AI 视觉判断是闭环手段，服务端不做元素识别。
6. **键盘输入默认关闭**：这是刻意的安全摩擦，需要用户显式评估风险后开启。
7. **学习/研究用途**：本项目定位是学习与研究，桌面自动化能力请在有授权的环境中使用。
