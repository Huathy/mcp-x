package desktop

import (
	"errors"
	"fmt"
	"strings"
)

// MoveRequest moves the cursor to a validated point without pressing buttons.
type MoveRequest struct {
	Target      Target
	ScreenToken string
	DryRun      bool
	Token       string
}

// allowedKeys is a fixed whitelist. Accepting a free-form combo string would
// let a caller synthesize arbitrary hotkeys (Win+R, Alt+F4, Ctrl+Shift+Del),
// so modifiers are exposed as separate booleans instead.
var allowedKeys = map[string]struct{}{
	"ENTER": {}, "TAB": {}, "ESC": {}, "BACKSPACE": {}, "DELETE": {},
	"UP": {}, "DOWN": {}, "LEFT": {}, "RIGHT": {},
	"HOME": {}, "END": {}, "PAGEUP": {}, "PAGEDOWN": {},
	"SPACE": {},
	"F1":    {}, "F2": {}, "F3": {}, "F4": {}, "F5": {}, "F6": {},
	"F7": {}, "F8": {}, "F9": {}, "F10": {}, "F11": {}, "F12": {},
}

// AllowedKeys returns the whitelist in a fixed order so the MCP schema
// description and error text stay stable across runs (map order does not).
func AllowedKeys() []string {
	return []string{
		"ENTER", "TAB", "ESC", "BACKSPACE", "DELETE", "SPACE",
		"UP", "DOWN", "LEFT", "RIGHT",
		"HOME", "END", "PAGEUP", "PAGEDOWN",
		"F1", "F2", "F3", "F4", "F5", "F6",
		"F7", "F8", "F9", "F10", "F11", "F12",
	}
}

// AllowedKey reports whether key is in the whitelist. Lookup is
// case-insensitive so the AI does not have to guess the casing.
func AllowedKey(key string) bool {
	_, ok := allowedKeys[strings.ToUpper(strings.TrimSpace(key))]
	return ok
}

func joinKeys(keys []string) string { return strings.Join(keys, ", ") }

// asDenyError is a local errors.As shim kept explicit so the intent reads
// clearly at the call site.
func asDenyError(err error, target **DenyError) bool {
	return errors.As(err, target)
}

// Screenshot captures the whole screen or a validated region. Region scope
// runs the same validation chain as a click, so a screenshot cannot be used
// to peek at arbitrary screen areas.
func (c *Controller) Screenshot(req ScreenshotRequest) (Result, error) {
	if !c.cfg.Capabilities.ScreenshotEnabled {
		return Result{}, &DenyError{
			Code: DenyDisabled, Reason: "desktop.screenshot.enabled = false",
			Suggest: "由用户开启 desktop.screenshot.enabled",
		}
	}
	started := nowFunc()
	deny := func(err error) (Result, error) {
		c.audit(ToolScreenshot, "", "", "", 0, 0, "", "deny", codeOf(err), started, err)
		return Result{}, err
	}

	screen, err := c.backend.ScreenInfo()
	if err != nil {
		return deny(&DenyError{
			Code:    DenyParam,
			Reason:  fmt.Sprintf("读取屏幕信息失败：%v", err),
			Suggest: "确认 mcp-x 运行在交互式桌面会话中",
		})
	}
	if len(screen.Monitors) == 0 {
		return deny(&DenyError{
			Code:    DenyNoInteractiveDesk,
			Reason:  "未检测到任何显示器",
			Suggest: "在用户登录的桌面会话中启动 mcp-x",
		})
	}
	if err := c.preflight(ToolScreenshot, screen, req.ScreenToken); err != nil {
		return deny(err)
	}

	scope := req.Scope
	if scope == "" {
		scope = "screen"
	}
	switch scope {
	case "screen":
		if !c.cfg.Capabilities.AllowFullScreen {
			return deny(&DenyError{
				Code:    DenyFullScreenBlocked,
				Reason:  "desktop.screenshot.allow_fullscreen = false，禁止全屏截图",
				Suggest: "改用 scope=window 或 scope=region 截取指定区域",
			})
		}
	case "window", "region":
		if !req.HasTarget {
			return deny(&DenyError{
				Code:    DenyParam,
				Reason:  fmt.Sprintf("scope=%s 需要提供 window_title/window_x/window_y/mouse_x/mouse_y", scope),
				Suggest: "先调用 desktop_list_windows 取得窗口信息",
			})
		}
	default:
		return deny(&DenyError{
			Code:    DenyParam,
			Reason:  fmt.Sprintf("scope 只能是 screen/window/region，收到 %q", scope),
			Suggest: "改用标准 scope 值",
		})
	}

	var (
		rect  Rect
		win   WindowInfo
		point Point
	)
	if scope == "screen" {
		rect = screen.Virtual
	} else {
		wins, err := c.backend.ListWindows()
		if err != nil {
			return deny(&DenyError{
				Code:    DenyParam,
				Reason:  fmt.Sprintf("枚举窗口失败：%v", err),
				Suggest: "重试一次",
			})
		}
		w, p, err := c.resolve(screen, wins, req.Target)
		if err != nil {
			return deny(err)
		}
		win, point = w, p
		if scope == "window" {
			rect = w.Rect
		} else {
			// region: a small rectangle around the requested point, clipped
			// to the window so the capture cannot leak surrounding content.
			rect = regionAround(w, p)
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	img, err := c.backend.Screenshot(rect)
	if err != nil {
		c.audit(ToolScreenshot, win.Title, targetCoords(req.Target), win.Rect.String(),
			point.X, point.Y, screen.Fingerprint(), "error", "", started, err)
		return Result{}, fmt.Errorf("screenshot %s: %w", rect.String(), err)
	}
	plan := Plan{
		Window:      win,
		Target:      req.Target,
		Point:       point,
		Screen:      screen,
		Fingerprint: screen.Fingerprint(),
		Extra:       map[string]any{"scope": scope, "rect": rect, "image": img},
	}
	c.audit(ToolScreenshot, win.Title, targetCoords(req.Target), win.Rect.String(),
		point.X, point.Y, screen.Fingerprint(), "allow", "", started, nil)
	return Result{Plan: plan, Executed: true,
		Detail: fmt.Sprintf("已截取 %s（%s）", rect.String(), scope)}, nil
}

// regionRadius is the half-size in pixels of a `region` capture around the
// requested point.
const regionRadius = 200

func regionAround(win WindowInfo, p Point) Rect {
	x, y := p.X-regionRadius, p.Y-regionRadius
	w, h := regionRadius*2, regionRadius*2
	if x < win.Rect.X {
		w += x - win.Rect.X
		x = win.Rect.X
	}
	if y < win.Rect.Y {
		h += y - win.Rect.Y
		y = win.Rect.Y
	}
	if x+w > win.Rect.Right() {
		w = win.Rect.Right() - x
	}
	if y+h > win.Rect.Bottom() {
		h = win.Rect.Bottom() - y
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return Rect{X: x, Y: y, Width: w, Height: h}
}
