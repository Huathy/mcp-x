# 14 桌面控制测试子计划

> 日期：2026-09-29
> 关联：[09-desktop-control.md](../09-desktop-control.md)
> 范围：9 个 desktop MCP 工具 + 13 步校验链 + Windows 后端

## 1. 工具清单

| ID | 工具 | 平台 | 默认注册 |
|----|------|------|---------|
| D1 | desktop_screen_info | Windows | 是 |
| D2 | desktop_list_windows | Windows | 是 |
| D3 | desktop_mouse_move | Windows | 需 `mouse.enabled` |
| D4 | desktop_mouse_click | Windows | 需 `mouse.enabled` |
| D5 | desktop_mouse_drag | Windows | 需 `mouse.enabled` + `allow_drag` |
| D6 | desktop_mouse_scroll | Windows | 需 `mouse.enabled` |
| D7 | desktop_type_text | Windows | 需 `keyboard.enabled`（默认关） |
| D8 | desktop_key | Windows | 需 `keyboard.enabled`（默认关） |
| D9 | desktop_screenshot | Windows | 需 `screenshot.enabled` |

## 2. 单元测试（已实现，`internal/desktop/desktop_test.go`）

用 fake `Backend` 驱动，无需 Windows 环境。每条拒绝路径都同时断言
"返回了正确的拒绝码" + "Backend 一次都没被改动桌面"。

### 2.1 放行路径

| ID | 用例 | 期望 |
|----|------|------|
| T-1401 | 正常点击 | 放行，Backend 收到绝对坐标 (140,260) |
| T-1402 | 位置漂移 1px（tol=2） | 放行 |
| T-1403 | 负坐标显示器上的点击 | 放行，绝对坐标为负值 |
| T-1404 | `require_foreground: false` 时点后台窗口 | 放行 |
| T-1405 | `dry_run=true` | 返回 plan_token，Backend 零改动 |
| T-1406 | dry_run 后带 token 执行 | 放行，且只执行一次 |
| T-1407 | 双击（count=2） | Backend 收到 2 次按下 |
| T-1408 | 拖拽插值 | MoveTo 调用 1+steps 次，末点等于终点，仅 1 次按下/抬起 |
| T-1409 | 滚轮 up/down | 下滚传负值，上滚传正值 |
| T-1410 | 白名单按键 | 转发到 Backend |
| T-1411 | 键盘两阶段 | 无 token 拒绝；dry_run 后带 token 放行 |
| T-1412 | 全屏截图 | 截取虚拟桌面矩形 |
| T-1413 | 窗口截图 | 截取窗口 rect |
| T-1414 | dry_run 不消耗限流额度 | 3 次预演后再执行仍成功 |
| T-1415 | 限流窗口滑出后恢复 | 超过 1 分钟后额度重置 |
| T-1416 | 截断标题唯一时子串匹配 | 放行 |
| T-1417 | 鼠标移动不按任何键 | 只有 MoveTo |
| T-1418 | 屏幕指纹随布局变化 | 改变分辨率后指纹不同 |

### 2.2 拒绝路径

| ID | 用例 | 拒绝码 |
|----|------|--------|
| T-1420 | `window_title` 为空 | DENY_PARAM |
| T-1421 | 相对坐标为负 | DENY_PARAM |
| T-1422 | `count=5` | DENY_PARAM |
| T-1423 | `direction` 非法 | DENY_PARAM |
| T-1424 | `notches` 超上限 | DENY_PARAM |
| T-1425 | 按键不在白名单（`WIN+R`） | DENY_PARAM |
| T-1426 | 拖拽起终点相同 | DENY_PARAM |
| T-1427 | 窗口截图缺定位参数 | DENY_PARAM |
| T-1428 | scope 非法 | DENY_PARAM |
| T-1429 | `safety.mode: read-only` | DENY_SAFETY_MODE |
| T-1430 | `desktop.enabled: false` | DENY_DISABLED |
| T-1431 | `mouse.enabled: false` | DENY_DISABLED |
| T-1432 | `keyboard.enabled: false`（默认） | DENY_DISABLED |
| T-1433 | `allow_double_click: false` 时双击 | DENY_DISABLED |
| T-1434 | `allow_drag: false` 时拖拽 | DENY_DISABLED |
| T-1435 | `screenshot.enabled: false` | DENY_DISABLED |
| T-1436 | 键盘已开但未带 token | DENY_CONFIRM_REQUIRED |
| T-1437 | 超过每分钟上限 | DENY_RATE_LIMIT |
| T-1438 | 目标点在 `blocked_regions` | DENY_BLOCKED_REGION |
| T-1439 | 窗口标题 0 命中 | DENY_WINDOW_NOT_FOUND |
| T-1440 | 窗口标题多命中 | DENY_WINDOW_AMBIGUOUS |
| T-1441 | 窗口已最小化 | DENY_WINDOW_MINIMIZED |
| T-1442 | 窗口不可见 | DENY_WINDOW_INVISIBLE |
| T-1443 | 窗口位置漂移 3px（tol=2） | DENY_WINDOW_MOVED |
| T-1444 | 相对坐标超出窗口宽 | DENY_OUT_OF_WINDOW |
| T-1445 | 相对坐标落在 edge_margin 内 | DENY_OUT_OF_WINDOW |
| T-1446 | 拖拽终点超出窗口 | DENY_OUT_OF_WINDOW |
| T-1447 | region 截图中心点在窗口外 | DENY_OUT_OF_WINDOW |
| T-1448 | 绝对坐标超出虚拟桌面右界 | DENY_OUT_OF_SCREEN |
| T-1449 | 绝对坐标在虚拟桌面原点左侧 | DENY_OUT_OF_SCREEN |
| T-1450 | `screen_token` 与实时指纹不符 | DENY_SCREEN_CHANGED |
| T-1451 | 该点被另一窗口遮挡 | DENY_OCCLUDED |
| T-1452 | 该点无任何窗口 | DENY_OCCLUDED |
| T-1453 | 拖拽终点被遮挡 | DENY_OCCLUDED |
| T-1454 | 非前台窗口 + `require_foreground` | DENY_NOT_FOREGROUND |
| T-1455 | 按钮不在 `allow_buttons` | DENY_BUTTON_NOT_ALLOWED |
| T-1456 | 文本超长 | DENY_TEXT_TOO_LONG |
| T-1457 | `allow_fullscreen: false` 时全屏截图 | DENY_FULLSCREEN_BLOCKED |
| T-1458 | 无显示器（Session 0） | DENY_NO_INTERACTIVE_DESKTOP |
| T-1459 | token 挪用给不同坐标 | DENY_TOKEN_INVALID |
| T-1460 | token 挪用给不同工具 | DENY_TOKEN_INVALID |
| T-1461 | token 被篡改 | DENY_TOKEN_INVALID |
| T-1462 | token 过期 | DENY_TOKEN_EXPIRED |
| T-1463 | token 绑定后分辨率变化 | DENY_TOKEN_SCREEN_STALE |

### 2.3 拒绝时零副作用（核心不变量）

T-1401~T-1463 中每条 `wantDeny` 用例都由 `assertNoMutation` 断言
`MoveTo / MouseDown / MouseUp / MouseWheel / TypeText / PressKey`
一次都没有被调用。这是"防乱点击"设计的可验证形式：拒绝发生在任何平台调用之前。

### 2.4 Win32 结构布局（`internal/desktop/backend_windows_test.go`）

| ID | 用例 | 期望 |
|----|------|------|
| T-1470 | `sizeof(INPUT)` | 40 字节（x64） |
| T-1471 | `sizeof(MOUSEINPUT)` | 32 字节 |
| T-1472 | `sizeof(KEYBDINPUT)` | 24 字节 |
| T-1473 | union 偏移 | 8 字节（4 字节 type + 对齐填充） |
| T-1474 | 键盘 overlay 不超出 union | 通过 |
| T-1475 | `Available()` / `NewBackend` | Windows 上成功返回 |

## 3. 配置解析测试（`internal/config/config_test.go`）

| ID | 用例 | 期望 |
|----|------|------|
| T-1480 | 缺 `desktop:` 块 | 全部关闭，`dpi_aware`/`allow_fullscreen` 默认 true |
| T-1481 | 显式 `dpi_aware: false` | 不被默认值覆盖 |
| T-1482 | 显式 `allow_fullscreen: false` | 不被默认值覆盖 |
| T-1483 | 完整配置 | 各字段全部按 YAML 解析 |
| T-1484 | 部分配置 | 未写的字段保留默认值 |

## 4. 集成测试（需 Windows 桌面会话，手工执行）

| ID | 场景 | 期望 |
|----|------|------|
| T-1500 | 非 Windows 平台 | 工具完全不注册 |
| T-1501 | Windows + `desktop.enabled: false` | 工具完全不注册 |
| T-1502 | 真实窗口（记事本）点击 | 光标落到预期位置 |
| T-1503 | 125% 缩放屏幕 | 坐标不偏移（验证 DPI 感知） |
| T-1504 | 双显示器含负坐标 | 跨屏定位正确 |
| T-1505 | 拖拽文件/滑块 | 真实拖拽生效 |
| T-1506 | 截图 | PNG 可打开，分辨率正确，能抓到分层窗口（CAPTUREBLT） |
| T-1507 | 中文输入 | `desktop_type_text` 正确输入中文 |
| T-1508 | 运行中移动目标窗口 | 下一次点击被 DENY_WINDOW_MOVED 拒绝 |
| T-1509 | 目标窗口被其他窗口覆盖 | 被 DENY_OCCLUDED 拒绝 |
| T-1510 | 以管理员运行的目标程序 + 非管理员 mcp-x | 输入注入被 UIPI 拒绝（已知限制，需在文档中说明） |
| T-1511 | Session 0 服务运行 | 窗口列表为空，鼠标工具被 DENY_NO_INTERACTIVE_DESKTOP 拒绝 |
| T-1512 | `return_base64: true` 截图 | 返回 `ImageContent`，AI 可直接看图 |

## 5. 已实现状态（2026-09-29）

- 单元测试 T-1401~T-1475 全部通过（70 条）
- 配置测试 T-1480~T-1484 全部通过（5 条）
- T-1500~T-1512 需 Windows 桌面环境，留作集成测试
- 跨平台编译已验证：Linux amd64 与 darwin arm64 的 `internal/desktop` 均可编译

## 6. 回归

`internal/desktop` 是独立包，不触碰 driver 层。现有 66 条 SQL/Redis/DocStore/Object 用例不受影响。

修复记录：`internal/docconv/docx_com_stub.go` 的 stub 函数名曾与 `docx_com.go` 的导出名不一致
（`comProbe` vs `ComProbe`），导致非 Windows 平台编译失败。已修正，该问题与本模块无关但会阻断跨平台构建。
