package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yourname/mcp-x/internal/config"
	"github.com/yourname/mcp-x/internal/datasource"
)

type Server struct {
	server   *mcp.Server
	mgr      *datasource.Manager
	logger   *slog.Logger
	cfg      *config.Config
	comProgID string
}

func NewServer(name, version string, mgr *datasource.Manager, cfg *config.Config, logger *slog.Logger) (*Server, error) {
	s := &Server{
		server: mcp.NewServer(&mcp.Implementation{
			Name:    name,
			Version: version,
		}, nil),
		mgr:    mgr,
		logger: logger,
		cfg:    cfg,
	}

	if err := s.registerTools(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) registerTools() error {
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "db_list",
		Description: "List all configured data sources and their types (mysql/redis/etc). Use this first to know available databases.",
	}, s.handleDBList)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "db_ping",
		Description: "Ping a data source to check connectivity. Returns latency in ms.",
	}, s.handleDBPing)

	s.registerSQLTools()
	s.registerRedisTools()
	s.registerDocTools()
	s.registerObjectTools()
	s.registerDocconvTools()

	return nil
}

type emptyInput struct{}

type dbListOutput struct {
	Text string `json:"text" jsonschema:"the data source list as markdown table"`
}

func (s *Server) handleDBList(ctx context.Context, req *mcp.CallToolRequest, in emptyInput) (*mcp.CallToolResult, dbListOutput, error) {
	names := s.mgr.Names()
	if len(names) == 0 {
		return nil, dbListOutput{Text: "## Data Sources\n\nNo data sources configured."}, nil
	}

	out := "## Data Sources\n\n| Name | Status |\n|------|--------|\n"
	for _, name := range names {
		d, err := s.mgr.Get(name)
		status := "ok"
		if err != nil {
			status = "error: " + err.Error()
		} else if perr := d.Ping(ctx); perr != nil {
			status = "down: " + perr.Error()
		}
		out += fmt.Sprintf("| %s | %s |\n", name, status)
	}
	return nil, dbListOutput{Text: out}, nil
}

type dbPingInput struct {
	DataSource string `json:"datasource" jsonschema:"the name of the data source to ping"`
}

type dbPingOutput struct {
	Text string `json:"text" jsonschema:"the ping result as text"`
}

func (s *Server) handleDBPing(ctx context.Context, req *mcp.CallToolRequest, in dbPingInput) (*mcp.CallToolResult, dbPingOutput, error) {
	d, err := s.mgr.Get(in.DataSource)
	if err != nil {
		return nil, dbPingOutput{}, fmt.Errorf("data source %s: %w", in.DataSource, err)
	}

	start := time.Now()
	if err := d.Ping(ctx); err != nil {
		return nil, dbPingOutput{Text: fmt.Sprintf("## Ping Result\n\n**%s**: DOWN\n**Error**: %s\n**Latency**: N/A", in.DataSource, err)}, nil
	}
	latency := time.Since(start).Milliseconds()

	return nil, dbPingOutput{Text: fmt.Sprintf("## Ping Result\n\n**%s**: OK\n**Latency**: %dms", in.DataSource, latency)}, nil
}

func (s *Server) Run(ctx context.Context) error {
	s.logger.Info("mcp server starting", "transport", "stdio")
	return s.server.Run(ctx, &mcp.StdioTransport{})
}
