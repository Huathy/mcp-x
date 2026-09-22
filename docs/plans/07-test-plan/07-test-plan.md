# mcp-x 功能测试计划

> 编写日期：2026-09-16
> 测试目标：覆盖 mcp-x 全部 22 个 MCP 工具、8 个 driver、安全层、配置加载
> 数据基准：本机已运行容器快照（见下表）

## 1. 测试范围

### 1.1 已支持 Driver（8 个）

| Driver   | 类型       | 本机状态                | 测试策略                              |
|----------|-----------|------------------------|---------------------------------------|
| mysql    | SQL       | 运行中（3306/5.7, 3308/8.4） | 复用本地容器，root/admin@2025         |
| redis    | NoSQL     | 运行中（6379）           | 复用本地容器，admin@2025              |
| postgres | SQL       | 不存在                  | Docker 临时创建，测完删除             |
| kingbase | SQL       | 不存在                  | 跳过（无官方 Docker 镜像，企业商用）  |
| dameng   | SQL       | 不存在                  | 跳过（无官方 Docker 镜像，企业商用）  |
| mongodb  | NoSQL     | 停止中                  | 启动本地容器（无密码）                |
| elasticsearch | DocStore | 不存在           | Docker 临时创建，测完删除             |
| minio    | Object    | 停止中                  | 启动本地容器，admin/admin@2025        |

> **Kingbase / Dameng 跳过说明**：均为国产商业数据库，无官方可用 Docker 镜像，本地无法快速拉起。代码路径与 postgres 共用 `pglike` 实现（kingbase）或独立 `dameng.go`，待有真实环境再补端到端测试。

### 1.2 MCP 工具（22 个）

| 分类     | 工具                                                              |
|----------|-------------------------------------------------------------------|
| 通用     | `db_list`, `db_ping`                                              |
| SQL      | `db_query`, `db_execute`, `db_tables`, `db_schema`                |
| Redis    | `redis_get`, `redis_set`, `redis_del`, `redis_keys`, `redis_type`, `redis_ttl` |
| DocStore | `doc_list_indices`, `doc_search`, `doc_get`, `doc_index`, `doc_delete` |
| Object   | `obj_list_buckets`, `obj_list`, `obj_get`, `obj_put`, `obj_delete` |

> mongodb 注册为 NoSQLDriver，复用 redis_* 工具集（Get/Set/Del/Keys/KeyType/TTL）。

## 2. 测试环境准备

### 2.1 本地已运行服务（无需创建）

| 服务    | 端口 | 账号              | 用途                      |
|---------|------|-------------------|---------------------------|
| MySQL 5.7 | 3306 | root/admin@2025   | 主测试库（已含业务库）    |
| MySQL 8.4 | 3308 | root/admin@2025   | UTF8 多语言、8.x 行为测试  |
| Redis 7  | 6379 | admin@2025        | 缓存测试                  |

### 2.2 需启动的本地停止容器

```bash
# MinIO（已存在，仅启动）
docker start minio
# MongoDB（已存在，仅启动）
docker start mongodb
```

- MinIO 账号：admin / admin@2025（环境变量 `MINIO_ROOT_USER/PASSWORD`）
- MongoDB：无密码（`mongod` 无 auth）

### 2.3 需 Docker 临时创建的服务（测完删除）

```bash
# PostgreSQL 16
docker run -d --name mcp-x-test-pg \
  -e POSTGRES_PASSWORD=admin@2025 \
  -p 55432:5432 \
  postgres:16-alpine

# Elasticsearch 8（单节点，关闭安全）
docker run -d --name mcp-x-test-es \
  -e discovery.type=single-node \
  -e xpack.security.enabled=false \
  -e ES_JAVA_OPTS="-Xms512m -Xmx512m" \
  -p 9200:9200 \
  docker.elastic.co/elasticsearch/elasticsearch:8.17.0
```

### 2.4 清理（仅删除本测试创建的容器）

```bash
docker stop mcp-x-test-pg mcp-x-test-es
docker rm mcp-x-test-pg mcp-x-test-es
# 注意：不删除 minio / mongodb / mysql-* / redis（用户已有）
```

## 3. 测试配置文件

放在 `D:\works\temp\kilo\mcp-x-test.yaml`（避免污染项目目录）。

```yaml
server:
  name: "mcp-x-test"
  version: "0.1.0"

safety:
  mode: "read-write"
  max_rows: 1000
  query_timeout: 30s
  blocked_keywords: ["DROP", "TRUNCATE", "GRANT", "REVOKE", "ALTER", "SHUTDOWN"]
  allow_blocked_keywords: ["DROP"]   # 测试 CREATE/DROP 临时表
  blocked_commands: ["FLUSHALL", "FLUSHDB", "CONFIG", "SHUTDOWN", "KEYS", "BGREWRITEAOF", "BGSAVE"]
  allow_blocked_commands: []

datasources:
  # MySQL 5.7（主测试，读写）
  - name: "mysql57"
    driver: "mysql"
    dsn: "root:admin@2025@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=true&allowNativePasswords=true"
    safety:
      mode: "read-write"

  # MySQL 8.4（UTF8 多语言测试，读写）
  - name: "mysql8"
    driver: "mysql"
    dsn: "root:admin@2025@tcp(127.0.0.1:3308)/mysql?charset=utf8mb4&parseTime=true"
    safety:
      mode: "read-write"

  # MySQL 只读测试
  - name: "mysql-ro"
    driver: "mysql"
    dsn: "root:admin@2025@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=true"
    safety:
      mode: "read-only"

  # Redis（读写）
  - name: "redis"
    driver: "redis"
    addr: "127.0.0.1:6379"
    password: "admin@2025"
    db: 0
    safety:
      mode: "read-write"

  # Redis 只读测试
  - name: "redis-ro"
    driver: "redis"
    addr: "127.0.0.1:6379"
    password: "admin@2025"
    db: 1
    safety:
      mode: "read-only"

  # PostgreSQL（临时容器）
  - name: "pg"
    driver: "postgres"
    dsn: "postgres://postgres:admin@2025@127.0.0.1:55432/postgres?sslmode=disable"
    safety:
      mode: "read-write"

  # MongoDB（本地停止容器启动后）
  - name: "mongo"
    driver: "mongodb"
    dsn: "mongodb://127.0.0.1:27017"
    database: "mcp_x_test"
    bucket: "kv"
    safety:
      mode: "read-write"

  # Elasticsearch（临时容器）
  - name: "es"
    driver: "elasticsearch"
    addrs:
      - "http://127.0.0.1:9200"
    index_name: "mcp-x-test"
    safety:
      mode: "read-write"

  # MinIO（本地停止容器启动后）
  - name: "minio"
    driver: "minio"
    endpoint: "127.0.0.1:9000"
    access_key: "admin"
    secret_key: "admin@2025"
    use_ssl: false
    region: "us-east-1"
    bucket: "mcp-x-test"
    safety:
      mode: "read-write"
```

## 4. MCP JSON-RPC 调用约定

mcp-x 走 stdio，通过管道发送 JSON-RPC 2.0 消息测试。一次会话流程：

1. `initialize` → 获取 server capabilities（应含 tools 列表）
2. `tools/list` → 校验 22 个工具全部注册
3. `tools/call` → 逐个工具调用

调用模板（PowerShell here-string 管道）：

```powershell
$req = '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"<tool>","arguments":{<args>}}}'
$req | & "D:\works\mystudy\my_open\mcp_x\bin\mcp-x.exe" --config "D:\works\temp\kilo\mcp-x-test.yaml"
```

> 实际测试需先 `initialize`，再 `tools/list`，再 `tools/call`。
> 多条请求需在同一 stdin 流内顺序发送（以 `Content-Length` header 或换行分隔，取决于 SDK）。

## 5. 测试用例

### 5.1 通用工具

| ID    | 工具       | 数据源     | 预期                                              |
|-------|-----------|-----------|---------------------------------------------------|
| T-001 | `db_list` | -         | 列出 8 个数据源，全部 status=ok                  |
| T-002 | `db_ping` | mysql57   | OK，延迟 < 100ms                                  |
| T-003 | `db_ping` | redis     | OK，延迟 < 50ms                                   |
| T-004 | `db_ping` | pg        | OK，延迟 < 100ms                                  |
| T-005 | `db_ping` | nonexistent | 返回 DOWN，error 含 "not found"                  |

### 5.2 SQL 工具 - MySQL

| ID    | 工具        | 操作                                              | 预期                                              |
|-------|------------|---------------------------------------------------|---------------------------------------------------|
| T-101 | `db_tables` | mysql57                                          | 返回表列表，含 mysql/user 等系统表               |
| T-102 | `db_schema` | mysql57, table=user                              | 列出列/类型/索引                                  |
| T-103 | `db_query`  | `SELECT 1 AS v`                                  | 返回 1 行，v=1                                    |
| T-104 | `db_query`  | `SELECT * FROM user LIMIT 1`                     | 返回 1 行用户数据                                 |
| T-105 | `db_query`  | 参数化 `SELECT * FROM user WHERE User=?` args=["root"] | 返回 root 用户                                |
| T-106 | `db_query`  | `SELECT 1`（mysql-ro 只读）                      | OK，只读允许 SELECT                               |
| T-107 | `db_execute` | `CREATE TABLE IF NOT EXISTS mcp_test(id INT PRIMARY KEY, val VARCHAR(100))` | Affected rows=0 |
| T-108 | `db_execute` | `INSERT INTO mcp_test(id,val) VALUES(?,?)` args=[1,"hello"] | Affected rows=1                       |
| T-109 | `db_query`  | `SELECT * FROM mcp_test`                         | 返回 1 行                                         |
| T-110 | `db_execute` | `UPDATE mcp_test SET val=? WHERE id=?` args=["world",1] | Affected rows=1                            |
| T-111 | `db_execute` | `DELETE FROM mcp_test WHERE id=?` args=[1]       | Affected rows=1                                   |
| T-112 | `db_execute` | `DROP TABLE mcp_test`                             | OK（allow_blocked_keywords 含 DROP）             |

### 5.3 SQL 工具 - 安全拦截

| ID    | 工具        | 操作                                              | 预期                                              |
|-------|------------|---------------------------------------------------|---------------------------------------------------|
| T-201 | `db_execute` | mysql-ro 执行 `INSERT INTO user VALUES(...)`   | 拒绝："read-only"                                 |
| T-202 | `db_execute` | `DROP TABLE x`（mysql57，DROP 未放行）            | 拒绝：blocked keyword DROP                        |
| T-203 | `db_execute` | `DELETE FROM mcp_test`（无 WHERE）              | 拒绝：DELETE without WHERE                       |
| T-204 | `db_execute` | `UPDATE mcp_test SET val='x'`（无 WHERE）        | 拒绝：UPDATE without WHERE                       |
| T-205 | `db_query`  | `SELECT * FROM mysql.user`（mysql-ro）           | OK，只读允许                                      |
| T-206 | `db_query`  | `DELETE FROM x`                                  | 拒绝：db_query only allows SELECT                |

### 5.4 SQL 工具 - UTF8 多语言

| ID    | 工具        | 操作                                              | 预期                                              |
|-------|------------|---------------------------------------------------|---------------------------------------------------|
| T-301 | `db_execute` | mysql8 `CREATE TABLE mcp_utf8(id INT, err TEXT) DEFAULT CHARSET=utf8mb4` | OK          |
| T-302 | `db_execute` | `INSERT INTO mcp_utf8(id,err) VALUES(830,NULL),(835,'温度超过阈值80度')` | OK        |
| T-303 | `db_execute` | `INSERT INTO mcp_utf8 VALUES(840,'温度が上昇値80度を超えました')` | OK          |
| T-304 | `db_execute` | `INSERT INTO mcp_utf8 VALUES(843,'온도가 한계값 80도를 초과했습니다')` | OK        |
| T-305 | `db_query`  | `SELECT id,err FROM mcp_utf8 ORDER BY id`         | 多语言字符完整无损                                |
| T-306 | `db_execute` | `DROP TABLE mcp_utf8`                             | 清理                                              |

### 5.5 SQL 工具 - PostgreSQL

| ID    | 工具        | 操作                                              | 预期                                              |
|-------|------------|---------------------------------------------------|---------------------------------------------------|
| T-401 | `db_tables` | pg                                               | 初始含 public schema，无表                        |
| T-402 | `db_execute` | `CREATE TABLE mcp_test(id SERIAL PRIMARY KEY, val TEXT)` | OK                              |
| T-403 | `db_execute` | `INSERT INTO mcp_test(val) VALUES(?)` args=["pg-hello"] | Affected rows=1                             |
| T-404 | `db_query`  | `SELECT * FROM mcp_test`                          | 1 行                                              |
| T-405 | `db_schema` | pg, table=mcp_test                                | 列出 id/val                                       |
| T-406 | `db_execute` | `DROP TABLE mcp_test`                             | OK（allow DROP）                                  |

### 5.6 Redis 工具

| ID    | 工具         | 操作                                              | 预期                                              |
|-------|-------------|---------------------------------------------------|---------------------------------------------------|
| T-501 | `redis_set`  | key=mcp:test, value=hello, ttl=0                 | OK                                                |
| T-502 | `redis_get`  | key=mcp:test                                     | value=hello, type=string                          |
| T-503 | `redis_type` | key=mcp:test                                     | string                                            |
| T-504 | `redis_ttl`  | key=mcp:test                                     | -1（无过期）                                      |
| T-505 | `redis_set`  | key=mcp:ttl, value=x, ttl=10                     | OK                                                |
| T-506 | `redis_ttl`  | key=mcp:ttl                                      | 10（或接近 10）                                   |
| T-507 | `redis_keys` | pattern=mcp:*                                    | 至少 2 个                                         |
| T-508 | `redis_del`  | key=mcp:test                                     | Deleted=1                                         |
| T-509 | `redis_del`  | key=mcp:ttl                                      | Deleted=1                                         |
| T-510 | `redis_keys` | pattern=mcp:*                                    | No keys matched                                   |

### 5.7 Redis 工具 - 安全拦截

| ID    | 工具        | 操作                                              | 预期                                              |
|-------|------------|---------------------------------------------------|---------------------------------------------------|
| T-601 | `redis_set`  | redis-ro 写入                                     | 拒绝：read-only                                   |
| T-602 | `redis_del`  | redis-ro 删除                                     | 拒绝：read-only                                   |

### 5.8 Redis 工具 - MongoDB

> mongodb 复用 NoSQLDriver 接口，用 redis_* 工具调用，datasource 指向 mongo。

| ID    | 工具        | 操作                                              | 预期                                              |
|-------|------------|---------------------------------------------------|---------------------------------------------------|
| T-701 | `redis_set`  | mongo, key=mcp:test, value=hello                 | OK                                                |
| T-702 | `redis_get`  | mongo, key=mcp:test                              | value=hello                                        |
| T-703 | `redis_keys` | mongo, pattern=mcp:*                             | 至少 1 个                                         |
| T-704 | `redis_type` | mongo, key=mcp:test                              | string（或 mongo 自定义类型）                    |
| T-705 | `redis_del`  | mongo, key=mcp:test                              | Deleted=1                                         |

### 5.9 Elasticsearch 工具

| ID    | 工具             | 操作                                          | 预期                                              |
|-------|-----------------|-----------------------------------------------|---------------------------------------------------|
| T-801 | `doc_list_indices` | es                                         | 初始可能为空或系统索引                            |
| T-802 | `doc_index`        | es, index=mcp-x-test, id=1, body=`{"msg":"hello 中文"}` | OK                                  |
| T-803 | `doc_get`          | es, index=mcp-x-test, id=1                 | 返回 msg=hello 中文                               |
| T-804 | `doc_search`       | es, index=mcp-x-test, query=`{"query":{"match_all":{}}}` | 1 doc                              |
| T-805 | `doc_delete`       | es, index=mcp-x-test, id=1                 | OK                                                |
| T-806 | `doc_get`          | es, index=mcp-x-test, id=1                 | not found                                         |

### 5.10 MinIO 工具

| ID    | 工具              | 操作                                          | 预期                                              |
|-------|------------------|-----------------------------------------------|---------------------------------------------------|
| T-901 | `obj_list_buckets` | minio                                      | 含 mcp-x-test（或先创建）                        |
| T-902 | `obj_put`          | minio, bucket=mcp-x-test, key=test/hello.txt, body="hello 中文" | OK            |
| T-903 | `obj_list`         | minio, bucket=mcp-x-test, prefix=test/     | 1 对象                                            |
| T-904 | `obj_get`          | minio, bucket=mcp-x-test, key=test/hello.txt | content="hello 中文"                             |
| T-905 | `obj_delete`       | minio, bucket=mcp-x-test, key=test/hello.txt | OK                                                |
| T-906 | `obj_list`         | minio, bucket=mcp-x-test, prefix=test/     | No objects                                        |

### 5.11 MinIO 安全拦截

| ID     | 工具         | 操作                                              | 预期                                              |
|--------|------------|---------------------------------------------------|---------------------------------------------------|
| T-1001 | `obj_put`    | bucket=other-bucket（非配置的 mcp-x-test）       | 拒绝：bucket not allowed                          |
| T-1002 | `obj_put`    | key=../escape                                      | 拒绝：must not contain '..'                       |

## 6. 测试执行步骤

### Step 1：启动服务

```bash
# 启动本地停止容器
docker start minio mongodb

# 创建临时容器
docker run -d --name mcp-x-test-pg -e POSTGRES_PASSWORD=admin@2025 -p 55432:5432 postgres:16-alpine
docker run -d --name mcp-x-test-es -e discovery.type=single-node -e xpack.security.enabled=false -e ES_JAVA_OPTS="-Xms512m -Xmx512m" -p 9200:9200 docker.elastic.co/elasticsearch/elasticsearch:8.17.0
```

### Step 2：等待就绪

```bash
# PG
docker exec mcp-x-test-pg pg_isready -U postgres
# ES
curl http://127.0.0.1:9200
# MinIO 端口
docker exec minio mc admin info local
```

### Step 3：准备 MinIO bucket

```bash
docker exec minio mc mb local/mcp-x-test 2>$null
# 或通过 obj_list_buckets 看是否存在，不存在则需先创建
```

> MinIO 启动后默认无 bucket，需通过 mc CLI 或 minio 控制台预先创建 `mcp-x-test` bucket（mcp-x 无创建 bucket 工具）。

### Step 4：编译 mcp-x

```bash
cd D:\works\mystudy\my_open\mcp_x
go build -o bin/mcp-x.exe ./cmd/mcp-x
```

### Step 5：写测试配置

将第 3 节配置写入 `D:\works\temp\kilo\mcp-x-test.yaml`。

### Step 6：执行测试用例

按第 5 节顺序执行 JSON-RPC 调用，记录每个用例的实际返回。

> 建议用脚本批量发送：`initialize` → `tools/list` → 逐个 `tools/call`。

### Step 7：清理

```bash
# 删除本测试创建的临时容器
docker stop mcp-x-test-pg mcp-x-test-es
docker rm mcp-x-test-pg mcp-x-test-es

# 停止本测试启动的本地容器（恢复原状）
docker stop minio mongodb

# 删除测试数据（如 MySQL 测试残留表）
docker exec mysql-3306 mysql -uroot -padmin@2025 -e "DROP TABLE IF EXISTS mysql.mcp_test;"
docker exec mysql8_3308 mysql -uroot -padmin@2025 -e "DROP TABLE IF EXISTS mysql.mcp_utf8;"
docker exec redis redis-cli -a admin@2025 DEL mcp:test mcp:ttl
```

## 7. 预期结果汇总

| 类别         | 用例数 | 全部通过标准                                     |
|--------------|--------|--------------------------------------------------|
| 通用工具     | 5      | db_list 列出 8 源，ping 正常                      |
| SQL-MySQL    | 12     | CRUD 闭环，参数化正常                            |
| SQL-安全     | 6      | 所有危险操作正确拦截                              |
| SQL-UTF8     | 6      | 中/日/韩文无损读写                                |
| SQL-PG       | 6      | CRUD 闭环                                        |
| Redis        | 10     | 读写/扫描/TTL 闭环                               |
| Redis-安全   | 2      | 只读拒绝写                                        |
| MongoDB      | 5      | 复用 redis_* 工具读写闭环                         |
| Elasticsearch | 6     | 索引/搜索/获取/删除闭环                           |
| MinIO        | 6      | bucket/object 读写闭环                            |
| MinIO-安全   | 2      | bucket/key 校验拦截                               |
| **合计**     | **66** |                                                  |

## 8. 已知限制与跳过项

1. **Kingbase / Dameng**：无官方 Docker 镜像，跳过端到端测试。kingbase 复用 pglike 实现，待真实环境验证。
2. **Redis 集群 / 哨兵模式**：本地无集群，跳过 `mode: cluster/sentinel` 测试。
3. **`db_ping` 不存在数据源**：T-005 预期返回 DOWN（非 panic），验证错误处理路径。
4. **MCP SDK 传输细节**：实际 JSON-RPC 分隔方式以 go-sdk v1.7.0 行为准，测试时需确认是换行分隔还是 Content-Length。
5. **PostgreSQL 5.7 密码警告**：MySQL 5.7 命令行密码警告是 stderr 噪音，不影响功能。
6. **MongoDB TTL**：mongo driver 的 TTL 实现可能返回固定值（需看代码确认，见 `internal/driver/mongodb/mongodb.go:133`）。

## 9. 测试输出存放

- 测试日志：`D:\works\temp\kilo\mcp-x-test-log.md`
- 异常返回：记录原始 JSON-RPC response
- 通过/失败统计：按 T-XXX ID 汇总
