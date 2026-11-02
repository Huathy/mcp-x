// Package desktop implements the Windows desktop control tools exposed over
// MCP. The validation chain lives here, free of platform calls, so it can be
// exercised by unit tests against a fake Backend. Only backend_windows.go
// touches user32/gdi32.
package desktop

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"strings"
	"sync"
	"time"
)

var (
	// ErrUnsupportedPlatform is returned by the non-Windows stub backend.
	ErrUnsupportedPlatform = errors.New("桌面控制仅支持 Windows 平台")
	// ErrDisabled is returned when the module or a sub-capability is off.
	ErrDisabled = errors.New("桌面控制未启用")
	// ErrConfirmRequired is returned when a write op lacks a plan token.
	ErrConfirmRequired = errors.New("该操作需先以 dry_run=true 预演并取得 plan_token")
)

// DenyCode identifies which validation step rejected an operation. The code is
// returned to the AI so it can self-correct instead of retrying blindly.
type DenyCode string

const (
	DenyDisabled          DenyCode = "DENY_DISABLED"
	DenySafetyMode        DenyCode = "DENY_SAFETY_MODE"
	DenyParam             DenyCode = "DENY_PARAM"
	DenyRateLimit         DenyCode = "DENY_RATE_LIMIT"
	DenyBlockedRegion     DenyCode = "DENY_BLOCKED_REGION"
	DenyWindowNotFound    DenyCode = "DENY_WINDOW_NOT_FOUND"
	DenyWindowAmbiguous   DenyCode = "DENY_WINDOW_AMBIGUOUS"
	DenyWindowMinimized   DenyCode = "DENY_WINDOW_MINIMIZED"
	DenyWindowInvisible   DenyCode = "DENY_WINDOW_INVISIBLE"
	DenyWindowMoved       DenyCode = "DENY_WINDOW_MOVED"
	DenyOutOfWindow       DenyCode = "DENY_OUT_OF_WINDOW"
	DenyOutOfScreen       DenyCode = "DENY_OUT_OF_SCREEN"
	DenyScreenChanged     DenyCode = "DENY_SCREEN_CHANGED"
	DenyOccluded          DenyCode = "DENY_OCCLUDED"
	DenyNotForeground     DenyCode = "DENY_NOT_FOREGROUND"
	DenyButtonNotAllowed  DenyCode = "DENY_BUTTON_NOT_ALLOWED"
	DenyConfirmRequired   DenyCode = "DENY_CONFIRM_REQUIRED"
	DenyTokenInvalid      DenyCode = "DENY_TOKEN_INVALID"
	DenyTokenExpired      DenyCode = "DENY_TOKEN_EXPIRED"
	DenyTokenScreenStale  DenyCode = "DENY_TOKEN_SCREEN_STALE"
	DenyTextTooLong       DenyCode = "DENY_TEXT_TOO_LONG"
	DenyFullScreenBlocked DenyCode = "DENY_FULLSCREEN_BLOCKED"
	DenyNoInteractiveDesk DenyCode = "DENY_NO_INTERACTIVE_DESKTOP"
)

// Rect is a screen rectangle in physical pixels. Virtual desktops can start at
// negative coordinates when a monitor sits left of / above the primary one.
type Rect struct {
	X      int
	Y      int
	Width  int
	Height int
}

// Right returns the exclusive right edge.
func (r Rect) Right() int { return r.X + r.Width }

// Bottom returns the exclusive bottom edge.
func (r Rect) Bottom() int { return r.Y + r.Height }

// Contains reports whether the point lies inside the rectangle.
func (r Rect) Contains(x, y int) bool {
	return x >= r.X && x < r.Right() && y >= r.Y && y < r.Bottom()
}

func (r Rect) String() string {
	return fmt.Sprintf("(%d,%d %dx%d)", r.X, r.Y, r.Width, r.Height)
}

// Monitor describes one display in a multi-monitor setup.
type Monitor struct {
	Index    int
	Primary  bool
	Device   string
	Rect     Rect
	Dpi      int
	ScalePct int
}

// ScreenInfo is the full picture of the display layout.
type ScreenInfo struct {
	PrimaryWidth  int
	PrimaryHeight int
	Virtual       Rect
	Dpi           int
	ScalePct      int
	Monitors      []Monitor
	CursorX       int
	CursorY       int
	Foreground    string
}

// Fingerprint changes whenever the display layout changes, so a stale
// observation (resolution change, monitor unplug) invalidates old plan tokens.
func (s ScreenInfo) Fingerprint() string {
	parts := []string{
		fmt.Sprintf("p=%dx%d", s.PrimaryWidth, s.PrimaryHeight),
		fmt.Sprintf("v=%d,%d,%d,%d", s.Virtual.X, s.Virtual.Y, s.Virtual.Width, s.Virtual.Height),
		fmt.Sprintf("n=%d", len(s.Monitors)),
	}
	for _, m := range s.Monitors {
		parts = append(parts, fmt.Sprintf("m%d=%s@%d", m.Index, m.Rect.String(), m.Dpi))
	}
	return strings.Join(parts, ";")
}

// WindowInfo describes a top-level window.
type WindowInfo struct {
	Handle     uintptr
	Title      string
	Class      string
	Rect       Rect
	Visible    bool
	Minimized  bool
	Foreground bool
	PID        uint32
	Process    string
}

// Mods are the modifier keys supported for a single key press. Arbitrary
// combinations are not accepted on purpose: a free-form combo string would let
// the caller synthesise hotkeys such as Win+R.
type Mods struct {
	Ctrl  bool
	Alt   bool
	Shift bool
}

func (m Mods) Any() bool { return m.Ctrl || m.Alt || m.Shift }

// Backend is the platform-specific surface used by the validation chain.
// Implementations must be safe for concurrent calls; Controller serializes
// the mutating ones.
type Backend interface {
	// ScreenInfo reports the current display layout. Called on every
	// validation so a stale plan cannot survive a resolution change.
	ScreenInfo() (ScreenInfo, error)
	// ListWindows returns visible top-level windows.
	ListWindows() ([]WindowInfo, error)
	// WindowAtPoint returns the root window that would receive a click at
	// the given screen point. ok is false when nothing is there.
	WindowAtPoint(x, y int) (WindowInfo, bool, error)
	// SetForeground activates a window so clicks land on it.
	SetForeground(hwnd uintptr) error
	// CursorPos returns the current mouse position.
	CursorPos() (int, int, error)
	MoveTo(x, y int) error
	MouseDown(button string) error
	MouseUp(button string) error
	// MouseWheel turns the wheel. horizontal selects the HWHEEL axis;
	// notches is signed, positive meaning up or right.
	MouseWheel(horizontal bool, notches int) error
	TypeText(text string) error
	PressKey(key string, mods Mods) error
	// Screenshot captures the given screen rectangle.
	Screenshot(rect Rect) (image.Image, error)
}

// Region is a rectangle the controller refuses to touch.
type Region struct {
	X      int
	Y      int
	Width  int
	Height int
	Reason string
}

func (r Region) contains(x, y int) bool {
	return x >= r.X && x < r.X+r.Width && y >= r.Y && y < r.Y+r.Height
}

// Capabilities describes which sub-features are enabled.
type Capabilities struct {
	MouseEnabled           bool
	MouseButtons           []string
	MouseDoubleClick       bool
	MouseDrag              bool
	DragSteps              int
	DragDurationMs         int
	KeyboardEnabled        bool
	KeyboardRequireConfirm bool
	MaxTextLength          int
	ScreenshotEnabled      bool
	AllowFullScreen        bool
}

// Config is the resolved controller configuration.
type Config struct {
	Enabled             bool
	MaxActionsPerMinute int
	PositionTolerancePx int
	RequireForeground   bool
	EdgeMargin          int
	ConfirmRequired     bool
	ConfirmTTL          time.Duration
	BlockedRegions      []Region
	Capabilities        Capabilities
}

// Logger records audit entries. Implemented by *slog.Logger at the call site.
type Logger interface {
	Audit(tool, windowTitle, passed, real string, absX, absY int,
		fingerprint, decision string, denyCode DenyCode, elapsedMs int64, err error)
}

// Target identifies a point inside a window, expressed the way the MCP tools
// accept it: window origin plus an offset relative to that origin.
type Target struct {
	WindowTitle string
	WindowX     int
	WindowY     int
	MouseX      int
	MouseY      int
}

// Point is an absolute screen point computed by the controller.
type Point struct {
	X int
	Y int
}

// Plan is the validated, ready-to-execute description of an operation. The
// MCP layer renders it as the dry_run response and as the success summary.
type Plan struct {
	Window       WindowInfo
	Target       Target
	Point        Point
	Screen       ScreenInfo
	Fingerprint  string
	Preview      bool
	Token        string
	TokenExpires time.Time
	Extra        map[string]any
}

// DenyError carries the reject reason plus enough context for the AI to fix
// its own next call.
type DenyError struct {
	Code    DenyCode
	Reason  string
	Suggest string
	Target  *Target
	Passed  string
	Real    string
	Point   *Point
}

func (e *DenyError) Error() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "桌面操作被拒绝 [%s]：%s", e.Code, e.Reason)
	if e.Passed != "" || e.Real != "" {
		fmt.Fprintf(&sb, "\n  传入位置: %s  实际位置: %s", orNone(e.Passed), orNone(e.Real))
	}
	if e.Suggest != "" {
		fmt.Fprintf(&sb, "\n  建议: %s", e.Suggest)
	}
	return sb.String()
}

func orNone(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// controller-clock indirection keeps token TTL testable without sleeping.
var nowFunc = time.Now

// Controller owns the validation chain, rate limiter and token signer.
type Controller struct {
	cfg     Config
	backend Backend
	log     Logger

	mu       sync.Mutex
	attempts []time.Time
	secret   []byte
}

// NewController builds a controller. The secret is generated per process, so
// tokens do not survive a restart.
func NewController(cfg Config, backend Backend, log Logger) (*Controller, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("generate plan token secret: %w", err)
	}
	return &Controller{cfg: cfg, backend: backend, log: log, secret: secret}, nil
}

// Config returns the effective configuration.
func (c *Controller) Config() Config { return c.cfg }

// ScreenInfo proxies the backend and is the source for desktop_screen_info.
func (c *Controller) ScreenInfo() (ScreenInfo, error) {
	return c.backend.ScreenInfo()
}

// ListWindows proxies the backend, applying the title filter and limit.
func (c *Controller) ListWindows(filter string, limit int) ([]WindowInfo, error) {
	wins, err := c.backend.ListWindows()
	if err != nil {
		return nil, err
	}
	if filter != "" {
		lowered := strings.ToLower(filter)
		filtered := make([]WindowInfo, 0, len(wins))
		for _, w := range wins {
			if strings.Contains(strings.ToLower(w.Title), lowered) {
				filtered = append(filtered, w)
			}
		}
		wins = filtered
	}
	if limit > 0 && len(wins) > limit {
		wins = wins[:limit]
	}
	return wins, nil
}

// ---------------------------------------------------------------- validation

// resolve validates a target and returns the absolute point plus the matched
// window. It runs checks C3, C5 through C13 from the design document.
func (c *Controller) resolve(screen ScreenInfo, wins []WindowInfo, t Target) (WindowInfo, Point, error) {
	if t.MouseX < 0 || t.MouseY < 0 {
		return WindowInfo{}, Point{}, &DenyError{
			Code:    DenyParam,
			Reason:  fmt.Sprintf("mouse_x/mouse_y 不能为负数，收到 (%d,%d)", t.MouseX, t.MouseY),
			Suggest: "相对坐标必须是窗口内的非负偏移",
			Target:  &t,
		}
	}

	absX := t.WindowX + t.MouseX
	absY := t.WindowY + t.MouseY
	if absX < screen.Virtual.X || absX >= screen.Virtual.Right() ||
		absY < screen.Virtual.Y || absY >= screen.Virtual.Bottom() {
		return WindowInfo{}, Point{}, &DenyError{
			Code: DenyOutOfScreen,
			Reason: fmt.Sprintf("换算后的绝对坐标 (%d,%d) 超出虚拟桌面 %s",
				absX, absY, screen.Virtual.String()),
			Suggest: "确认 window_x/window_y 与 mouse_x/mouse_y 是否都填对",
			Target:  &t,
			Passed:  fmt.Sprintf("(%d,%d)", absX, absY),
			Point:   &Point{X: absX, Y: absY},
		}
	}

	// C5: blocked regions are checked in absolute coordinates, before any
	// window lookup, so a blocked point is never resolved at all.
	for _, r := range c.cfg.BlockedRegions {
		if r.contains(absX, absY) {
			reason := r.Reason
			if reason == "" {
				reason = "命中配置的黑名单区域"
			}
			return WindowInfo{}, Point{}, &DenyError{
				Code:    DenyBlockedRegion,
				Reason:  fmt.Sprintf("绝对坐标 (%d,%d) %s：%s", absX, absY, blockedRectString(r), reason),
				Suggest: "改选其它位置，或由用户调整 desktop.blocked_regions 配置",
				Target:  &t,
				Point:   &Point{X: absX, Y: absY},
			}
		}
	}

	// C6: window identity.
	win, err := c.matchWindow(wins, t.WindowTitle)
	if err != nil {
		return WindowInfo{}, Point{}, err
	}

	// C7: window state.
	if win.Minimized {
		return WindowInfo{}, Point{}, &DenyError{
			Code:    DenyWindowMinimized,
			Reason:  fmt.Sprintf("目标窗口已最小化，rect %s 不可用于计算点击位置", win.Rect.String()),
			Suggest: "先让用户还原该窗口，再重新调用 desktop_list_windows",
			Target:  &t,
			Real:    win.Rect.String(),
		}
	}
	if !win.Visible {
		return WindowInfo{}, Point{}, &DenyError{
			Code:    DenyWindowInvisible,
			Reason:  fmt.Sprintf("目标窗口不可见：%q", win.Title),
			Suggest: "换一个可见窗口，或先让用户显示该窗口",
			Target:  &t,
			Real:    win.Rect.String(),
		}
	}
	if win.Rect.Width <= 0 || win.Rect.Height <= 0 {
		return WindowInfo{}, Point{}, &DenyError{
			Code:    DenyWindowInvisible,
			Reason:  fmt.Sprintf("目标窗口尺寸非法：%s", win.Rect.String()),
			Suggest: "重新调用 desktop_list_windows 确认窗口状态",
			Target:  &t,
			Real:    win.Rect.String(),
		}
	}

	// C8: position consistency. This is the check that catches a window moved
	// between the AI's observation and the action.
	tol := c.cfg.PositionTolerancePx
	passed := fmt.Sprintf("(%d,%d)", t.WindowX, t.WindowY)
	real := win.Rect.String()
	if abs(win.Rect.X-t.WindowX) > tol || abs(win.Rect.Y-t.WindowY) > tol {
		return WindowInfo{}, Point{}, &DenyError{
			Code: DenyWindowMoved,
			Reason: fmt.Sprintf("窗口位置已变化，OS 返回 %s，传入 %s，容差 %dpx",
				real, passed, tol),
			Suggest: "重新调用 desktop_list_windows 获取最新坐标后重试",
			Target:  &t,
			Passed:  passed,
			Real:    real,
			Point:   &Point{X: absX, Y: absY},
		}
	}

	// C9: relative bounds with edge margin, so a click never lands on the
	// resize border or the window close button.
	m := c.cfg.EdgeMargin
	if t.MouseX < m || t.MouseX >= win.Rect.Width-m ||
		t.MouseY < m || t.MouseY >= win.Rect.Height-m {
		return WindowInfo{}, Point{}, &DenyError{
			Code: DenyOutOfWindow,
			Reason: fmt.Sprintf("相对坐标 (%d,%d) 超出窗口 %s 的可点击范围（边距 %dpx）",
				t.MouseX, t.MouseY, real, m),
			Suggest: "相对坐标必须在窗口内部，避开最外圈边框",
			Target:  &t,
			Passed:  passed,
			Real:    real,
			Point:   &Point{X: absX, Y: absY},
		}
	}

	// C12: hit test. Catches a click that would land on a different window
	// sitting on top of the target.
	hit, ok, err := c.backend.WindowAtPoint(absX, absY)
	if err != nil {
		return WindowInfo{}, Point{}, &DenyError{
			Code:    DenyParam,
			Reason:  fmt.Sprintf("命中测试失败：%v", err),
			Suggest: "重试一次；若持续失败请检查 mcp-x 与目标程序的运行权限是否一致",
			Target:  &t,
		}
	}
	if !ok {
		return WindowInfo{}, Point{}, &DenyError{
			Code:    DenyOccluded,
			Reason:  fmt.Sprintf("绝对坐标 (%d,%d) 处没有任何可接收输入的窗口", absX, absY),
			Suggest: "确认目标窗口未被其它窗口遮挡",
			Target:  &t,
			Point:   &Point{X: absX, Y: absY},
		}
	}
	if hit.Handle != win.Handle {
		return WindowInfo{}, Point{}, &DenyError{
			Code: DenyOccluded,
			Reason: fmt.Sprintf("绝对坐标 (%d,%d) 实际命中窗口 %q，而非目标窗口 %q（被遮挡）",
				absX, absY, hit.Title, win.Title),
			Suggest: "先把遮挡窗口移开或最小化，再重试",
			Target:  &t,
			Real:    real,
			Point:   &Point{X: absX, Y: absY},
		}
	}

	// C13: foreground check. Clicking coordinates on a background window
	// would otherwise be delivered to whatever has focus.
	if c.cfg.RequireForeground && !win.Foreground {
		fg := screen.Foreground
		if fg == "" {
			fg = "(无)"
		}
		return WindowInfo{}, Point{}, &DenyError{
			Code: DenyNotForeground,
			Reason: fmt.Sprintf("目标窗口 %q 不是前台窗口（当前前台: %s）",
				win.Title, fg),
			Suggest: "让用户点击该窗口使其获得焦点，或先把 desktop.require_foreground 设为 false",
			Target:  &t,
			Real:    real,
			Point:   &Point{X: absX, Y: absY},
		}
	}

	return win, Point{X: absX, Y: absY}, nil
}

func blockedRectString(r Region) string {
	return fmt.Sprintf("位于禁用区域 (%d,%d %dx%d)", r.X, r.Y, r.Width, r.Height)
}

// matchWindow resolves a window title to exactly one window. An empty title is
// rejected: falling back to "the only window" would make mis-clicks likely.
func (c *Controller) matchWindow(wins []WindowInfo, title string) (WindowInfo, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return WindowInfo{}, &DenyError{
			Code:    DenyParam,
			Reason:  "window_title 不能为空",
			Suggest: "先调用 desktop_list_windows 拿到标题，再原样传入",
		}
	}
	var matches []WindowInfo
	for _, w := range wins {
		if w.Title == title {
			matches = append(matches, w)
		}
	}
	if len(matches) == 0 {
		// Fall back to a substring match so a truncated title still works,
		// but only when it is unambiguous.
		for _, w := range wins {
			if strings.Contains(w.Title, title) {
				matches = append(matches, w)
			}
		}
	}
	switch len(matches) {
	case 0:
		return WindowInfo{}, &DenyError{
			Code:    DenyWindowNotFound,
			Reason:  fmt.Sprintf("未找到标题为 %q 的窗口", title),
			Suggest: "窗口可能已关闭，请重新调用 desktop_list_windows",
		}
	case 1:
		return matches[0], nil
	default:
		titles := make([]string, 0, len(matches))
		for _, m := range matches {
			titles = append(titles, fmt.Sprintf("%q@%s", m.Title, m.Rect.String()))
		}
		return WindowInfo{}, &DenyError{
			Code: DenyWindowAmbiguous,
			Reason: fmt.Sprintf("标题 %q 匹配到 %d 个窗口：%s",
				title, len(matches), strings.Join(titles, ", ")),
			Suggest: "改用完整标题（来自 desktop_list_windows）以唯一确定目标窗口",
		}
	}
}

// preflight runs the checks that need no window context: capability gate,
// safety mode, parameter shape, rate limit and screen freshness.
func (c *Controller) preflight(tool string, screen ScreenInfo, screenToken string) error {
	if !c.cfg.Enabled {
		return &DenyError{
			Code:    DenyDisabled,
			Reason:  "desktop.enabled = false",
			Suggest: "由用户在配置中设置 desktop.enabled: true 后重启 mcp-x",
		}
	}
	if err := c.checkSafetyMode(); err != nil {
		return err
	}
	if screenToken != "" && screenToken != screen.Fingerprint() {
		return &DenyError{
			Code: DenyScreenChanged,
			Reason: fmt.Sprintf("传入的 screen_token 与当前屏幕指纹不一致（传入 %q，当前 %q）",
				shortFP(screenToken), shortFP(screen.Fingerprint())),
			Suggest: "屏幕布局可能已变化（改分辨率/拔显示器），请重新调用 desktop_screen_info",
		}
	}
	return nil
}

// checkSafetyModeFn is installed by the MCP layer so desktop writes obey the
// same read-only mode as database writes. It is a package-level hook rather
// than a field so Config stays a plain data struct.
var checkSafetyModeFn func() error

// CheckWriteFunc registers the read-only mode check. Called once at tool
// registration; the desktop package does not otherwise depend on config.
func CheckWriteFunc(fn func() error) { checkSafetyModeFn = fn }

func (c *Controller) checkSafetyMode() error {
	if checkSafetyModeFn == nil {
		return nil
	}
	if err := checkSafetyModeFn(); err != nil {
		return &DenyError{
			Code:    DenySafetyMode,
			Reason:  err.Error(),
			Suggest: "由用户把 safety.mode 改为 read-write，或先用只读方式完成任务",
		}
	}
	return nil
}

// allowRate returns whether one more mutating action fits in the sliding
// window. Actions are counted on real execution only: dry runs are free, so
// the AI can preview without burning its budget.
func (c *Controller) allowRate() error {
	limit := c.cfg.MaxActionsPerMinute
	if limit <= 0 {
		return nil
	}
	now := nowFunc()
	cutoff := now.Add(-time.Minute)
	kept := c.attempts[:0]
	for _, t := range c.attempts {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= limit {
		c.attempts = kept
		return &DenyError{
			Code:    DenyRateLimit,
			Reason:  fmt.Sprintf("已达到每分钟操作上限 %d 次", limit),
			Suggest: "降低操作频率，或由用户调整 desktop.max_actions_per_minute",
		}
	}
	c.attempts = append(kept, now)
	return nil
}

// ------------------------------------------------------------------- tokens

type tokenPayload struct {
	Tool        string `json:"t"`
	ParamsHash  string `json:"p"`
	Fingerprint string `json:"s"`
	Expires     int64  `json:"e"`
}

// hashParams binds a token to one exact set of arguments, so a token issued
// for a click cannot be replayed as a drag.
func hashParams(tool string, parts ...any) string {
	h := sha256.New()
	fmt.Fprint(h, tool)
	for _, p := range parts {
		fmt.Fprintf(h, "\x00%v", p)
	}
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil)[:16])
}

func (c *Controller) issueToken(tool, paramsHash, fingerprint string) (string, time.Time, error) {
	exp := nowFunc().Add(c.confirmTTL())
	if c.confirmTTL() <= 0 {
		exp = nowFunc().Add(60 * time.Second)
	}
	body, err := json.Marshal(tokenPayload{
		Tool:        tool,
		ParamsHash:  paramsHash,
		Fingerprint: fingerprint,
		Expires:     exp.Unix(),
	})
	if err != nil {
		return "", time.Time{}, err
	}
	mac := hmac.New(sha256.New, c.secret)
	mac.Write(body)
	return base64.RawURLEncoding.EncodeToString(body) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), exp, nil
}

func (c *Controller) verifyToken(token, tool, paramsHash, fingerprint string) error {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return &DenyError{
			Code:    DenyTokenInvalid,
			Reason:  "plan_token 格式非法",
			Suggest: "先用 dry_run=true 调用一次以获取新的 plan_token",
		}
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return &DenyError{
			Code:    DenyTokenInvalid,
			Reason:  "plan_token 载荷无法解码",
			Suggest: "重新以 dry_run=true 获取 token",
		}
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return &DenyError{
			Code:    DenyTokenInvalid,
			Reason:  "plan_token 签名无法解码",
			Suggest: "重新以 dry_run=true 获取 token",
		}
	}
	mac := hmac.New(sha256.New, c.secret)
	mac.Write(body)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return &DenyError{
			Code:    DenyTokenInvalid,
			Reason:  "plan_token 签名校验失败（可能被篡改或来自另一次 mcp-x 进程）",
			Suggest: "重新以 dry_run=true 获取 token",
		}
	}
	var p tokenPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return &DenyError{
			Code:    DenyTokenInvalid,
			Reason:  "plan_token 内容无法解析",
			Suggest: "重新以 dry_run=true 获取 token",
		}
	}
	if p.Tool != tool {
		return &DenyError{
			Code:    DenyTokenInvalid,
			Reason:  fmt.Sprintf("plan_token 属于工具 %q，不能用于 %q", p.Tool, tool),
			Suggest: "为该工具单独做一次 dry_run",
		}
	}
	if p.ParamsHash != paramsHash {
		return &DenyError{
			Code:    DenyTokenInvalid,
			Reason:  "plan_token 与本次参数不匹配（token 已与具体参数绑定，不可挪用）",
			Suggest: "参数与预演时保持完全一致，或重新 dry_run",
		}
	}
	if p.Fingerprint != fingerprint {
		return &DenyError{
			Code:    DenyTokenScreenStale,
			Reason:  "plan_token 绑定的屏幕指纹已失效（分辨率或显示器布局已变化）",
			Suggest: "重新调用 desktop_screen_info 并再次 dry_run",
		}
	}
	if nowFunc().Unix() > p.Expires {
		return &DenyError{
			Code:    DenyTokenExpired,
			Reason:  fmt.Sprintf("plan_token 已于 %s 过期", time.Unix(p.Expires, 0).Format(time.RFC3339)),
			Suggest: "重新以 dry_run=true 获取 token",
		}
	}
	return nil
}

func (c *Controller) confirmTTL() time.Duration { return c.cfg.ConfirmTTL }

// requiresConfirm reports whether the two-phase handshake is in force for this
// call. A dry run is always allowed; a real run in confirm mode must carry a
// token, which the caller then verifies against the exact parameters.
func (c *Controller) requiresConfirm(tool string, dryRun bool, token string) (bool, error) {
	need := c.cfg.ConfirmRequired
	if tool == ToolTypeText || tool == ToolKey {
		need = need || c.cfg.Capabilities.KeyboardRequireConfirm
	}
	if !need || dryRun {
		return false, nil
	}
	if token == "" {
		return true, &DenyError{
			Code:    DenyConfirmRequired,
			Reason:  "当前配置要求两阶段确认，但未提供 plan_token",
			Suggest: fmt.Sprintf("先以 dry_run=true 调用 %s，拿到 plan_token 后原样回传", tool),
		}
	}
	return true, nil
}

func shortFP(s string) string {
	if len(s) > 32 {
		return s[:32] + "..."
	}
	return s
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
