package mcp

import (
	"context"
	"fmt"
	"image"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yourname/mcp-x/internal/desktop"
	"github.com/yourname/mcp-x/internal/safety"
)

// targetArgs is the coordinate contract shared by every point-based tool.
// Callers never pass an absolute point: the service computes it from the
// window origin plus an offset relative to that origin, which is what makes
// the cross-check against the real window position possible.
type targetArgs struct {
	WindowTitle string `json:"window_title" jsonschema:"exact window title as returned by desktop_list_windows; must be unambiguous"`
	WindowX     int    `json:"window_x" jsonschema:"window left edge on screen, exactly as returned by desktop_list_windows"`
	WindowY     int    `json:"window_y" jsonschema:"window top edge on screen, exactly as returned by desktop_list_windows"`
	MouseX      int    `json:"mouse_x" jsonschema:"target X offset relative to the window's left edge (non-negative, inside the window)"`
	MouseY      int    `json:"mouse_y" jsonschema:"target Y offset relative to the window's top edge (non-negative, inside the window)"`
}

type previewArgs struct {
	ScreenToken string `json:"screen_token,omitempty" jsonschema:"fingerprint from desktop_screen_info; pass it back so a changed display layout is detected"`
	DryRun      bool   `json:"dry_run,omitempty" jsonschema:"validate only: returns the absolute point and the matched window without moving the cursor"`
	Token       string `json:"plan_token,omitempty" jsonschema:"token returned by a prior dry_run of the identical call; required when confirm_required is on"`
}

type desktopOutput struct {
	Text string `json:"text" jsonschema:"result as text"`
}

func (s *Server) registerDesktopTools() {
	if !s.cfg.Desktop.Enabled || !desktop.Available() {
		return
	}
	ctrl, err := s.newDesktopController()
	if err != nil {
		s.logger.Error("desktop control unavailable", "err", err)
		return
	}
	s.desktop = ctrl

	desktop.CheckWriteFunc(func() error {
		return safety.New(s.cfg.Safety).CheckWrite()
	})

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        desktop.ToolScreenInfo,
		Description: "Get the desktop display layout: primary resolution, virtual desktop rect, per-monitor DPI, current cursor position and the screen fingerprint. Call this first; pass the fingerprint back as screen_token on later calls.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: boolPtr(false)},
	}, s.handleDesktopScreenInfo)

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        desktop.ToolListWindows,
		Description: "List visible top-level windows with title, rect, process and foreground flag. Use this to obtain the exact window_x/window_y to pass to the mouse tools; the title must be echoed back verbatim.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: boolPtr(false)},
	}, s.handleDesktopListWindows)

	if s.cfg.Desktop.Mouse.Enabled {
		mcp.AddTool(s.server, &mcp.Tool{
			Name:        desktop.ToolMouseMove,
			Description: "Move the mouse cursor to a point inside a window without clicking. Use desktop_mouse_click for an actual click.",
			Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
		}, s.handleDesktopMouseMove)

		mcp.AddTool(s.server, &mcp.Tool{
			Name:        desktop.ToolMouseClick,
			Description: "Click at a point inside a window. The service re-reads the real window position from Windows and refuses the call if the window has moved, is minimized, is covered by another window, or is not focused.",
			Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
		}, s.handleDesktopMouseClick)

		mcp.AddTool(s.server, &mcp.Tool{
			Name:        desktop.ToolMouseScroll,
			Description: "Scroll the mouse wheel at a point inside a window.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: boolPtr(false)},
		}, s.handleDesktopMouseScroll)

		if s.cfg.Desktop.Mouse.AllowDrag {
			mcp.AddTool(s.server, &mcp.Tool{
				Name:        desktop.ToolMouseDrag,
				Description: "Drag from one point to another inside the same window, with interpolated movement. Both endpoints are validated; neither may fall outside the target window.",
				Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
			}, s.handleDesktopMouseDrag)
		}
	}

	if s.cfg.Desktop.Keyboard.Enabled {
		kbNote := ""
		if s.cfg.Desktop.Keyboard.RequireConfirm {
			kbNote = " Two-phase confirmation is mandatory: call with dry_run=true first and pass the returned plan_token."
		}
		mcp.AddTool(s.server, &mcp.Tool{
			Name:        desktop.ToolTypeText,
			Description: "Type Unicode text (including Chinese) into the focused window by moving the cursor to the target point first." + kbNote,
			Annotations: &mcp.ToolAnnotations{},
		}, s.handleDesktopTypeText)

		mcp.AddTool(s.server, &mcp.Tool{
			Name:        desktop.ToolKey,
			Description: fmt.Sprintf("Press one whitelisted key (%s) with optional Ctrl/Alt/Shift modifiers at a point inside a window. Arbitrary key strings are rejected on purpose.", strings.Join(desktop.AllowedKeys(), "/")) + kbNote,
			Annotations: &mcp.ToolAnnotations{},
		}, s.handleDesktopKey)
	}

	if s.cfg.Desktop.Screenshot.Enabled {
		mcp.AddTool(s.server, &mcp.Tool{
			Name:        desktop.ToolScreenshot,
			Description: "Capture the screen, a window, or a region around a point. Window and region scopes go through the same window validation as a click.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: boolPtr(false)},
		}, s.handleDesktopScreenshot)
	}
}

func boolPtr(b bool) *bool { return &b }

func (s *Server) newDesktopController() (*desktop.Controller, error) {
	dcfg := s.cfg.Desktop
	backend, err := desktop.NewBackend(dcfg.DpiAware)
	if err != nil {
		return nil, err
	}
	regions := make([]desktop.Region, 0, len(dcfg.BlockedRegions))
	for _, r := range dcfg.BlockedRegions {
		regions = append(regions, desktop.Region{
			X: r.X, Y: r.Y, Width: r.Width, Height: r.Height, Reason: r.Reason,
		})
	}
	cfg := desktop.Config{
		Enabled:             dcfg.Enabled,
		MaxActionsPerMinute: dcfg.MaxActionsPerMinute,
		PositionTolerancePx: dcfg.PositionTolerancePx,
		RequireForeground:   dcfg.RequireForeground,
		EdgeMargin:          dcfg.EdgeMargin,
		ConfirmRequired:     dcfg.ConfirmRequired,
		ConfirmTTL:          dcfg.ConfirmTTL.Std(),
		BlockedRegions:      regions,
		Capabilities: desktop.Capabilities{
			MouseEnabled:           dcfg.Mouse.Enabled,
			MouseButtons:           dcfg.Mouse.AllowButtons,
			MouseDoubleClick:       dcfg.Mouse.AllowDoubleClick,
			MouseDrag:              dcfg.Mouse.AllowDrag,
			DragSteps:              dcfg.Mouse.DragSteps,
			DragDurationMs:         dcfg.Mouse.DragDurationMs,
			KeyboardEnabled:        dcfg.Keyboard.Enabled,
			KeyboardRequireConfirm: dcfg.Keyboard.Enabled && dcfg.Keyboard.RequireConfirm,
			MaxTextLength:          dcfg.Keyboard.MaxTextLength,
			ScreenshotEnabled:      dcfg.Screenshot.Enabled,
			AllowFullScreen:        dcfg.Screenshot.AllowFullScreen,
		},
	}
	return desktop.NewController(cfg, backend, desktopAudit{s.logger})
}

// desktopAudit adapts the shared slog logger to the desktop audit sink.
type desktopAudit struct{ logger *slog.Logger }

func (a desktopAudit) Audit(tool, windowTitle, passed, real string, absX, absY int,
	fingerprint, decision string, code desktop.DenyCode, elapsedMs int64, err error) {
	attrs := []any{
		"tool", tool,
		"window", windowTitle,
		"passed", passed,
		"real", real,
		"abs", fmt.Sprintf("(%d,%d)", absX, absY),
		"screen", fingerprint,
		"decision", decision,
		"elapsed_ms", elapsedMs,
	}
	if code != "" {
		attrs = append(attrs, "deny_code", string(code))
	}
	if err != nil {
		attrs = append(attrs, "err", err.Error())
	}
	a.logger.Info("desktop action", attrs...)
}

func targetOf(t targetArgs) desktop.Target {
	return desktop.Target{
		WindowTitle: t.WindowTitle,
		WindowX:     t.WindowX,
		WindowY:     t.WindowY,
		MouseX:      t.MouseX,
		MouseY:      t.MouseY,
	}
}

type screenInfoInput struct{}

type screenInfoOutput struct {
	Text string `json:"text" jsonschema:"display layout as text"`
}

func (s *Server) handleDesktopScreenInfo(ctx context.Context, req *mcp.CallToolRequest, in screenInfoInput) (*mcp.CallToolResult, screenInfoOutput, error) {
	info, err := s.desktop.ScreenInfo()
	if err != nil {
		return nil, screenInfoOutput{}, err
	}
	var sb strings.Builder
	sb.WriteString("## Desktop Screen Info\n\n")
	fmt.Fprintf(&sb, "**Primary**: %dx%d\n", info.PrimaryWidth, info.PrimaryHeight)
	fmt.Fprintf(&sb, "**Virtual desktop**: %s\n", info.Virtual.String())
	fmt.Fprintf(&sb, "**DPI**: %d (scale %d%%)\n", info.Dpi, info.ScalePct)
	fmt.Fprintf(&sb, "**Cursor**: (%d,%d)\n", info.CursorX, info.CursorY)
	fg := info.Foreground
	if fg == "" {
		fg = "(none)"
	}
	fmt.Fprintf(&sb, "**Foreground window**: %s\n", fg)

	if len(info.Monitors) > 0 {
		sb.WriteString("\n### Monitors\n\n| # | Primary | Device | Rect | DPI | Scale |\n")
		sb.WriteString("|---|---------|--------|------|-----|-------|\n")
		for _, m := range info.Monitors {
			primary := ""
			if m.Primary {
				primary = "yes"
			}
			fmt.Fprintf(&sb, "| %d | %s | %s | %s | %d | %d%% |\n",
				m.Index, primary, m.Device, m.Rect.String(), m.Dpi, m.ScalePct)
		}
	}

	sb.WriteString("\n### Usage\n")
	fmt.Fprintf(&sb, "Pass `%q` back as `screen_token` on every later call.\n", info.Fingerprint())
	sb.WriteString("Workflow: desktop_list_windows -> use its window_x/window_y plus a mouse offset -> dry_run first -> execute.")

	d := s.cfg.Desktop
	fmt.Fprintf(&sb, "\n### Capabilities\n\n- mouse: %t (buttons %v, double_click %t, drag %t)\n",
		d.Mouse.Enabled, d.Mouse.AllowButtons, d.Mouse.AllowDoubleClick, d.Mouse.AllowDrag)
	fmt.Fprintf(&sb, "- keyboard: %t", d.Keyboard.Enabled)
	if d.Keyboard.Enabled && d.Keyboard.RequireConfirm {
		sb.WriteString(" (two-phase confirmation required)")
	}
	sb.WriteString("\n")
	fmt.Fprintf(&sb, "- screenshot: %t (fullscreen %t)\n", d.Screenshot.Enabled, d.Screenshot.AllowFullScreen)
	fmt.Fprintf(&sb, "- require_foreground: %t, position_tolerance: %dpx, edge_margin: %dpx, confirm_required: %t",
		d.RequireForeground, d.PositionTolerancePx, d.EdgeMargin, d.ConfirmRequired)
	if len(d.BlockedRegions) > 0 {
		sb.WriteString("\n\n### Blocked regions\n\n| Rect | Reason |\n|------|--------|\n")
		for _, r := range d.BlockedRegions {
			reason := r.Reason
			if reason == "" {
				reason = "(none)"
			}
			fmt.Fprintf(&sb, "| (%d,%d %dx%d) | %s |\n", r.X, r.Y, r.Width, r.Height, reason)
		}
	}

	return nil, screenInfoOutput{Text: sb.String()}, nil
}

type listWindowsInput struct {
	Filter           string `json:"filter,omitempty" jsonschema:"case-insensitive substring of the window title"`
	Limit            int    `json:"limit,omitempty" jsonschema:"max windows to return (default 50)"`
	IncludeOffscreen bool   `json:"include_offscreen,omitempty" jsonschema:"include minimized windows (default false)"`
}

type listWindowsOutput struct {
	Text string `json:"text" jsonschema:"window list as text"`
}

func (s *Server) handleDesktopListWindows(ctx context.Context, req *mcp.CallToolRequest, in listWindowsInput) (*mcp.CallToolResult, listWindowsOutput, error) {
	limit := in.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	wins, err := s.desktop.ListWindows(in.Filter, limit)
	if err != nil {
		return nil, listWindowsOutput{}, err
	}
	visible := make([]desktop.WindowInfo, 0, len(wins))
	for _, w := range wins {
		if w.Minimized && !in.IncludeOffscreen {
			continue
		}
		visible = append(visible, w)
	}
	if len(visible) == 0 {
		return nil, listWindowsOutput{Text: "## Windows\n\nNo matching windows. A minimized or service (Session 0) context yields an empty list."}, nil
	}
	// Foreground first: it is the only window that cannot be occluded, so it is
	// the one the AI can act on without a second round trip.
	sort.SliceStable(visible, func(i, j int) bool {
		if visible[i].Foreground != visible[j].Foreground {
			return visible[i].Foreground
		}
		return visible[i].Title < visible[j].Title
	})

	var sb strings.Builder
	fmt.Fprintf(&sb, "## Windows (%d)\n\n", len(visible))
	sb.WriteString("| # | Title | Class | X | Y | W | H | Process | Flags |\n")
	sb.WriteString("|---|-------|-------|---|---|---|---|---------|-------|\n")
	for i, w := range visible {
		flags := make([]string, 0, 3)
		if w.Foreground {
			flags = append(flags, "foreground")
		}
		if w.Minimized {
			flags = append(flags, "minimized")
		}
		if !w.Visible {
			flags = append(flags, "hidden")
		}
		flagStr := "-"
		if len(flags) > 0 {
			flagStr = strings.Join(flags, ",")
		}
		fmt.Fprintf(&sb, "| %d | %s | %s | %d | %d | %d | %d | %s | %s |\n",
			i+1, w.Title, w.Class, w.Rect.X, w.Rect.Y, w.Rect.Width, w.Rect.Height,
			w.Process, flagStr)
	}
	sb.WriteString("\nUse `Title` verbatim as `window_title`, and its X/Y as `window_x`/`window_y`.")
	sb.WriteString("\n`mouse_x`/`mouse_y` are offsets from the window's top-left corner.")
	sb.WriteString("\nPrefer the foreground window: a covered window is rejected by the occlusion check.")
	return nil, listWindowsOutput{Text: sb.String()}, nil
}

type mouseMoveInput struct {
	targetArgs
	previewArgs
}

type mouseMoveOutput struct {
	Text string `json:"text" jsonschema:"result as text"`
}

func (s *Server) handleDesktopMouseMove(ctx context.Context, req *mcp.CallToolRequest, in mouseMoveInput) (*mcp.CallToolResult, mouseMoveOutput, error) {
	res, err := s.desktop.Move(desktop.MoveRequest{
		Target:      targetOf(in.targetArgs),
		ScreenToken: in.ScreenToken,
		DryRun:      in.DryRun,
		Token:       in.Token,
	})
	if err != nil {
		return nil, mouseMoveOutput{}, err
	}
	return nil, mouseMoveOutput{Text: s.renderPlan(desktop.ToolMouseMove, res)}, nil
}

type mouseClickInput struct {
	targetArgs
	previewArgs
	Button string `json:"button,omitempty" jsonschema:"left (default) | right | middle"`
	Count  int    `json:"count,omitempty" jsonschema:"1 = single click, 2 = double click (default 1)"`
}

type mouseClickOutput struct {
	Text string `json:"text" jsonschema:"result as text"`
}

func (s *Server) handleDesktopMouseClick(ctx context.Context, req *mcp.CallToolRequest, in mouseClickInput) (*mcp.CallToolResult, mouseClickOutput, error) {
	res, err := s.desktop.Click(desktop.ClickRequest{
		Target:      targetOf(in.targetArgs),
		Button:      in.Button,
		Count:       in.Count,
		ScreenToken: in.ScreenToken,
		DryRun:      in.DryRun,
		Token:       in.Token,
	})
	if err != nil {
		return nil, mouseClickOutput{}, err
	}
	return nil, mouseClickOutput{Text: s.renderPlan(desktop.ToolMouseClick, res)}, nil
}

type mouseDragInput struct {
	targetArgs
	previewArgs
	FromX  int    `json:"from_x" jsonschema:"drag start X offset relative to the window's left edge"`
	FromY  int    `json:"from_y" jsonschema:"drag start Y offset relative to the window's top edge"`
	ToX    int    `json:"to_x" jsonschema:"drag end X offset relative to the window's left edge"`
	ToY    int    `json:"to_y" jsonschema:"drag end Y offset relative to the window's top edge"`
	Button string `json:"button,omitempty" jsonschema:"left (default) | right | middle"`
}

type mouseDragOutput struct {
	Text string `json:"text" jsonschema:"result as text"`
}

func (s *Server) handleDesktopMouseDrag(ctx context.Context, req *mcp.CallToolRequest, in mouseDragInput) (*mcp.CallToolResult, mouseDragOutput, error) {
	res, err := s.desktop.Drag(desktop.DragRequest{
		Target:      targetOf(in.targetArgs),
		FromX:       in.FromX,
		FromY:       in.FromY,
		ToX:         in.ToX,
		ToY:         in.ToY,
		Button:      in.Button,
		ScreenToken: in.ScreenToken,
		DryRun:      in.DryRun,
		Token:       in.Token,
	})
	if err != nil {
		return nil, mouseDragOutput{}, err
	}
	return nil, mouseDragOutput{Text: s.renderPlan(desktop.ToolMouseDrag, res)}, nil
}

type mouseScrollInput struct {
	targetArgs
	previewArgs
	Direction string `json:"direction,omitempty" jsonschema:"up | down | left | right (default down)"`
	Notches   int    `json:"notches,omitempty" jsonschema:"wheel notches, max 50 (default 3)"`
}

type mouseScrollOutput struct {
	Text string `json:"text" jsonschema:"result as text"`
}

func (s *Server) handleDesktopMouseScroll(ctx context.Context, req *mcp.CallToolRequest, in mouseScrollInput) (*mcp.CallToolResult, mouseScrollOutput, error) {
	res, err := s.desktop.Scroll(desktop.ScrollRequest{
		Target:      targetOf(in.targetArgs),
		Direction:   in.Direction,
		Notches:     in.Notches,
		ScreenToken: in.ScreenToken,
		DryRun:      in.DryRun,
		Token:       in.Token,
	})
	if err != nil {
		return nil, mouseScrollOutput{}, err
	}
	return nil, mouseScrollOutput{Text: s.renderPlan(desktop.ToolMouseScroll, res)}, nil
}

type typeTextInput struct {
	targetArgs
	previewArgs
	Text string `json:"text" jsonschema:"text to type; Unicode, Chinese supported"`
}

type typeTextOutput struct {
	Text string `json:"text" jsonschema:"result as text"`
}

func (s *Server) handleDesktopTypeText(ctx context.Context, req *mcp.CallToolRequest, in typeTextInput) (*mcp.CallToolResult, typeTextOutput, error) {
	res, err := s.desktop.TypeText(desktop.TextRequest{
		Target:      targetOf(in.targetArgs),
		Text:        in.Text,
		ScreenToken: in.ScreenToken,
		DryRun:      in.DryRun,
		Token:       in.Token,
	})
	if err != nil {
		return nil, typeTextOutput{}, err
	}
	return nil, typeTextOutput{Text: s.renderPlan(desktop.ToolTypeText, res)}, nil
}

type keyInput struct {
	targetArgs
	previewArgs
	Key       string `json:"key" jsonschema:"ENTER | TAB | ESC | BACKSPACE | DELETE | SPACE | UP | DOWN | LEFT | RIGHT | HOME | END | PAGEUP | PAGEDOWN | F1..F12"`
	WithCtrl  bool   `json:"with_ctrl,omitempty" jsonschema:"hold Ctrl"`
	WithAlt   bool   `json:"with_alt,omitempty" jsonschema:"hold Alt"`
	WithShift bool   `json:"with_shift,omitempty" jsonschema:"hold Shift"`
}

type keyOutput struct {
	Text string `json:"text" jsonschema:"result as text"`
}

func (s *Server) handleDesktopKey(ctx context.Context, req *mcp.CallToolRequest, in keyInput) (*mcp.CallToolResult, keyOutput, error) {
	res, err := s.desktop.PressKey(desktop.KeyRequest{
		Target: targetOf(in.targetArgs),
		Key:    in.Key,
		Mods: desktop.Mods{
			Ctrl:  in.WithCtrl,
			Alt:   in.WithAlt,
			Shift: in.WithShift,
		},
		ScreenToken: in.ScreenToken,
		DryRun:      in.DryRun,
		Token:       in.Token,
	})
	if err != nil {
		return nil, keyOutput{}, err
	}
	return nil, keyOutput{Text: s.renderPlan(desktop.ToolKey, res)}, nil
}

type screenshotInput struct {
	Scope        string `json:"scope,omitempty" jsonschema:"screen | window | region (default screen)"`
	WindowTitle  string `json:"window_title,omitempty" jsonschema:"window title; required for scope=window or region"`
	WindowX      int    `json:"window_x,omitempty" jsonschema:"window left edge, required for scope=window or region"`
	WindowY      int    `json:"window_y,omitempty" jsonschema:"window top edge, required for scope=window or region"`
	MouseX       int    `json:"mouse_x,omitempty" jsonschema:"center X offset relative to the window, required for scope=region"`
	MouseY       int    `json:"mouse_y,omitempty" jsonschema:"center Y offset relative to the window, required for scope=region"`
	ScreenToken  string `json:"screen_token,omitempty" jsonschema:"fingerprint from desktop_screen_info"`
	MaxWidth     int    `json:"max_width,omitempty" jsonschema:"downscale so the image is at most this wide (default from config)"`
	ReturnBase64 bool   `json:"return_base64,omitempty" jsonschema:"also return the image inline for direct viewing (default false)"`
}

func (s *Server) handleDesktopScreenshot(ctx context.Context, req *mcp.CallToolRequest, in screenshotInput) (*mcp.CallToolResult, desktopOutput, error) {
	scope := in.Scope
	if scope == "" {
		scope = "screen"
	}
	req2 := desktop.ScreenshotRequest{
		Scope:       scope,
		ScreenToken: in.ScreenToken,
	}
	if strings.TrimSpace(in.WindowTitle) != "" {
		req2.HasTarget = true
		req2.Target = desktop.Target{
			WindowTitle: in.WindowTitle,
			WindowX:     in.WindowX,
			WindowY:     in.WindowY,
			MouseX:      in.MouseX,
			MouseY:      in.MouseY,
		}
	}
	res, err := s.desktop.Screenshot(req2)
	if err != nil {
		return nil, desktopOutput{}, err
	}

	img, _ := res.Plan.Extra["image"].(image.Image)
	rect, _ := res.Plan.Extra["rect"].(desktop.Rect)
	if img == nil {
		return nil, desktopOutput{}, fmt.Errorf("screenshot returned no image data")
	}
	maxWidth := in.MaxWidth
	if maxWidth <= 0 {
		maxWidth = s.cfg.Desktop.Screenshot.MaxWidth
	}
	if img.Bounds().Dx() > maxWidth {
		img = scaleImage(img, maxWidth)
	}

	path, err := s.saveScreenshot(img)
	if err != nil {
		return nil, desktopOutput{}, err
	}
	abs, _ := filepath.Abs(path)
	out := desktopOutput{Text: fmt.Sprintf(
		"## Screenshot\n\n**Scope**: %s\n**Rect**: %s\n**Size**: %dx%d\n**Saved to**: %s\n**Screen fingerprint**: %s",
		scope, rect.String(), img.Bounds().Dx(), img.Bounds().Dy(), abs, res.Plan.Fingerprint)}

	if !in.ReturnBase64 {
		return nil, out, nil
	}
	png, err := encodePNG(img)
	if err != nil {
		return nil, out, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: out.Text},
			&mcp.ImageContent{Data: png, MIMEType: "image/png"},
		},
	}, desktopOutput{}, nil
}

func (s *Server) saveScreenshot(img image.Image) (string, error) {
	dir := s.cfg.Desktop.Screenshot.Workdir
	if dir == "" {
		dir = "./data/desktop"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create screenshot dir: %w", err)
	}
	name := fmt.Sprintf("shot-%s.png", time.Now().Format("20060102-150405.000"))
	path := filepath.Join(dir, name)
	if err := encodeTo(img, path); err != nil {
		return "", err
	}
	return path, nil
}

// renderPlan produces the dry_run / success text. It always states the
// absolute point and the window that was actually matched, so the AI can
// verify the outcome instead of assuming it.
func (s *Server) renderPlan(tool string, res desktop.Result) string {
	var sb strings.Builder
	status := "executed"
	if !res.Executed {
		status = "preview (dry_run, nothing was executed)"
	}
	fmt.Fprintf(&sb, "## %s - %s\n\n", tool, status)
	fmt.Fprintf(&sb, "**Detail**: %s\n", res.Detail)
	fmt.Fprintf(&sb, "**Window**: %q (class %s, process %s)\n", res.Plan.Window.Title, res.Plan.Window.Class, res.Plan.Window.Process)
	fmt.Fprintf(&sb, "**Window rect**: %s\n", res.Plan.Window.Rect.String())
	fmt.Fprintf(&sb, "**Absolute point**: (%d,%d)\n", res.Plan.Point.X, res.Plan.Point.Y)
	fmt.Fprintf(&sb, "**Screen fingerprint**: %s\n", res.Plan.Fingerprint)
	// A dry run issues the token and a real run states that one is required;
	// both halves are needed for the AI to complete the handshake.
	if res.Plan.Preview && res.Plan.Token != "" {
		fmt.Fprintf(&sb, "\n**plan_token** (valid until %s, bound to these exact parameters):\n\n`%s`\n",
			res.Plan.TokenExpires.Format(time.RFC3339), res.Plan.Token)
	}
	if !res.Plan.Preview && s != nil && s.cfg.Desktop.ConfirmRequired {
		sb.WriteString("\nThis configuration requires confirmation: repeat this call with dry_run=true, then pass the returned plan_token as plan_token.\n")
	}
	return sb.String()
}
