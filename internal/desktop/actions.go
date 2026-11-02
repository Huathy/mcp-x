package desktop

import (
	"fmt"
	"time"
)

// MCP tool names, also used as the token scope so a token issued for one tool
// cannot authorize another.
const (
	ToolScreenInfo  = "desktop_screen_info"
	ToolListWindows = "desktop_list_windows"
	ToolMouseMove   = "desktop_mouse_move"
	ToolMouseClick  = "desktop_mouse_click"
	ToolMouseDrag   = "desktop_mouse_drag"
	ToolMouseScroll = "desktop_mouse_scroll"
	ToolTypeText    = "desktop_type_text"
	ToolKey         = "desktop_key"
	ToolScreenshot  = "desktop_screenshot"
)

// ClickRequest describes a click at a validated target.
type ClickRequest struct {
	Target      Target
	Button      string
	Count       int
	ScreenToken string
	DryRun      bool
	Token       string
}

// DragRequest describes a drag between two points of the same window.
type DragRequest struct {
	Target      Target
	FromX       int
	FromY       int
	ToX         int
	ToY         int
	Button      string
	ScreenToken string
	DryRun      bool
	Token       string
}

// ScrollRequest describes a wheel action at a validated target.
type ScrollRequest struct {
	Target      Target
	Direction   string
	Notches     int
	ScreenToken string
	DryRun      bool
	Token       string
}

// TextRequest describes Unicode text entry at a validated target.
type TextRequest struct {
	Target      Target
	Text        string
	ScreenToken string
	DryRun      bool
	Token       string
}

// KeyRequest describes a single named key press at a validated target.
type KeyRequest struct {
	Target      Target
	Key         string
	Mods        Mods
	ScreenToken string
	DryRun      bool
	Token       string
}

// ScreenshotRequest captures either the whole screen or a validated region.
type ScreenshotRequest struct {
	Scope       string
	Target      Target
	HasTarget   bool
	ScreenToken string
}

// Result reports what a mutating call did, for the MCP success text.
type Result struct {
	Plan     Plan
	Executed bool
	Detail   string
}

// withValidation is the shared skeleton of every mutating tool: preflight,
// capability gate, target resolution, confirm handshake, then execution under
// the controller lock. The backend is never touched before every check passes.
func (c *Controller) withValidation(
	tool string,
	screenToken string,
	dryRun bool,
	token string,
	paramsHashParts []any,
	capability func() error,
	targetOf func() Target,
	run func(win WindowInfo, p Point) (string, error),
) (Result, error) {
	started := nowFunc()
	deny := func(err error) (Result, error) {
		c.audit(tool, "", "", "", 0, 0, "", "deny", codeOf(err), started, err)
		return Result{}, err
	}

	screen, err := c.backend.ScreenInfo()
	if err != nil {
		return deny(&DenyError{
			Code:    DenyParam,
			Reason:  fmt.Sprintf("读取屏幕信息失败：%v", err),
			Suggest: "确认 mcp-x 运行在交互式桌面会话中（Session 1），服务进程（Session 0）无法访问桌面",
		})
	}
	if len(screen.Monitors) == 0 {
		return deny(&DenyError{
			Code:    DenyNoInteractiveDesk,
			Reason:  "未检测到任何显示器，mcp-x 可能未运行在交互式桌面会话中",
			Suggest: "在用户登录的桌面会话中启动 mcp-x",
		})
	}

	if err := c.preflight(tool, screen, screenToken); err != nil {
		return deny(err)
	}
	if err := capability(); err != nil {
		return deny(err)
	}

	needsConfirm, err := c.requiresConfirm(tool, dryRun, token)
	if err != nil {
		return deny(err)
	}

	wins, err := c.backend.ListWindows()
	if err != nil {
		return deny(&DenyError{
			Code:    DenyParam,
			Reason:  fmt.Sprintf("枚举窗口失败：%v", err),
			Suggest: "重试一次；若持续失败请检查是否有权限限制",
		})
	}

	target := targetOf()
	win, point, err := c.resolve(screen, wins, target)
	if err != nil {
		return deny(err)
	}

	fp := screen.Fingerprint()
	plan := Plan{
		Window:      win,
		Target:      target,
		Point:       point,
		Screen:      screen,
		Fingerprint: fp,
		Preview:     dryRun,
	}

	paramsHash := hashParams(tool, paramsHashParts...)
	if needsConfirm {
		if err := c.verifyToken(token, tool, paramsHash, fp); err != nil {
			return deny(err)
		}
	}

	if dryRun {
		plan.Token, plan.TokenExpires, err = c.issueToken(tool, paramsHash, fp)
		if err != nil {
			return deny(&DenyError{
				Code:    DenyParam,
				Reason:  fmt.Sprintf("生成 plan_token 失败：%v", err),
				Suggest: "重试一次",
			})
		}
		c.audit(tool, win.Title, targetCoords(target), win.Rect.String(),
			point.X, point.Y, fp, "preview", "", started, nil)
		return Result{Plan: plan, Executed: false, Detail: "预演通过，未执行任何操作"}, nil
	}

	// Only now, with every check passed, may the cursor move.
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.allowRate(); err != nil {
		return deny(err)
	}
	detail, err := run(win, point)
	if err != nil {
		c.audit(tool, win.Title, targetCoords(target), win.Rect.String(),
			point.X, point.Y, fp, "error", "", started, err)
		return Result{}, err
	}
	plan.Preview = false
	c.audit(tool, win.Title, targetCoords(target), win.Rect.String(),
		point.X, point.Y, fp, "allow", "", started, nil)
	return Result{Plan: plan, Executed: true, Detail: detail}, nil
}

func targetCoords(t Target) string {
	return fmt.Sprintf("win(%d,%d)+off(%d,%d)", t.WindowX, t.WindowY, t.MouseX, t.MouseY)
}

func codeOf(err error) DenyCode {
	var de *DenyError
	if asDenyError(err, &de) {
		return de.Code
	}
	return DenyParam
}

func (c *Controller) audit(tool, windowTitle, passed, real string, absX, absY int,
	fingerprint, decision string, code DenyCode, started time.Time, err error) {
	if c.log == nil {
		return
	}
	c.log.Audit(tool, windowTitle, passed, real, absX, absY, fingerprint,
		decision, code, nowFunc().Sub(started).Milliseconds(), err)
}

// Move moves the cursor to the validated point without pressing any button.
func (c *Controller) Move(req MoveRequest) (Result, error) {
	if !c.cfg.Capabilities.MouseEnabled {
		return Result{}, mouseDisabled()
	}
	return c.withValidation(
		ToolMouseMove, req.ScreenToken, req.DryRun, req.Token,
		[]any{req.Target},
		func() error { return nil },
		func() Target { return req.Target },
		func(_ WindowInfo, p Point) (string, error) {
			if err := c.backend.MoveTo(p.X, p.Y); err != nil {
				return "", fmt.Errorf("move mouse: %w", err)
			}
			return fmt.Sprintf("鼠标已移动到 (%d,%d)", p.X, p.Y), nil
		},
	)
}

// Click presses and releases a mouse button at the validated point.
func (c *Controller) Click(req ClickRequest) (Result, error) {
	if !c.cfg.Capabilities.MouseEnabled {
		return Result{}, mouseDisabled()
	}
	button := req.Button
	if button == "" {
		button = "left"
	}
	count := req.Count
	if count <= 0 {
		count = 1
	}
	if count > 2 {
		return Result{}, &DenyError{
			Code:    DenyParam,
			Reason:  fmt.Sprintf("count 只支持 1 或 2，收到 %d", count),
			Suggest: "需要三连击请拆成多次调用",
		}
	}
	if count == 2 && !c.cfg.Capabilities.MouseDoubleClick {
		return Result{}, &DenyError{
			Code: DenyDisabled, Reason: "desktop.mouse.allow_double_click = false",
			Suggest: "由用户开启 allow_double_click 后再双击",
		}
	}
	if !buttonAllowed(c.cfg.Capabilities.MouseButtons, button) {
		return Result{}, &DenyError{
			Code: DenyButtonNotAllowed,
			Reason: fmt.Sprintf("鼠标键 %q 不在允许列表 %v 中",
				button, c.cfg.Capabilities.MouseButtons),
			Suggest: "改用允许的按键，或由用户调整 desktop.mouse.allow_buttons",
		}
	}
	return c.withValidation(
		ToolMouseClick, req.ScreenToken, req.DryRun, req.Token,
		[]any{req.Target, button, count},
		func() error { return nil },
		func() Target { return req.Target },
		func(_ WindowInfo, p Point) (string, error) {
			if err := c.backend.MoveTo(p.X, p.Y); err != nil {
				return "", fmt.Errorf("move mouse: %w", err)
			}
			// A short settle delay lets the window under the cursor register
			// the hover before the button goes down.
			time.Sleep(hoverSettle)
			for i := 0; i < count; i++ {
				if i > 0 {
					time.Sleep(doubleClickGap)
				}
				if err := c.backend.MouseDown(button); err != nil {
					return "", fmt.Errorf("mouse down: %w", err)
				}
				time.Sleep(buttonHold)
				if err := c.backend.MouseUp(button); err != nil {
					return "", fmt.Errorf("mouse up: %w", err)
				}
			}
			return fmt.Sprintf("已在 (%d,%d) 执行 %d 次 %s 键点击", p.X, p.Y, count, button), nil
		},
	)
}

// Drag presses at one point inside the window, moves through interpolated
// steps, and releases at the destination.
func (c *Controller) Drag(req DragRequest) (Result, error) {
	if !c.cfg.Capabilities.MouseEnabled {
		return Result{}, mouseDisabled()
	}
	if !c.cfg.Capabilities.MouseDrag {
		return Result{}, &DenyError{
			Code: DenyDisabled, Reason: "desktop.mouse.allow_drag = false",
			Suggest: "由用户开启 allow_drag 后再拖拽",
		}
	}
	button := req.Button
	if button == "" {
		button = "left"
	}
	if !buttonAllowed(c.cfg.Capabilities.MouseButtons, button) {
		return Result{}, &DenyError{
			Code: DenyButtonNotAllowed,
			Reason: fmt.Sprintf("鼠标键 %q 不在允许列表 %v 中",
				button, c.cfg.Capabilities.MouseButtons),
			Suggest: "改用允许的按键，或由用户调整 desktop.mouse.allow_buttons",
		}
	}
	if req.FromX < 0 || req.FromY < 0 || req.ToX < 0 || req.ToY < 0 {
		return Result{}, &DenyError{
			Code:    DenyParam,
			Reason:  "from_x/from_y/to_x/to_y 不能为负数",
			Suggest: "相对坐标必须是窗口内的非负偏移",
		}
	}
	if req.FromX == req.ToX && req.FromY == req.ToY {
		return Result{}, &DenyError{
			Code:    DenyParam,
			Reason:  "起点与终点相同，无需拖拽",
			Suggest: "检查 from_x/from_y 与 to_x/to_y 是否都填对",
		}
	}
	return c.withValidation(
		ToolMouseDrag, req.ScreenToken, req.DryRun, req.Token,
		[]any{req.Target, req.FromX, req.FromY, req.ToX, req.ToY, button},
		func() error { return nil },
		func() Target {
			t := req.Target
			t.MouseX, t.MouseY = req.FromX, req.FromY
			return t
		},
		func(win WindowInfo, from Point) (string, error) {
			toX, toY := req.Target.WindowX+req.ToX, req.Target.WindowY+req.ToY
			// The destination goes through the same bounds and hit-test
			// checks as the origin, so a drag can never leave the window.
			absTarget := req.Target
			absTarget.MouseX, absTarget.MouseY = req.ToX, req.ToY
			if _, err := c.checkInsideWindow(win, absTarget); err != nil {
				return "", err
			}
			if err := c.backend.MoveTo(from.X, from.Y); err != nil {
				return "", fmt.Errorf("move to drag start: %w", err)
			}
			time.Sleep(hoverSettle)
			if err := c.backend.MouseDown(button); err != nil {
				return "", fmt.Errorf("mouse down: %w", err)
			}
			steps := c.cfg.Capabilities.DragSteps
			if steps < 2 {
				steps = 2
			}
			perStep := time.Duration(c.cfg.Capabilities.DragDurationMs) * time.Millisecond / time.Duration(steps)
			if perStep <= 0 {
				perStep = time.Millisecond
			}
			for i := 1; i <= steps; i++ {
				x := from.X + (toX-from.X)*i/steps
				y := from.Y + (toY-from.Y)*i/steps
				if err := c.backend.MoveTo(x, y); err != nil {
					// Release on failure, otherwise the button stays down and
					// the next click anywhere would extend a selection.
					_ = c.backend.MouseUp(button)
					return "", fmt.Errorf("drag step %d: %w", i, err)
				}
				time.Sleep(perStep)
			}
			if err := c.backend.MouseUp(button); err != nil {
				return "", fmt.Errorf("mouse up: %w", err)
			}
			return fmt.Sprintf("已从 (%d,%d) 拖拽到 (%d,%d)，%d 步", from.X, from.Y, toX, toY, steps), nil
		},
	)
}

// checkInsideWindow re-runs the in-window and hit-test checks for a secondary
// point of the same operation.
func (c *Controller) checkInsideWindow(win WindowInfo, t Target) (Point, error) {
	m := c.cfg.EdgeMargin
	if t.MouseX < m || t.MouseX >= win.Rect.Width-m ||
		t.MouseY < m || t.MouseY >= win.Rect.Height-m {
		return Point{}, &DenyError{
			Code: DenyOutOfWindow,
			Reason: fmt.Sprintf("相对坐标 (%d,%d) 超出窗口 %s 的可操作范围（边距 %dpx）",
				t.MouseX, t.MouseY, win.Rect.String(), m),
			Suggest: "拖拽起点与终点都必须落在同一目标窗口内",
			Target:  &t,
			Real:    win.Rect.String(),
		}
	}
	absX, absY := t.WindowX+t.MouseX, t.WindowY+t.MouseY
	hit, ok, err := c.backend.WindowAtPoint(absX, absY)
	if err != nil {
		return Point{}, &DenyError{
			Code:    DenyParam,
			Reason:  fmt.Sprintf("命中测试失败：%v", err),
			Suggest: "重试一次",
			Target:  &t,
		}
	}
	if !ok || hit.Handle != win.Handle {
		got := "(无窗口)"
		if ok {
			got = fmt.Sprintf("%q", hit.Title)
		}
		return Point{}, &DenyError{
			Code: DenyOccluded,
			Reason: fmt.Sprintf("绝对坐标 (%d,%d) 实际命中 %s，而非目标窗口 %q",
				absX, absY, got, win.Title),
			Suggest: "拖拽起点与终点都必须落在同一目标窗口内",
			Target:  &t,
			Point:   &Point{X: absX, Y: absY},
		}
	}
	return Point{X: absX, Y: absY}, nil
}

// Scroll turns the wheel at the validated point.
func (c *Controller) Scroll(req ScrollRequest) (Result, error) {
	if !c.cfg.Capabilities.MouseEnabled {
		return Result{}, mouseDisabled()
	}
	dir := req.Direction
	if dir == "" {
		dir = "down"
	}
	notches := req.Notches
	if notches == 0 {
		notches = 3
	}
	if notches < 0 {
		notches = -notches
	}
	if notches > 50 {
		return Result{}, &DenyError{
			Code:    DenyParam,
			Reason:  fmt.Sprintf("notches 超过上限 50，收到 %d", notches),
			Suggest: "拆成多次调用",
		}
	}
	// Win32 convention: positive delta scrolls up / right, negative down / left.
	var (
		delta      int
		horizontal bool
	)
	switch dir {
	case "up":
		delta = notches
	case "down":
		delta = -notches
	case "right":
		delta, horizontal = notches, true
	case "left":
		delta, horizontal = -notches, true
	default:
		return Result{}, &DenyError{
			Code:    DenyParam,
			Reason:  fmt.Sprintf("direction 只能是 up/down/left/right，收到 %q", dir),
			Suggest: "改用标准方向值",
		}
	}
	return c.withValidation(
		ToolMouseScroll, req.ScreenToken, req.DryRun, req.Token,
		[]any{req.Target, dir, notches},
		func() error { return nil },
		func() Target { return req.Target },
		func(_ WindowInfo, p Point) (string, error) {
			if err := c.backend.MoveTo(p.X, p.Y); err != nil {
				return "", fmt.Errorf("move mouse: %w", err)
			}
			time.Sleep(hoverSettle)
			if err := c.backend.MouseWheel(horizontal, delta); err != nil {
				return "", fmt.Errorf("scroll: %w", err)
			}
			return fmt.Sprintf("已在 (%d,%d) 向 %s 滚动 %d 格", p.X, p.Y, dir, notches), nil
		},
	)
}

// TypeText injects Unicode text at the validated point.
func (c *Controller) TypeText(req TextRequest) (Result, error) {
	if !c.cfg.Capabilities.KeyboardEnabled {
		return Result{}, keyboardDisabled()
	}
	if max := c.cfg.Capabilities.MaxTextLength; max > 0 && len([]rune(req.Text)) > max {
		return Result{}, &DenyError{
			Code:    DenyTextTooLong,
			Reason:  fmt.Sprintf("文本长度 %d 超过上限 %d", len([]rune(req.Text)), max),
			Suggest: "拆成多次调用",
		}
	}
	return c.withValidation(
		ToolTypeText, req.ScreenToken, req.DryRun, req.Token,
		[]any{req.Target, req.Text},
		func() error { return nil },
		func() Target { return req.Target },
		func(_ WindowInfo, p Point) (string, error) {
			if err := c.backend.MoveTo(p.X, p.Y); err != nil {
				return "", fmt.Errorf("move mouse: %w", err)
			}
			time.Sleep(hoverSettle)
			if err := c.backend.TypeText(req.Text); err != nil {
				return "", fmt.Errorf("type text: %w", err)
			}
			return fmt.Sprintf("已在 (%d,%d) 输入 %d 个字符", p.X, p.Y, len([]rune(req.Text))), nil
		},
	)
}

// PressKey injects one whitelisted key, optionally with modifiers.
func (c *Controller) PressKey(req KeyRequest) (Result, error) {
	if !c.cfg.Capabilities.KeyboardEnabled {
		return Result{}, keyboardDisabled()
	}
	if !AllowedKey(req.Key) {
		return Result{}, &DenyError{
			Code:    DenyParam,
			Reason:  fmt.Sprintf("按键 %q 不在白名单内", req.Key),
			Suggest: "可用按键: " + joinKeys(AllowedKeys()),
		}
	}
	return c.withValidation(
		ToolKey, req.ScreenToken, req.DryRun, req.Token,
		[]any{req.Target, req.Key, req.Mods},
		func() error { return nil },
		func() Target { return req.Target },
		func(_ WindowInfo, p Point) (string, error) {
			if err := c.backend.MoveTo(p.X, p.Y); err != nil {
				return "", fmt.Errorf("move mouse: %w", err)
			}
			time.Sleep(hoverSettle)
			if err := c.backend.PressKey(req.Key, req.Mods); err != nil {
				return "", fmt.Errorf("press key: %w", err)
			}
			return fmt.Sprintf("已在 (%d,%d) 按下 %s", p.X, p.Y, describeMods(req.Key, req.Mods)), nil
		},
	)
}

func mouseDisabled() error {
	return &DenyError{
		Code:    DenyDisabled,
		Reason:  "desktop.mouse.enabled = false",
		Suggest: "由用户开启 desktop.mouse.enabled",
	}
}

func keyboardDisabled() error {
	return &DenyError{
		Code:    DenyDisabled,
		Reason:  "desktop.keyboard.enabled = false（键盘输入需人工审核后由用户开启）",
		Suggest: "请用户在配置中显式开启 desktop.keyboard.enabled",
	}
}

func describeMods(key string, m Mods) string {
	out := key
	if m.Ctrl {
		out = "Ctrl+" + out
	}
	if m.Alt {
		out = "Alt+" + out
	}
	if m.Shift {
		out = "Shift+" + out
	}
	return out
}

func buttonAllowed(allowed []string, button string) bool {
	for _, a := range allowed {
		if a == button {
			return true
		}
	}
	return false
}

// Timing for synthetic input. Windows treats an instantaneous press/release as
// an unreliable click on some UI frameworks, and a drag with too few
// intermediate moves is treated as a jump rather than a drag.
//
// These are variables, not constants, so tests can shrink them; production
// code never reassigns them.
var (
	hoverSettle    = 30 * time.Millisecond
	buttonHold     = 40 * time.Millisecond
	doubleClickGap = 60 * time.Millisecond
)
