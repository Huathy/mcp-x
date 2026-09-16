# mcp-x

通用数据库 MCP Server，让 AI 编程助手（kilocode / cursor / claude code 等）通过 MCP 协议安全操作数据库。

## 特性

- **MySQL + Redis** 开箱即用；架构可扩展至国产数据库（OceanBase / 达梦 / 金仓）
- **安全层**：默认只读；写操作需显式配置；危险关键词拦截；DELETE/UPDATE 缺 WHERE 拦截；行数限制 + 查询超时
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

  - name: "cache-redis"
    driver: "redis"
    addr: "127.0.0.1:6379"
    password: ""
    db: 0
    pool_size: 10
    safety:
      mode: "read-write"
```

### 3. 手动验证

```bash
# 启动并发送 MCP JSON-RPC 测试
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | ./bin/mcp-x --config ./examples/mcp-x.yaml.example
```

正常会返回 `initialize` 响应，包含 server capabilities。

### 4. 接入 AI 编程助手

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

共 12 个工具：

### 通用工具

| 工具 | 说明 |
|------|------|
| `db_list` | 列出所有已配置数据源及状态 |
| `db_ping` | 健康检查，返回延迟 |

### SQL 工具（MySQL / OceanBase / 达梦 / 金仓）

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

安全层调用链：

```
tool handler
  -> safety.CheckWrite()                     # 读写模式检查
  -> safety.CheckSQL() / CheckRedisCommand() # 危险词拦截
  -> DELETE/UPDATE WHERE 检测
  -> driver.Query/Execute(ctxWithTimeout)     # 超时控制
```

## 测试环境

本地 Docker 起 MySQL + Redis：

```bash
docker run -d --name mysql-test -e MYSQL_ROOT_PASSWORD=test -p 3306:3306 mysql:8
docker run -d --name redis-test -p 6379:6379 redis:7
```

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
│   ├── driver/                 # Driver 接口 + MySQL/Redis 实现
│   ├── datasource/             # 数据源管理器
│   ├── safety/                 # 安全层
│   └── mcp/                    # MCP server + 工具 handler
├── examples/                   # 配置示例
├── docs/plans/                 # 设计文档
└── Makefile
```

## 扩展新数据库

1. 写 `internal/driver/xxx/xxx.go` 实现 `Driver` 或 `NoSQLDriver` 接口
2. 在 `init()` 中调用 `driver.Register("xxx", ...)`
3. 在 `cmd/mcp-x/main.go` import `_ "github.com/yourname/mcp-x/internal/driver/xxx"`
4. 配置文件加数据源，`driver: xxx`

**MySQL 协议兼容的数据库**（OceanBase / TiDB）可直接复用 MySQL driver，仅改 driver 标识名即可。

## 开发

```bash
make build     # 编译
make test      # 测试
make run       # 编译+运行
make clean     # 清理
```

## 路线图

| 版本 | 内容 |
|------|------|
| v0.1.0 | MySQL + Redis + 安全层，stdio 传输 ✅ |
| v0.2.0 | OceanBase + 金仓支持，审计日志 |
| v0.3.0 | 达梦 DM8 支持 |
| v0.4.0 | HTTP/SSE 传输 |
| v0.5.0 | 连接池监控、慢查询日志 |

## 技术栈

- Go 1.25+
- [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk) v1.7.0（官方 SDK）
- [go-sql-driver/mysql](https://github.com/go-sql-driver/mysql) v1.10.1
- [go-redis/v9](https://github.com/redis/go-redis) v9.22.0
- `log/slog` 标准库结构化日志
