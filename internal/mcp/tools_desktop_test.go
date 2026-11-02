package mcp

import (
	"context"
	"log/slog"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yourname/mcp-x/internal/config"
	"github.com/yourname/mcp-x/internal/datasource"
	"github.com/yourname/mcp-x/internal/desktop"
)

func newTestServer(t *testing.T, desktopCfg config.DesktopConfig) *Server {
	t.Helper()
	cfg := &config.Config{
		Server:  config.ServerConfig{Name: "mcp-x", Version: "test"},
		Safety:  config.SafetyConfig{Mode: "read-write"},
		Desktop: desktopCfg,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr, err := datasource.NewManager(nil, logger)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() })
	s, err := NewServer(cfg.Server.Name, cfg.Server.Version, mgr, cfg, logger)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return s
}

// desktopEnabledConfig mirrors what config.applyDefaults produces, so the test
// exercises the same registration path as a real config file.
func desktopEnabledConfig(t *testing.T) config.DesktopConfig {
	t.Helper()
	return config.DesktopConfig{
		Enabled:             true,
		MaxActionsPerMinute: 60,
		PositionTolerancePx: 2,
		EdgeMargin:          1,
		ConfirmTTL:          config.Duration(60 * time.Second),
		DpiAware:            true,
		Mouse: config.DesktopMouseCfg{
			Enabled:          true,
			AllowButtons:     []string{"left", "right", "middle"},
			AllowDoubleClick: true,
			AllowDrag:        true,
			DragSteps:        20,
			DragDurationMs:   300,
		},
		Keyboard: config.DesktopKeyboardCfg{
			Enabled: true, RequireConfirm: true, MaxTextLength: 2000,
		},
		Screenshot: config.DesktopShotCfg{
			Enabled: true, Workdir: t.TempDir(), MaxWidth: 1600, AllowFullScreen: true,
		},
	}
}

// listToolNames performs a real tools/list round trip over an in-memory
// transport, so the assertions reflect what the AI would actually see.
func listToolNames(t *testing.T, s *Server) map[string]bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientT, serverT := mcp.NewInMemoryTransports()
	ss, err := s.server.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	defer ss.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer cs.Close()

	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	names := make(map[string]bool, len(res.Tools))
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	return names
}

func requireWindows(t *testing.T) {
	t.Helper()
	if !desktop.Available() {
		t.Skip("desktop control is only available on Windows")
	}
}

func TestDesktopToolsNotRegisteredWhenDisabled(t *testing.T) {
	requireWindows(t)
	s := newTestServer(t, config.DesktopConfig{})
	if s.desktop != nil {
		t.Error("controller must stay nil when the module is disabled")
	}
	names := listToolNames(t, s)
	for name := range names {
		if strings.HasPrefix(name, "desktop_") {
			t.Errorf("no desktop tool may register while disabled, found %s", name)
		}
	}
}

func TestDesktopToolsRegisteredOnWindows(t *testing.T) {
	requireWindows(t)
	s := newTestServer(t, desktopEnabledConfig(t))
	if s.desktop == nil {
		t.Fatal("controller must be built when desktop.enabled is true")
	}
	names := listToolNames(t, s)
	want := []string{
		desktop.ToolScreenInfo, desktop.ToolListWindows,
		desktop.ToolMouseMove, desktop.ToolMouseClick,
		desktop.ToolMouseDrag, desktop.ToolMouseScroll,
		desktop.ToolTypeText, desktop.ToolKey, desktop.ToolScreenshot,
	}
	var missing []string
	for _, name := range want {
		if !names[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("tools not registered: %v", missing)
	}
}

func TestDesktopKeyboardToolsHiddenWhenDisabled(t *testing.T) {
	requireWindows(t)
	cfg := desktopEnabledConfig(t)
	cfg.Keyboard.Enabled = false
	s := newTestServer(t, cfg)
	names := listToolNames(t, s)
	if names[desktop.ToolTypeText] || names[desktop.ToolKey] {
		t.Error("keyboard tools must stay hidden while keyboard.enabled is false")
	}
	if !names[desktop.ToolMouseClick] {
		t.Error("mouse tools should be unaffected by the keyboard switch")
	}
}

func TestDesktopMouseToolsHiddenWhenMouseDisabled(t *testing.T) {
	requireWindows(t)
	cfg := desktopEnabledConfig(t)
	cfg.Mouse.Enabled = false
	s := newTestServer(t, cfg)
	names := listToolNames(t, s)
	for _, name := range []string{
		desktop.ToolMouseMove, desktop.ToolMouseClick,
		desktop.ToolMouseDrag, desktop.ToolMouseScroll,
	} {
		if names[name] {
			t.Errorf("tool %s must not register while mouse.enabled is false", name)
		}
	}
	if !names[desktop.ToolScreenInfo] {
		t.Error("read-only tools should still register")
	}
}

func TestDesktopScreenshotHiddenWhenDisabled(t *testing.T) {
	requireWindows(t)
	cfg := desktopEnabledConfig(t)
	cfg.Screenshot.Enabled = false
	s := newTestServer(t, cfg)
	if listToolNames(t, s)[desktop.ToolScreenshot] {
		t.Error("screenshot must not register while screenshot.enabled is false")
	}
}

func TestDesktopDragToolHiddenWhenDragDisabled(t *testing.T) {
	requireWindows(t)
	cfg := desktopEnabledConfig(t)
	cfg.Mouse.AllowDrag = false
	s := newTestServer(t, cfg)
	if listToolNames(t, s)[desktop.ToolMouseDrag] {
		t.Error("drag must not register while allow_drag is false")
	}
}

// The AI-facing schema is the contract it plans against, so the coordinate
// fields must be visible and required rather than optional.
func TestDesktopClickSchemaExposesCoordinateContract(t *testing.T) {
	requireWindows(t)
	s := newTestServer(t, desktopEnabledConfig(t))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientT, serverT := mcp.NewInMemoryTransports()
	ss, err := s.server.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).
		Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer cs.Close()

	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	for _, tool := range res.Tools {
		if tool.Name != desktop.ToolMouseClick {
			continue
		}
		schema, ok := tool.InputSchema.(map[string]any)
		if !ok {
			t.Fatalf("unexpected input schema type %T", tool.InputSchema)
		}
		props, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("schema has no properties: %v", schema)
		}
		for _, field := range []string{"window_title", "window_x", "window_y", "mouse_x", "mouse_y"} {
			if _, present := props[field]; !present {
				t.Errorf("click schema is missing %q", field)
			}
		}
		required, _ := schema["required"].([]any)
		requiredSet := make(map[string]bool, len(required))
		for _, r := range required {
			if s, ok := r.(string); ok {
				requiredSet[s] = true
			}
		}
		for _, field := range []string{"window_title", "window_x", "window_y", "mouse_x", "mouse_y"} {
			if !requiredSet[field] {
				t.Errorf("%q must be required; the service cannot derive a point without it", field)
			}
		}
		// The tool description is the AI's only guide before it calls.
		if !strings.Contains(tool.Description, "window") {
			t.Errorf("click description should mention the window contract: %q", tool.Description)
		}
		return
	}
	t.Fatal("desktop_mouse_click not registered")
}

func TestScreenInfoToolReportsLayoutOrFails(t *testing.T) {
	requireWindows(t)
	s := newTestServer(t, desktopEnabledConfig(t))
	// A real interactive session is needed for the display APIs; in a headless
	// or service context the handler must fail rather than invent a layout.
	_, out, err := s.handleDesktopScreenInfo(context.Background(), nil, screenInfoInput{})
	if err != nil {
		t.Skipf("no interactive desktop in this environment: %v", err)
	}
	if !strings.Contains(out.Text, "Virtual desktop") {
		t.Errorf("screen info should describe the virtual desktop:\n%s", out.Text)
	}
}

func TestRenderPlanShowsAbsolutePoint(t *testing.T) {
	plan := desktop.Plan{
		Window: desktop.WindowInfo{
			Title: "notepad - test", Class: "Notepad", Process: "notepad.exe",
			Rect: desktop.Rect{X: 100, Y: 200, Width: 800, Height: 600},
		},
		Point:       desktop.Point{X: 140, Y: 260},
		Fingerprint: "fp",
		Preview:     true,
	}
	s := newTestServer(t, config.DesktopConfig{})
	text := s.renderPlan(desktop.ToolMouseClick, desktop.Result{
		Plan: plan, Executed: false, Detail: "preview passed, nothing executed",
	})
	for _, want := range []string{"(140,260)", "notepad - test", "dry_run"} {
		if !strings.Contains(text, want) {
			t.Errorf("plan text missing %q:\n%s", want, text)
		}
	}
}

// A dry run must hand back the plan_token, otherwise the AI cannot complete
// the two-phase handshake.
func TestRenderPlanIncludesTokenOnPreview(t *testing.T) {
	cfg := desktopEnabledConfig(t)
	s := newTestServer(t, cfg)
	plan := desktop.Plan{
		Window:       desktop.WindowInfo{Title: "notepad - test", Rect: desktop.Rect{X: 0, Y: 0, Width: 100, Height: 100}},
		Point:        desktop.Point{X: 10, Y: 10},
		Preview:      true,
		Token:        "tok-abc",
		TokenExpires: time.Now().Add(time.Minute),
	}
	text := s.renderPlan(desktop.ToolMouseClick, desktop.Result{Plan: plan})
	if !strings.Contains(text, "plan_token") || !strings.Contains(text, "tok-abc") {
		t.Errorf("a preview must expose the plan_token:\n%s", text)
	}
}

// When confirmation is mandatory, a successful call must say how to proceed,
// otherwise the AI has no way to learn the handshake exists.
func TestRenderPlanExplainsConfirmRequirement(t *testing.T) {
	cfg := desktopEnabledConfig(t)
	cfg.ConfirmRequired = true
	s := newTestServer(t, cfg)
	plan := desktop.Plan{
		Window: desktop.WindowInfo{Title: "notepad - test", Rect: desktop.Rect{X: 0, Y: 0, Width: 100, Height: 100}},
		Point:  desktop.Point{X: 10, Y: 10},
	}
	text := s.renderPlan(desktop.ToolMouseClick, desktop.Result{Plan: plan, Executed: true})
	if !strings.Contains(text, "dry_run") {
		t.Errorf("an executed call under confirm_required must mention dry_run:\n%s", text)
	}
}
