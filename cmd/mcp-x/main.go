package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/yourname/mcp-x/internal/config"
	"github.com/yourname/mcp-x/internal/datasource"
	mcpserver "github.com/yourname/mcp-x/internal/mcp"

	_ "github.com/yourname/mcp-x/internal/driver/dameng"
	_ "github.com/yourname/mcp-x/internal/driver/elasticsearch"
	_ "github.com/yourname/mcp-x/internal/driver/kingbase"
	_ "github.com/yourname/mcp-x/internal/driver/minio"
	_ "github.com/yourname/mcp-x/internal/driver/mongodb"
	_ "github.com/yourname/mcp-x/internal/driver/mysql"
	_ "github.com/yourname/mcp-x/internal/driver/postgres"
	_ "github.com/yourname/mcp-x/internal/driver/redis"
)

const helpText = `mcp-x: 通用数据库 MCP Server
让 AI 编程助手（kilocode / cursor / claude code 等）通过 MCP 协议安全操作数据库。

用法:
  mcp-x --config <配置文件路径>
  mcp-x -config <配置文件路径>
  mcp-x                 # 自动查找配置（见下方查找顺序）

参数:
  --config <path>       指定 YAML 配置文件路径
  -h, --help            显示本帮助信息

配置文件查找顺序（未传 --config 时）:
  1. ./.kilo/mcp-x.yaml          （项目级，推荐）
  2. ./.kilo/mcp-x.yml
  3. ~/.config/mcp_x/config.yaml（全局级，所有项目共享）
  4. ~/.config/mcp_x/config.yml

示例配置: examples/mcp-x.yaml.example

支持的数据源 Driver:
  关系型: mysql, postgres, dameng, kingbase
  NoSQL : redis, mongodb, elasticsearch
  对象  : minio

安全模式（safety.mode）:
  read-only   只允许 SELECT / 读操作（默认推荐）
  read-write  允许 INSERT/UPDATE/DELETE，危险关键词仍拦截
  危险关键词拦截: DROP / TRUNCATE / GRANT / REVOKE / ALTER / SHUTDOWN
  危险命令拦截  : FLUSHALL / FLUSHDB / CONFIG / KEYS 等
  DELETE/UPDATE 缺 WHERE 自动拦截；max_rows + 查询超时保护

工具一览（共 22+8 个，按数据源/功能启用）:
  通用: db_list, db_ping
  SQL : db_query, db_execute, db_tables, db_schema
  Redis: redis_get/set/del/keys/type/ttl
  ES  : doc_list_indices, doc_search, doc_get, doc_index, doc_delete
  MinIO: obj_list_buckets, obj_list, obj_get, obj_put, obj_delete
  文档转换(docconv, 需 config.docconv.enabled):
    excel_to_md, md_to_excel, pdf_to_md, docx_to_md, md_to_docx, md_to_pdf
    Windows+Office/WPS: word_to_pdf, pdf_to_word(实验性)

更多信息: https://github.com/yourname/mcp-x
`

func main() {
	var (
		configPath string
		showHelp   bool
	)
	flag.StringVar(&configPath, "config", "", "path to config file (default: ./.kilo/mcp-x.yaml)")
	flag.BoolVar(&showHelp, "help", false, "show help and exit")
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, helpText)
	}
	flag.Parse()

	if showHelp {
		fmt.Print(helpText)
		os.Exit(0)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	if configPath == "" {
		configPath = findConfig()
	}
	if configPath == "" {
		logger.Error("未找到配置文件，请先配置 mcp_x",
			"searched",
			"./.kilo/mcp-x.yaml, ./.kilo/mcp-x.yml, ~/.config/mcp_x/config.yaml, ~/.config/mcp_x/config.yml",
		)
		fmt.Fprintln(os.Stderr, "\n[提示] 请先创建配置文件，任选其一：")
		fmt.Fprintln(os.Stderr, "  1. 项目级: ./.kilo/mcp-x.yaml  (推荐，仅当前项目)")
		fmt.Fprintln(os.Stderr, "  2. 全局级: ~/.config/mcp_x/config.yaml (所有项目共享)")
		fmt.Fprintln(os.Stderr, "\n示例配置见: examples/mcp-x.yaml.example")
		fmt.Fprintln(os.Stderr, "\n完整用法: mcp-x --help")
		os.Exit(1)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Error("load config failed", "err", err, "path", configPath)
		os.Exit(1)
	}
	logger.Info("config loaded", "path", configPath, "datasources", len(cfg.DataSources))

	mgr, err := datasource.NewManager(cfg.DataSources, logger)
	if err != nil {
		logger.Error("init datasource manager failed", "err", err)
		os.Exit(1)
	}
	defer mgr.Close()

	srv, err := mcpserver.NewServer(cfg.Server.Name, cfg.Server.Version, mgr, cfg, logger)
	if err != nil {
		logger.Error("create mcp server failed", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("signal received, shutting down", "signal", sig)
		cancel()
	}()

	if err := srv.Run(ctx); err != nil {
		logger.Error("mcp server stopped", "err", err)
		os.Exit(1)
	}
}

func findConfig() string {
	candidates := []string{
		"./.kilo/mcp-x.yaml",
		"./.kilo/mcp-x.yml",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	home, err := os.UserHomeDir()
	if err == nil {
		for _, p := range []string{
			home + "/.config/mcp_x/config.yaml",
			home + "/.config/mcp_x/config.yml",
		} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}
