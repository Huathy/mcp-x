# mcp-x

A universal database MCP Server that lets AI coding assistants (kilocode / cursor / claude code, etc.) operate databases safely via the MCP protocol.

> Predecessor: mcp-dbx (renamed to mcp-x)

## Features

- **Multiple databases out of the box**: MySQL, PostgreSQL, Dameng, Kingbase, Redis, Elasticsearch, MongoDB, MinIO object storage
- **Document conversion**: Excel↔Markdown, PDF↔Markdown, DOCX↔Markdown and other format conversions (explicitly enabled via config)
- **Safety layer**: read-only by default; explicit config required for writes; dangerous keyword interception; DELETE/UPDATE without WHERE blocked; row limit + query timeout; object storage bucket/key validation
- **stdio transport**, direct local connection for kilocode / cursor
- **YAML config**, multi-datasource named management, routing by name
- **Single binary**, zero-dependency deployment

## Quick Start

### 1. Build

```bash
# Requires Go 1.25+
make build
# or directly
go build -o bin/mcp-x ./cmd/mcp-x
```

Cross-platform build:

```bash
# Windows
$env:GOOS="windows"; $env:GOARCH="amd64"; go build -o bin/mcp-x.exe ./cmd/mcp-x
# Linux
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o bin/mcp-x ./cmd/mcp-x
# macOS
$env:GOOS="darwin"; $env:GOARCH="arm64"; go build -o bin/mcp-x ./cmd/mcp-x
```

### 2. Write Config

Copy `examples/mcp-x.yaml.example` and modify:

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
      mode: "read-only"     # force read-only for this datasource

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

### 3. Manual Verification

```bash
# Show help
./bin/mcp-x --help

# Start and send MCP JSON-RPC test
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | ./bin/mcp-x --config ./examples/mcp-x.yaml.example
```

A normal response returns `initialize` result with server capabilities.

### 4. Document Conversion (Optional)

Document conversion must be explicitly enabled in config:

```yaml
docconv:
  enabled: true
  workdir: "./data/docconv"     # temp directory
  max_file_size_mb: 50          # max file size
  com:
    prog_id: "auto"             # auto = probe Word.Application / kwps / wps
    timeout: 60s                 # COM call timeout
```

Windows requires WPS or Microsoft Office; Linux requires LibreOffice configuration.

### 5. Connect to AI Coding Assistant

#### kilocode

`.kilocode/mcp.json`:

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

`.cursor/mcp.json`:

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

Restart the IDE, then AI can directly call database tools.

## Tools

30 tools total:

### Common Tools

| Tool | Description |
|------|-------------|
| `db_list` | List all configured datasources and status |
| `db_ping` | Health check, returns latency |

### SQL Tools (MySQL / PostgreSQL / Dameng / Kingbase)

| Tool | Description |
|------|-------------|
| `db_query` | Execute SELECT, returns Markdown table, max 1000 rows |
| `db_execute` | Execute INSERT/UPDATE/DELETE, returns affected rows |
| `db_tables` | List all tables |
| `db_schema` | View table structure (columns/types/indexes) |

### Redis Tools

| Tool | Description |
|------|-------------|
| `redis_get` | Read key value |
| `redis_set` | Write key value (supports TTL) |
| `redis_del` | Delete key |
| `redis_keys` | SCAN keys (non-blocking, max 100) |
| `redis_type` | Check key type |
| `redis_ttl` | Check TTL |

### Document Store Tools (Elasticsearch / MongoDB)

| Tool | Description |
|------|-------------|
| `doc_list_indices` | List all indices/collections |
| `doc_search` | Full-text search |
| `doc_get` | Get document by ID |
| `doc_index` | Index/insert document |
| `doc_delete` | Delete document |

### Object Store Tools (MinIO / S3 compatible)

| Tool | Description |
|------|-------------|
| `obj_list_buckets` | List all buckets |
| `obj_list` | List objects in bucket |
| `obj_get` | Download object |
| `obj_put` | Upload object |
| `obj_delete` | Delete object |

### Document Conversion Tools (docconv)

| Tool | Description |
|------|-------------|
| `excel_to_md` | Excel → Markdown |
| `md_to_excel` | Markdown → Excel |
| `pdf_to_md` | PDF → Markdown |
| `docx_to_md` | DOCX → Markdown |
| `md_to_docx` | Markdown → DOCX |
| `md_to_pdf` | Markdown → PDF |
| `word_to_pdf` | Word → PDF |
| `pdf_to_word` | PDF → Word |

## Configuration

### Config File Lookup Order

1. `--config <path>` via command line
2. `./mcp-x.yaml` current directory
3. `~/.mcp-x/config.yaml` user directory

### Safety Priority

```
datasource safety > global safety
```

Datasource `safety` field overrides global. Unconfigured inherits global.

### Redis Cluster Config

```yaml
datasources:
  - name: "cluster-redis"
    driver: "redis"
    mode: "cluster"       # standalone(default) | cluster | sentinel
    addrs:                # cluster uses addrs (plural)
      - "10.0.0.1:6379"
      - "10.0.0.2:6379"
      - "10.0.0.3:6379"
    password: ""
    pool_size: 20
```

## Safety Policy

| Risk | Protection |
|------|------------|
| Accidental delete | DELETE/UPDATE without WHERE auto-blocked |
| Drop table / flush db | DROP/TRUNCATE blocked by default, configurable to allow |
| Full table scan | max_rows limit + query timeout |
| Redis KEYS blocking | Forced SCAN replacement, KEYS command blocked |
| Privilege escalation | GRANT/REVOKE blocked by default |
| MinIO large object read | 1MB limit to prevent memory overflow |
| MongoDB URI injection | Connection URI credentials auto URL-escaped |
| Elasticsearch injection | Query and document body JSON format validation |
| Object storage path traversal | bucket/key validation + bucket whitelist |
| MongoDB wildcard abuse | Keys search supports glob wildcards to prevent full collection scan |

Safety layer call chain:

```
tool handler
  -> safety.CheckWrite()                     # read-write mode check
  -> safety.CheckSQL() / CheckRedisCommand() # dangerous keyword interception
  -> DELETE/UPDATE WHERE detection
  -> driver.Query/Execute(ctxWithTimeout)     # timeout control
  -> object storage bucket/key validation
  -> MinIO object size limit (1MB)
```

## Test Environment

Start MySQL + PostgreSQL + Redis + Elasticsearch + MinIO + MongoDB via local Docker:

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

### Domestic Databases

- **Dameng DM8**: Download Dameng official Docker image, port 5236
- **KingbaseES**: Download Kingbase official Docker image, port 54321

## UTF8 Multi-language Read/Write Verification

MySQL DSN uses `charset=utf8mb4`, fully supporting multi-language UTF8 read/write. Test table `zy_alarm`, column `err longtext`.

### Test Data (id 830-847)

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
| 845 | timeout | N | 게이트웨이 시간 초과：30秒 지연 후 종료 |
| 846 | config | N | パラメータエラー：コンフィグバリデーションに不合格 |
| 847 | config | N | 配置错误：参数校验未通过 |

### Supported Languages

- English: `connection timed out after 30s`
- Chinese: `温度超过阈值80度`
- Japanese: `温度が上昇値80度を超えました`
- Korean: `온도가 한계값 80도를 초과했습니다`
- Mixed punctuation: `장치 연결 끊김：하트비트 응답 없음` (fullwidth colon)

### Key Points

- DSN must use `charset=utf8mb4` (not `utf8`; the latter only supports BMP 3-byte, missing some emoji and rare chars)
- MySQL table/column `COLLATE` recommended: `utf8mb4_general_ci` or `utf8mb4_unicode_ci`
- MCP JSON-RPC over stdio transmits UTF8 natively, no extra encoding needed

## Project Structure

```
mcp-x/
├── cmd/mcp-x/main.go           # entry point
├── internal/
│   ├── config/                 # YAML config parsing
│   ├── driver/                 # Driver interface + multi-database implementations
│   │   ├── mysql/              # MySQL
│   │   ├── pglike/             # PostgreSQL compatibility layer
│   │   ├── postgres/           # PostgreSQL
│   │   ├── kingbase/           # KingbaseES
│   │   ├── dameng/             # Dameng DM8
│   │   ├── redis/              # Redis
│   │   ├── elasticsearch/      # Elasticsearch
│   │   ├── mongodb/            # MongoDB
│   │   └── minio/              # MinIO object storage
│   ├── datasource/             # datasource manager
│   ├── safety/                 # safety layer
│   ├── docconv/                # document conversion module
│   └── mcp/                    # MCP server + tool handlers
├── examples/                   # config examples
├── docs/                       # design docs
└── Makefile
```

## Extending New Databases

1. Write `internal/driver/xxx/xxx.go` implementing `Driver`, `DocStoreDriver` or `ObjectStoreDriver` interface
2. In `init()`, call `driver.Register("xxx", ...)`
3. Import in `cmd/mcp-x/main.go`: `_ "github.com/yourname/mcp-x/internal/driver/xxx"`
4. Add datasource in config, `driver: xxx`

**MySQL-protocol-compatible databases** (OceanBase / TiDB) can reuse the MySQL driver directly; just change the driver label.

## Extending Document Formats

Document conversion module can be explicitly enabled via `docconv.enabled`, supporting:
- Excel↔Markdown
- PDF↔Markdown
- DOCX↔Markdown
- Markdown→Excel/PDF/DOCX
- Word→PDF / PDF→Word

Additional dependencies: go-ole (COM automation), excelize, goldmark, gopdf, ledongthuc/pdf

## Development

```bash
make build     # build
make test      # test
make run       # build + run
make clean     # clean
```

### Test Coverage

- 8 Drivers (MySQL, PostgreSQL, Kingbase, Dameng, Redis, Elasticsearch, MongoDB, MinIO)
- 30 MCP tools
- 66 test cases (including 22 document conversion tests)

## Roadmap

| Version | Content |
|---------|---------|
| v0.1.0 | MySQL + Redis + safety layer, stdio transport ✅ |
| v0.2.0 | PostgreSQL, Dameng, Kingbase, Elasticsearch, MinIO, MongoDB support ✅ |
| v0.3.0 | Document conversion module (Excel↔Markdown, PDF↔Markdown, DOCX↔Markdown) ✅ |
| v0.4.0 | HTTP/SSE transport, connection pool monitoring, slow query log |
| v0.5.0 | Audit log, more domestic database support |

## Tech Stack

- Go 1.25+
- [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk) v1.7.0 (official SDK)
- [go-sql-driver/mysql](https://github.com/go-sql-driver/mysql) v1.10.1
- [go-redis/v9](https://github.com/redis/go-redis) v9.22.0
- [elastic/go-elasticsearch](https://github.com/elastic/go-elasticsearch) v8.15.0
- [minio/minio-go](https://github.com/minio/minio-go) v7.0.70
- [mongodb/mongo-go-driver](https://github.com/mongodb/mongo-go-driver) v1.17.1
- [lib/pq](https://github.com/lib/pq) v1.10.9 (PostgreSQL)
- [excelize](https://github.com/xuri/excelize/v2) v2.8.0
- [goldmark](https://github.com/yuin/goldmark) v1.7.4
- [gopdf](https://github.com/phpdave11/gofpdf) v1.4.3
- [ledongthuc/pdf](https://github.com/ledongthuc/pdf) v0.8.0
- `log/slog` standard library structured logging
