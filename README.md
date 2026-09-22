# mcp-x

通用数据库 MCP Server，让 AI 编程助手（kilocode / cursor / claude code 等）通过 MCP 协议安全操作数据库。

> 前身项目：mcp-dbx（已重命名为 mcp-x）

## 特性

- **多数据库开箱即用**：MySQL、PostgreSQL、达梦、金仓、Redis、Elasticsearch、MongoDB、MinIO 对象存储
- **文档转换**：Excel↔Markdown、PDF↔Markdown、DOCX↔Markdown 等格式互转（需显式启用）
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

### 5. 接入 AI 编程助手

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

共 30 个工具：

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

安全层调用链：

```
tool handler
  -> safety.CheckWrite()                     # 读写模式检查
  -> safety.CheckSQL() / CheckRedisCommand() # 危险词拦截
  -> DELETE/UPDATE WHERE 检测
  -> driver.Query/Execute(ctxWithTimeout)     # 超时控制
  -> 对象存储 bucket/key 合法性校验
  -> MinIO 对象大小限制（1MB）
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
│   └── mcp/                    # MCP server + 工具 handler
├── examples/                   # 配置示例
├── docs/                       # 设计文档
└── Makefile
```

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
- 30 个 MCP 工具
- 66 条测试用例（含文档转换 22 条）

## 路线图

| 版本 | 内容 |
|------|------|
| v0.1.0 | MySQL + Redis + 安全层，stdio 传输 ✅ |
| v0.2.0 | PostgreSQL、达梦、金仓、Elasticsearch、MinIO、MongoDB 支持 ✅ |
| v0.3.0 | 文档转换模块（Excel↔Markdown、PDF↔Markdown、DOCX↔Markdown）✅ |
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
