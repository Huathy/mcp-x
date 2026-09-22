# mcp-x 测试汇总报告

> 日期：2026-09-16
> 测试人：Kilo 自动化测试
> 数据基准：本机容器快照（9 数据源全连接）

## 总览

| 子计划 | 类型 | 用例数 | 结果 |
|--------|------|--------|------|
| 00 环境准备 | 环境 | - | ✅ 通过 |
| 01 调用约定 | 约定 | - | ✅ 通过 |
| 02 通用工具 | MCP 通用 | 5 | ✅ 全通过 |
| 03 SQL-MySQL | SQL | 12 | ✅ 全通过 |
| 04 SQL-安全 | SQL 安全 | 6 | ✅ 全通过 |
| 05 SQL-UTF8 | SQL | 6 | ✅ 全通过 |
| 06 SQL-PostgreSQL | SQL | 6 | ✅ 全通过 |
| 07 Redis | NoSQL | 10 | ✅ 全通过 |
| 08 Redis-安全 | NoSQL 安全 | 2 | ✅ 全通过 |
| 09 MongoDB | NoSQL | 5 | ✅ 全通过 |
| 10 Elasticsearch | DocStore | 6 | ✅ 全通过 |
| 11 MinIO | Object | 6 | ✅ 全通过 |
| 12 MinIO-安全 | Object 安全 | 2 | ✅ 全通过 |
| **合计** | | **66** | **66/66 通过** |

## 发现并修复的 Bug

### Bug 1: MySQL DescribeTable NULL 默认值 scan 失败
- 文件：`internal/driver/mysql/mysql.go:127`
- 问题：`DescribeTable` 用 `string` 接收 `Default` 列，MySQL NULL 默认值报 `converting NULL to string is unsupported`
- 修复：改用 `sql.NullString`，重新编译后 T-102 通过

## 测试配置修正

1. **PG DSN**：URL 格式 `@` 编码无效（SASL 认证失败），需 `ALTER USER` 重置密码后用 key=value 格式 `host=... password='admin@2025'`
2. **MongoDB DSN**：需带认证 `mongodb://admin:admin%402025@127.0.0.1:27017/mcp_x_test?authSource=admin`，且 Set 值必须为合法 JSON
3. **MinIO bucket**：需 `mc alias set` + `mc mb` 预创建 `mcp-x-test` bucket
4. **ES Search query**：driver 已包 `"query":` 外壳，用户传 `{"match_all":{}}` 而非 `{"query":{...}}`

## 已知限制

1. Kingbase/Dameng：无 Docker 镜像，跳过（符合计划）
2. go-sdk v1.7.0 stdio = 换行分隔 JSON（非 Content-Length）
3. PowerShell 管道 stdin 关闭导致 EOF：需用 Process API 保持 stdin 开启
4. PostgreSQL 参数化占位符用 `$1` 非 `?`（pgx v5 特性，非 bug）

## 工具注册验证

22 个 MCP 工具全部注册成功，与 01 子计划第 2 节表一致。

## 结论

66/66 用例通过。1 个代码 bug 已修复。测试配置文档需更新 PG/MongoDB DSN 示例。
