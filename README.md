# mcp-x

通用数据库 MCP Server，让 AI 编程助手（kilocode / cursor / claude code 等）通过 MCP 协议安全操作数据库。

> 前身项目：mcp-dbx（已重命名为 mcp-x）

## 特性

- **多数据库开箱即用**：MySQL、PostgreSQL、达梦、金仓、Redis、Elasticsearch、MongoDB、MinIO 对象存储
- **文档转换**：Excel↔Markdown、PDF↔Markdown、DOCX↔Markdown 等格式互转（需显式启用）
- **Windows 桌面控制**：鼠标移动/点击/拖拽/滚轮、键盘输入、截图，把 AI 的操作范围从浏览器扩展到整个桌面（默认关闭，需显式启用）
- **安全层**：默认只读；写操作需显式配置；危险关键词拦截；DELETE/UPDATE 缺 WHERE 拦截；行数限制 + 查询超时；对象存储 bucket/key 合法性校验
- **stdio 传输**，kilocode / cursor 本地直连
- **YAML 配置**，多数据源命名管理，按名称路由
- **单一二进制**，零依赖部署

## 快速开始

### 1. 编译

```bash
# 需 Go 1.25+
make build
# 或直接
go build -o bin/mcp-x ./cmd/mcp-x
```

跨平台编译：

```bash
# Windows
$env:GOOS="windows"; $env:GOARCH="amd64"; go build -o bin/mcp-x.exe ./cmd/mcp-x
# Linux
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o bin/mcp-x ./cmd/mcp-x
# macOS
$env:GOOS="darwin"; $env:GOARCH="arm64"; go build -o bin/mcp-x ./cmd/mcp-x
```

### 2. 编写配置

复制 `examples/mcp-x.yaml.example` 并修改：

```yaml
server:
  name: "mcp-x"
  version: "0.1.0"

safety:
  mode: "read-write"        # read-only | read-write
  max_rows: 1000
  query_timeout: 30s
  blocked_keywords: ["DROP", "TRUNCATE", "GRANT", "REVOKE", "ALTER"]
  blocked_commands: ["FLUSHALL", "FLUSHDB", "CONFIG", "SHUTDOWN", "KEYS"]

datasources:
  - name: "main-mysql"
    driver: "mysql"
    dsn: "user:password@tcp(127.0.0.1:3306)/mydb?charset=utf8mb4&parseTime=true"
    max_open_conns: 10
    max_idle_conns: 5
    conn_max_lifetime: 5m
    safety:
      mode: "read-only"     # 此数据源强制只读

  - name: "main-postgres"
    driver: "postgres"
    dsn: "postgres://user:password@127.0.0.1:5432/mydb?sslmode=disable"
    max_open_conns: 10
    max_idle_conns: 5
    conn_max_lifetime: 5m
    safety:
      mode: "read-only"

  - name: "main-kingbase"
    driver: "kingbase"
    dsn: "postgres://system:123456@127.0.0.1:54321/test?sslmode=disable"
    max_open_conns: 10
    max_idle_conns: 5
    conn_max_lifetime: 5m
    safety:
      mode: "read-only"

  - name: "main-dameng"
    driver: "dameng"
    dsn: "dm://SYSDBA:SYSDBA@127.0.0.1:5236?autoCommit=true"
    max_open_conns: 10
    max_idle_conns: 5
    conn_max_lifetime: 5m
    safety:
      mode: "read-only"

  - name: "cache-redis"
    driver: "redis"
    addr: "127.0.0.1:6379"
    password: ""
    db: 0
    pool_size: 10
    safety:
      mode: "read-write"

  - name: "main-es"
    driver: "elasticsearch"
    addrs:
      - "http://127.0.0.1:9200"
    username: ""
    password: ""
    index_name: "my-index"
    safety:
      mode: "read-only"

  - name: "main-mongo"
    driver: "mongodb"
    dsn: "mongodb://user:password@127.0.0.1:27017/mydb"
    database: "mydb"
    bucket: "documents"
    pool_size: 10
    safety:
      mode: "read-only"

  - name: "main-minio"
    driver: "minio"
    endpoint: "127.0.0.1:9000"
    access_key: "<your-access-key>"
    secret_key: "<your-secret-key>"
    use_ssl: false
    region: "us-east-1"
    bucket: "mcp-x"
    safety:
      mode: "read-write"

docconv:
  enabled: true
  workdir: "./data/docconv"
  max_file_size_mb: 50
  com:
    prog_id: "auto"          # auto = probe Word.Application / kwps / wps
    timeout: 60s

desktop:
  enabled: false               # 默认关闭，见下文「桌面控制」
  max_actions_per_minute: 60
  position_tolerance_px: 2
  require_foreground: true
  edge_margin: 1
  confirm_required: false
  confirm_ttl: 60s
  dpi_aware: true
  mouse:
    enabled: true
  keyboard:
    enabled: false             # 需人工审核后开启
    require_confirm: true
  screenshot:
    enabled: true
    workdir: "./data/desktop"
    max_width: 1600
```

### 3. 手动验证

```bash
# 查看帮助
./bin/mcp-x --help

# 启动并发送 MCP JSON-RPC 测试
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | ./bin/mcp-x --config ./examples/mcp-x.yaml.example
```

正常会返回 `initialize` 响应，包含 server capabilities。

### 4. 文档转换模块（可选）

文档转换功能需在配置中显式启用：

```yaml
docconv:
  enabled: true
  workdir: "./data/docconv"     # 临时文件目录
  max_file_size_mb: 50          # 最大文件大小
  com:
    prog_id: "auto"             # auto = probe Word.Application / kwps / wps
    timeout: 60s                 # COM 调用超时
```

Windows 下需安装 WPS 或 Microsoft Office；Linux 下需额外配置 LibreOffice。

### 5. 桌面控制（可选，仅 Windows）

kilocode 自带的鼠标点击/拖拽只在 Chrome 里生效。开启本模块后，AI 可以操作整个 Windows 桌面：
原生应用（Excel、ERP 客户端、文件管理器）、非浏览器程序、浏览器无法暴露的原生控件。

**默认关闭**，因为启用后 AI 拥有移动真实鼠标、按下真实按键的能力。

```yaml
desktop:
  enabled: true
  max_actions_per_minute: 60
  position_tolerance_px: 2     # AI 传的窗口位置与真实位置的最大允许偏差
  require_foreground: true     # 目标窗口必须是前台窗口
  edge_margin: 1               # 拒绝距窗口边缘 1px 内的点（避开 resize 边框与关闭按钮）
  confirm_required: false      # true：所有写操作必须先 dry_run 预演并回传 plan_token
  blocked_regions:             # 永不触碰的屏幕区域
    - { x: 0, y: 1040, width: 1920, height: 40, reason: "任务栏" }
  mouse:
    enabled: true
    allow_double_click: true
    allow_drag: true
  keyboard:
    enabled: false             # 需人工审核：输入可直接触发「确定 / 删除 / 发送」
    require_confirm: true
  screenshot:
    enabled: true
    max_width: 1600
```

#### 防乱点击：坐标如何被校验

AI **不允许传绝对坐标**。它传的是：

| 参数 | 含义 |
|------|------|
| `window_x` / `window_y` | 目标窗口左上角的屏幕坐标 |
| `mouse_x` / `mouse_y` | 目标点相对该窗口左上角的偏移 |

服务端据此算出绝对坐标，并在执行前跑完一条校验链。**任何一项不通过都不会移动鼠标**：

1. 模块与子能力开关
2. `safety.mode` 必须为 read-write
3. 参数完整、非负、类型合法
4. 每分钟操作次数限流
5. 目标点不在 `blocked_regions` 内
6. 按标题查找 OS 真实窗口，0 命中 / 多命中（歧义）都拒绝
7. 窗口可见、未最小化、尺寸合法
8. **位置一致性**：AI 传的 `window_x/window_y` 与 OS 返回的真实位置偏差 ≤ `position_tolerance_px`。
   窗口在 AI 观察之后被移动过，这里就会拦下
9. 相对坐标落在窗口内部，且距离边缘 ≥ `edge_margin`
10. 绝对坐标落在虚拟桌面内（支持负坐标多屏）
11. **屏幕指纹一致**：`desktop_screen_info` 返回的指纹变了（改分辨率、拔显示器）则拒绝
12. **命中测试**：`WindowFromPoint` 反查该坐标实际命中的顶层窗口，必须就是目标窗口（防遮挡误点）
13. 目标窗口是前台窗口（`require_foreground`）

拒绝时返回带拒绝码的可操作错误，例如：

```
桌面操作被拒绝 [DENY_WINDOW_MOVED]：窗口位置已变化，OS 返回 (103,200 800x600)，传入 (100,200)，容差 2px
  传入位置: (100,200)  实际位置: (103,200 800x600)
  建议: 重新调用 desktop_list_windows 获取最新坐标后重试
```

#### 预演与两阶段确认

所有写操作都支持 `dry_run=true`：走完整条校验链，回显将要使用的绝对坐标与命中的窗口，但**不动鼠标**。

配置 `confirm_required: true` 后，写操作必须先 `dry_run` 拿到 `plan_token`，再原样回传才能执行。
token 用 HMAC-SHA256 签名，并绑定工具名、全部参数、屏幕指纹与过期时间，因此不可挪用、不可跨工具复用、
屏幕一变就失效。`desktop_type_text` / `desktop_key` 在键盘能力开启后强制走这条流程。

#### 运行环境要求

- 必须在**交互式桌面会话（Session 1）**中运行；作为 Windows 服务（Session 0）运行拿不到窗口列表
- mcp-x 与目标程序需处于**相同权限级别**；以管理员运行的窗口会拒绝非管理员进程的输入注入
- 高 DPI 缩放屏幕依赖 `dpi_aware: true`，否则坐标整体偏移

### 6. 接入 AI 编程助手

#### kilocode

`.kilocode/mcp.json`：

```json
{
  "mcpServers": {
    "mcp-x": {
      "command": "/path/to/mcp-x",
      "args": ["--config", "/path/to/mcp-x.yaml"],
      "env": {}
    }
  }
}
```

#### cursor

`.cursor/mcp.json`：

```json
{
  "mcpServers": {
    "mcp-x": {
      "command": "/path/to/mcp-x",
      "args": ["--config", "/path/to/mcp-x.yaml"]
    }
  }
}
```

重启 IDE 后，AI 可直接调用数据库工具。

## 工具列表

共 39 个工具（桌面控制 9 个默认关闭，仅 Windows 注册）：

### 通用工具

| 工具 | 说明 |
|------|------|
| `db_list` | 列出所有已配置数据源及状态 |
| `db_ping` | 健康检查，返回延迟 |

### SQL 工具（MySQL / PostgreSQL / 达梦 / 金仓）

| 工具 | 说明 |
|------|------|
| `db_query` | 执行 SELECT，返回 Markdown 表格，最大 1000 行 |
| `db_execute` | 执行 INSERT/UPDATE/DELETE，返回影响行数 |
| `db_tables` | 列出所有表 |
| `db_schema` | 查看表结构（列/类型/索引） |

### Redis 工具

| 工具 | 说明 |
|------|------|
| `redis_get` | 读取键值 |
| `redis_set` | 写入键值（支持 TTL） |
| `redis_del` | 删除键 |
| `redis_keys` | SCAN 扫描键（非阻塞，最大 100） |
| `redis_type` | 查看键类型 |
| `redis_ttl` | 查看过期时间 |

### 文档存储工具（Elasticsearch / MongoDB）

| 工具 | 说明 |
|------|------|
| `doc_list_indices` | 列出所有索引/集合 |
| `doc_search` | 全文搜索 |
| `doc_get` | 按 ID 获取文档 |
| `doc_index` | 索引/插入文档 |
| `doc_delete` | 删除文档 |

### 桌面控制工具（desktop，仅 Windows）

需 `desktop.enabled: true`。AI 传窗口原点 + 窗口内偏移，服务端从 Windows 读取真实窗口做交叉校验。

| 工具 | 说明 |
|------|------|
| `desktop_screen_info` | 屏幕分辨率、虚拟桌面、各显示器 DPI、当前鼠标位置、屏幕指纹、各能力开关 |
| `desktop_list_windows` | 可见顶层窗口（标题/类名/矩形/进程/前台标记），供后续调用取坐标 |
| `desktop_mouse_move` | 移动鼠标到窗口内某点（不点击） |
| `desktop_mouse_click` | 点击，支持 left/right/middle × 单击/双击 |
| `desktop_mouse_drag` | 同窗口内拖拽，插值移动；起终点都走校验 |
| `desktop_mouse_scroll` | 滚轮，up/down/left/right |
| `desktop_type_text` | 输入 Unicode 文本（含中文）。**默认关闭**，开启后强制两阶段确认 |
| `desktop_key` | 按下白名单按键 + 可选 Ctrl/Alt/Shift。不接受任意组合键字符串 |
| `desktop_screenshot` | 全屏 / 窗口 / 区域截图，PNG；可内联返回图片 |

### 对象存储工具（MinIO / S3 兼容）

| 工具 | 说明 |
|------|------|
| `obj_list_buckets` | 列出所有 bucket |
| `obj_list` | 列出 bucket 内对象 |
| `obj_get` | 下载对象 |
| `obj_put` | 上传对象 |
| `obj_delete` | 删除对象 |

### 文档转换工具（docconv）

| 工具 | 说明 |
|------|------|
| `excel_to_md` | Excel → Markdown |
| `md_to_excel` | Markdown → Excel |
| `pdf_to_md` | PDF → Markdown |
| `docx_to_md` | DOCX → Markdown |
| `md_to_docx` | Markdown → DOCX |
| `md_to_pdf` | Markdown → PDF |
| `word_to_pdf` | Word → PDF |
| `pdf_to_word` | PDF → Word |

## 配置说明

### 配置文件查找顺序

1. `--config <path>` 命令行指定
2. `./mcp-x.yaml` 当前目录
3. `~/.mcp-x/config.yaml` 用户目录

### 安全策略优先级

```
数据源 safety > 全局 safety
```

数据源配置 `safety` 字段覆盖全局。未配置则继承全局。

### Redis 集群配置

```yaml
datasources:
  - name: "cluster-redis"
    driver: "redis"
    mode: "cluster"       # standalone(默认) | cluster | sentinel
    addrs:                # 集群用 addrs（复数）
      - "10.0.0.1:6379"
      - "10.0.0.2:6379"
      - "10.0.0.3:6379"
    password: ""
    pool_size: 20
```

## 安全策略

| 风险 | 防护 |
|------|------|
| 误删数据 | DELETE/UPDATE 缺 WHERE 自动拦截 |
| 删表 / 清库 | DROP/TRUNCATE 默认拦截，可配置放行 |
| 全表扫描 | max_rows 限制 + 查询超时 |
| Redis KEYS 阻塞 | 强制用 SCAN 替代，KEYS 命令拦截 |
| 权限提升 | GRANT/REVOKE 默认拦截 |
| MinIO 大对象读取 | 限制 1MB，防止内存溢出 |
| MongoDB URI 注入 | 连接 URI 凭据自动 URL 转义 |
| Elasticsearch 注入 | 查询与文档体 JSON 格式校验 |
| 对象存储路径穿越 | bucket/key 合法性校验 + bucket 白名单 |
| MongoDB 通配符滥用 | Keys 搜索支持 glob 通配符，防止全集合扫描 |
| AI 误点桌面 | 窗口位置交叉校验 + 命中测试 + 焦点校验 + 屏幕指纹 + 禁点区域 + 限流，详见「桌面控制」 |
| 键盘误输入不可逆操作 | `desktop.keyboard.enabled` 默认关闭；开启后强制 dry_run + plan_token 两阶段 |
| 桌面操作失控 | `max_actions_per_minute` 滑动窗口限流；每次调用写结构化审计日志 |

安全层调用链：

```
tool handler
  -> safety.CheckWrite()                     # 读写模式检查
  -> safety.CheckSQL() / CheckRedisCommand() # 危险词拦截
  -> DELETE/UPDATE WHERE 检测
  -> driver.Query/Execute(ctxWithTimeout)     # 超时控制
  -> 对象存储 bucket/key 合法性校验
  -> MinIO 对象大小限制（1MB）
  -> 桌面控制：13 步校验链（开关/安全模式/限流/禁点区/窗口身份/窗口状态/
     位置一致性/窗口内边界/屏幕边界/屏幕指纹/命中测试/焦点/token）
```

## 测试环境

本地 Docker 起 MySQL + PostgreSQL + Redis + Elasticsearch + MinIO + MongoDB：

```bash
# MySQL
docker run -d --name mysql-test -e MYSQL_ROOT_PASSWORD=test -p 3306:3306 mysql:8

# PostgreSQL
docker run -d --name postgres-test -e POSTGRES_PASSWORD=test -p 5432:5432 postgres:15

# Redis
docker run -d --name redis-test -p 6379:6379 redis:7

# Elasticsearch
docker run -d --name es-test -p 9200:9200 -e "discovery.type=single-node" elasticsearch:8.11.0

# MinIO
docker run -d --name minio-test -p 9000:9000 -p 9001:9001 -e MINIO_ROOT_USER=minioadmin -e MINIO_ROOT_PASSWORD=minioadmin minio/minio server /data --console-address ":9001"

# MongoDB
docker run -d --name mongo-test -p 27017:27017 mongo:7
```

### 国产数据库

- **达梦 DM8**：需下载达梦官方 Docker 镜像，端口 5236
- **金仓 KingbaseES**：需下载金仓官方 Docker 镜像，端口 54321

## UTF8 多语言读写验证

MySQL DSN 配置 `charset=utf8mb4`，完整支持多语言 UTF8 读写。测试表 `zy_alarm`，字段 `err longtext`。

### 测试数据（id 830-847）

| id | type | ok | err |
|----|------|----|-----|
| 830 | threshold | Y | NULL |
| 831 | timeout | N | connection timed out after 30s |
| 832 | offline | N | device heartbeat lost |
| 835 | threshold | N | 温度超过阈值80度 |
| 836 | offline | N | 设备掉线：心跳超时未收到 |
| 837 | timeout | N | 网关连接超时：等待30秒无响应 |
| 840 | threshold | N | 温度が上昇値80度を超えました |
| 841 | offline | N | デバイスハートビート：サーバーから応答なし |
| 842 | timeout | N | タイムアウト発生：30秒遅延しました |
| 843 | threshold | N | 온도가 한계값 80도를 초과했습니다 |
| 844 | offline | N | 장치 연결 끊김：하트비트 응답 없음 |
| 845 | timeout | N | 게이트웨이 시간 초과：30초 지연 후 종료 |
| 846 | config | N | パラメータエラー：コンフィグバリデーションに不合格 |
| 847 | config | N | 配置错误：参数校验未通过 |

### 支持的语言

- 英文：`connection timed out after 30s`
- 中文：`温度超过阈值80度`
- 日文：`温度が上昇値80度を超えました`
- 韩文：`온도가 한계값 80도를 초과했습니다`
- 混合标点：`장치 연결 끊김：하트비트 응답 없음`（含全角冒号）

### 关键点

- DSN 必须用 `charset=utf8mb4`（非 `utf8`；后者只支持 BMP 3 字节，缺失部分 emoji 和生僻字）
- MySQL 表/列 `COLLATE` 建议用 `utf8mb4_general_ci` 或 `utf8mb4_unicode_ci`
- MCP JSON-RPC over stdio 原生传输 UTF8，无需额外编码

## 项目结构

```
mcp-x/
├── cmd/mcp-x/main.go           # 入口
├── internal/
│   ├── config/                 # YAML 配置解析
│   ├── driver/                 # Driver 接口 + 多数据库实现
│   │   ├── mysql/              # MySQL
│   │   ├── pglike/             # PostgreSQL 兼容层
│   │   ├── postgres/           # PostgreSQL
│   │   ├── kingbase/           # 金仓 KingbaseES
│   │   ├── dameng/             # 达梦 DM8
│   │   ├── redis/              # Redis
│   │   ├── elasticsearch/      # Elasticsearch
│   │   ├── mongodb/            # MongoDB
│   │   └── minio/              # MinIO 对象存储
│   ├── datasource/             # 数据源管理器
│   ├── safety/                 # 安全层
│   ├── docconv/                # 文档转换模块
│   ├── desktop/                # Windows 桌面控制（校验链 + Win32 后端）
│   └── mcp/                    # MCP server + 工具 handler
├── examples/                   # 配置示例
├── docs/                       # 设计文档
└── Makefile
```

## 扩展桌面控制

`internal/desktop` 分为两层：

- `desktop.go` / `actions.go` / `screenshot.go` — **平台无关**：类型、13 步校验链、限流、plan_token。
  这一层不碰任何 Win32 调用，因此可以用 fake `Backend` 做完整单测（`desktop_test.go`）
- `backend_windows.go` / `backend_other.go` — **平台相关**：`Backend` 接口的 user32/gdi32 实现与 stub

新增一种输入设备时，实现 `Backend` 的对应方法并在 `actions.go` 加一层校验即可；
`Backend` 接口本身就是跨平台抽象，将来支持 macOS/Linux 只需再加一个 `backend_*.go`。

## 扩展新数据库

1. 写 `internal/driver/xxx/xxx.go` 实现 `Driver`、`DocStoreDriver` 或 `ObjectStoreDriver` 接口
2. 在 `init()` 中调用 `driver.Register("xxx", ...)`
3. 在 `cmd/mcp-x/main.go` import `_ "github.com/yourname/mcp-x/internal/driver/xxx"`
4. 配置文件加数据源，`driver: xxx`

**MySQL 协议兼容的数据库**（OceanBase / TiDB）可直接复用 MySQL driver，仅改 driver 标识名即可。

## 扩展文档格式

文档转换模块支持通过 `docconv.enabled` 显式启用，支持：
- Excel↔Markdown
- PDF↔Markdown
- DOCX↔Markdown
- Markdown→Excel/PDF/DOCX
- Word→PDF / PDF→Word

需额外依赖：go-ole（COM 自动化）、excelize、goldmark、gopdf、ledongthuc/pdf

## 开发

```bash
make build     # 编译
make test      # 测试
make run       # 编译+运行
make clean     # 清理
```

### 测试覆盖

- 8 个 Driver（MySQL、PostgreSQL、金仓、达梦、Redis、Elasticsearch、MongoDB、MinIO）
- 39 个 MCP 工具（桌面控制 9 个需显式启用且仅 Windows 注册）
- 66 条集成测试用例（含文档转换 22 条）
- 桌面控制 70 条单元测试（校验链全拒绝路径 + Win32 结构布局断言）

## 路线图

| 版本 | 内容 |
|------|------|
| v0.1.0 | MySQL + Redis + 安全层，stdio 传输 ✅ |
| v0.2.0 | PostgreSQL、达梦、金仓、Elasticsearch、MinIO、MongoDB 支持 ✅ |
| v0.3.0 | 文档转换模块（Excel↔Markdown、PDF↔Markdown、DOCX↔Markdown）✅ |
| v0.3.1 | Windows 桌面控制模块（鼠标/键盘/截图，严格校验防误点）✅ |
| v0.4.0 | HTTP/SSE 传输、连接池监控、慢查询日志 |
| v0.5.0 | 审计日志、更多国产数据库支持 |

## 技术栈

- Go 1.25+
- [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk) v1.7.0（官方 SDK）
- [go-sql-driver/mysql](https://github.com/go-sql-driver/mysql) v1.10.1
- [go-redis/v9](https://github.com/redis/go-redis) v9.22.0
- [elastic/go-elasticsearch](https://github.com/elastic/go-elasticsearch) v8.15.0
- [minio/minio-go](https://github.com/minio/minio-go) v7.0.70
- [mongodb/mongo-go-driver](https://github.com/mongodb/mongo-go-driver) v1.17.1
- [lib/pq](https://github.com/lib/pq) v1.10.9（PostgreSQL）
- [excelize](https://github.com/xuri/excelize/v2) v2.8.0
- [goldmark](https://github.com/yuin/goldmark) v1.7.4
- [gopdf](https://github.com/phpdave11/gofpdf) v1.4.3
- [ledongthuc/pdf](https://github.com/ledongthuc/pdf) v0.8.0
- `log/slog` 标准库结构化日志
