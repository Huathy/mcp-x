package desktop

import (
	"errors"
	"image"
	"image/color"
	"os"
	"sync"
	"testing"
	"time"
)

// TestMain shrinks the synthetic-input delays. They exist to make Windows
// accept the events, which matters for the real backend but only slows the
// validation-chain tests down.
func TestMain(m *testing.M) {
	hoverSettle = time.Millisecond
	buttonHold = time.Millisecond
	doubleClickGap = time.Millisecond
	os.Exit(m.Run())
}

// fakeBackend is a scriptable Backend. Every mutating method records the call so
// a test can assert that a rejected request never reached the platform.
type fakeBackend struct {
	mu sync.Mutex

	screen  ScreenInfo
	windows []WindowInfo

	// overlay maps a screen point to a different handle, to simulate another
	// window stacked on top. A point absent from the map resolves to the
	// window whose rect contains it, which keeps the fixture small.
	overlay map[[2]int]uintptr
	// blank lists points with no window at all.
	blank map[[2]int]bool

	moves  []Point
	downs  []string
	ups    []string
	wheels []int
	texts  []string
	keys   []string
	shots  []Rect
	forced []uintptr

	calls []string
	fail  error
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{
		screen: ScreenInfo{
			PrimaryWidth:  1920,
			PrimaryHeight: 1080,
			Virtual:       Rect{X: 0, Y: 0, Width: 1920, Height: 1080},
			Dpi:           96,
			ScalePct:      100,
			Monitors: []Monitor{
				{Index: 0, Primary: true, Device: `\\.\DISPLAY1`, Rect: Rect{0, 0, 1920, 1080}, Dpi: 96, ScalePct: 100},
			},
			Foreground: "notepad - test",
		},
		overlay: map[[2]int]uintptr{},
		blank:   map[[2]int]bool{},
	}
}

func (f *fakeBackend) record(name string) error {
	f.calls = append(f.calls, name)
	return f.fail
}

func (f *fakeBackend) ScreenInfo() (ScreenInfo, error) {
	if err := f.record("ScreenInfo"); err != nil {
		return ScreenInfo{}, err
	}
	return f.screen, nil
}

func (f *fakeBackend) ListWindows() ([]WindowInfo, error) {
	if err := f.record("ListWindows"); err != nil {
		return nil, err
	}
	return append([]WindowInfo(nil), f.windows...), nil
}

func (f *fakeBackend) WindowAtPoint(x, y int) (WindowInfo, bool, error) {
	if err := f.record("WindowAtPoint"); err != nil {
		return WindowInfo{}, false, err
	}
	pt := [2]int{x, y}
	if f.blank[pt] {
		return WindowInfo{}, false, nil
	}
	if h, ok := f.overlay[pt]; ok {
		for _, w := range f.windows {
			if w.Handle == h {
				return w, true, nil
			}
		}
		return WindowInfo{Handle: h, Title: "other window"}, true, nil
	}
	// Topmost last: a later window in the list wins, matching z-order.
	for i := len(f.windows) - 1; i >= 0; i-- {
		if f.windows[i].Rect.Contains(x, y) {
			return f.windows[i], true, nil
		}
	}
	return WindowInfo{}, false, nil
}

func (f *fakeBackend) SetForeground(hwnd uintptr) error {
	if err := f.record("SetForeground"); err != nil {
		return err
	}
	f.forced = append(f.forced, hwnd)
	return nil
}

func (f *fakeBackend) CursorPos() (int, int, error) { return 0, 0, nil }
func (f *fakeBackend) MoveTo(x, y int) error {
	if err := f.record("MoveTo"); err != nil {
		return err
	}
	f.moves = append(f.moves, Point{x, y})
	return nil
}

func (f *fakeBackend) MouseDown(button string) error {
	if err := f.record("MouseDown"); err != nil {
		return err
	}
	f.downs = append(f.downs, button)
	return nil
}

func (f *fakeBackend) MouseUp(button string) error {
	if err := f.record("MouseUp"); err != nil {
		return err
	}
	f.ups = append(f.ups, button)
	return nil
}

func (f *fakeBackend) MouseWheel(horizontal bool, notches int) error {
	if err := f.record("MouseWheel"); err != nil {
		return err
	}
	f.wheels = append(f.wheels, notches)
	return nil
}

func (f *fakeBackend) TypeText(text string) error {
	if err := f.record("TypeText"); err != nil {
		return err
	}
	f.texts = append(f.texts, text)
	return nil
}

func (f *fakeBackend) PressKey(key string, mods Mods) error {
	if err := f.record("PressKey"); err != nil {
		return err
	}
	f.keys = append(f.keys, key)
	return nil
}

func (f *fakeBackend) Screenshot(rect Rect) (image.Image, error) {
	if err := f.record("Screenshot"); err != nil {
		return nil, err
	}
	f.shots = append(f.shots, rect)
	img := image.NewRGBA(image.Rect(0, 0, rect.Width, rect.Height))
	for y := 0; y < rect.Height; y++ {
		for x := 0; x < rect.Width; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 0, 255})
		}
	}
	return img, nil
}

// mutatingCalls returns only the calls that can move the cursor or send input.
func (f *fakeBackend) mutatingCalls() []string {
	mutating := map[string]bool{
		"MoveTo": true, "MouseDown": true, "MouseUp": true,
		"MouseWheel": true, "TypeText": true, "PressKey": true,
	}
	var out []string
	for _, c := range f.calls {
		if mutating[c] {
			out = append(out, c)
		}
	}
	return out
}

func baseConfig() Config {
	return Config{
		Enabled:             true,
		MaxActionsPerMinute: 60,
		PositionTolerancePx: 2,
		RequireForeground:   true,
		EdgeMargin:          1,
		ConfirmRequired:     false,
		ConfirmTTL:          time.Minute,
		BlockedRegions:      nil,
		Capabilities: Capabilities{
			MouseEnabled:      true,
			MouseButtons:      []string{"left", "right", "middle"},
			MouseDoubleClick:  true,
			MouseDrag:         true,
			DragSteps:         4,
			DragDurationMs:    8,
			MaxTextLength:     100,
			ScreenshotEnabled: true,
			AllowFullScreen:   true,
		},
	}
}

// setup wires a controller onto a fake with one visible foreground window at
// (100,200) sized 800x600. The fake resolves hit tests from the window rects,
// so no per-point fixture is needed.
func setup(t *testing.T, tweak func(*Config)) (*Controller, *fakeBackend) {
	t.Helper()
	f := newFakeBackend()
	f.windows = []WindowInfo{{
		Handle:     0x1000,
		Title:      "notepad - test",
		Class:      "Notepad",
		Rect:       Rect{X: 100, Y: 200, Width: 800, Height: 600},
		Visible:    true,
		Foreground: true,
		PID:        4242,
		Process:    "notepad.exe",
	}}
	f.screen.Foreground = "notepad - test"
	cfg := baseConfig()
	if tweak != nil {
		tweak(&cfg)
	}
	c, err := NewController(cfg, f, nil)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	return c, f
}

func goodTarget() Target {
	return Target{WindowTitle: "notepad - test", WindowX: 100, WindowY: 200, MouseX: 40, MouseY: 60}
}

func wantDeny(t *testing.T, err error, code DenyCode) {
	t.Helper()
	var de *DenyError
	if !errors.As(err, &de) {
		t.Fatalf("expected DenyError, got %T: %v", err, err)
	}
	if de.Code != code {
		t.Fatalf("expected code %s, got %s (%v)", code, de.Code, err)
	}
}

func TestClickAllowed(t *testing.T) {
	c, f := setup(t, nil)
	res, err := c.Click(ClickRequest{Target: goodTarget()})
	if err != nil {
		t.Fatalf("click: %v", err)
	}
	if !res.Executed {
		t.Fatal("expected execution")
	}
	if len(f.moves) != 1 || f.moves[0] != (Point{140, 260}) {
		t.Fatalf("expected move to (140,260), got %v", f.moves)
	}
	if len(f.downs) != 1 || f.downs[0] != "left" {
		t.Fatalf("expected one left press, got %v", f.downs)
	}
	if res.Plan.Point != (Point{140, 260}) {
		t.Fatalf("plan point mismatch: %v", res.Plan.Point)
	}
}

func TestDenyParamEmptyTitle(t *testing.T) {
	c, f := setup(t, nil)
	tgt := goodTarget()
	tgt.WindowTitle = "  "
	_, err := c.Click(ClickRequest{Target: tgt})
	wantDeny(t, err, DenyParam)
	assertNoMutation(t, f)
}

func TestDenyParamNegativeOffset(t *testing.T) {
	c, f := setup(t, nil)
	tgt := goodTarget()
	tgt.MouseX = -1
	_, err := c.Click(ClickRequest{Target: tgt})
	wantDeny(t, err, DenyParam)
	assertNoMutation(t, f)
}

func TestDenyParamBadCount(t *testing.T) {
	c, f := setup(t, nil)
	_, err := c.Click(ClickRequest{Target: goodTarget(), Count: 5})
	wantDeny(t, err, DenyParam)
	assertNoMutation(t, f)
}

func TestDenyButtonNotAllowed(t *testing.T) {
	c, f := setup(t, func(cfg *Config) { cfg.Capabilities.MouseButtons = []string{"left"} })
	_, err := c.Click(ClickRequest{Target: goodTarget(), Button: "right"})
	wantDeny(t, err, DenyButtonNotAllowed)
	assertNoMutation(t, f)
}

func TestDenyDoubleClickDisabled(t *testing.T) {
	c, f := setup(t, func(cfg *Config) { cfg.Capabilities.MouseDoubleClick = false })
	_, err := c.Click(ClickRequest{Target: goodTarget(), Count: 2})
	wantDeny(t, err, DenyDisabled)
	assertNoMutation(t, f)
}

func TestDoubleClickSendsTwoPresses(t *testing.T) {
	c, f := setup(t, nil)
	if _, err := c.Click(ClickRequest{Target: goodTarget(), Count: 2}); err != nil {
		t.Fatalf("click: %v", err)
	}
	if len(f.downs) != 2 {
		t.Fatalf("expected 2 presses, got %d", len(f.downs))
	}
}

func TestDenyRateLimit(t *testing.T) {
	c, _ := setup(t, func(cfg *Config) { cfg.MaxActionsPerMinute = 2 })
	for i := 0; i < 2; i++ {
		if _, err := c.Click(ClickRequest{Target: goodTarget()}); err != nil {
			t.Fatalf("click %d: %v", i, err)
		}
	}
	_, err := c.Click(ClickRequest{Target: goodTarget()})
	wantDeny(t, err, DenyRateLimit)
}

func TestDryRunDoesNotConsumeRateLimit(t *testing.T) {
	c, f := setup(t, func(cfg *Config) { cfg.MaxActionsPerMinute = 1 })
	for i := 0; i < 3; i++ {
		if _, err := c.Click(ClickRequest{Target: goodTarget(), DryRun: true}); err != nil {
			t.Fatalf("preview %d: %v", i, err)
		}
	}
	if len(f.mutatingCalls()) != 0 {
		t.Fatalf("dry_run must not mutate, got %v", f.mutatingCalls())
	}
}

func TestDenyBlockedRegion(t *testing.T) {
	c, f := setup(t, func(cfg *Config) {
		cfg.BlockedRegions = []Region{
			{X: 0, Y: 1000, Width: 1920, Height: 80, Reason: "taskbar"},
		}
	})
	// (150, 1040) is inside the taskbar region.
	f.windows[0].Rect = Rect{X: 100, Y: 1000, Width: 800, Height: 70}
	tgt := Target{WindowTitle: "notepad - test", WindowX: 100, WindowY: 1000, MouseX: 50, MouseY: 40}
	_, err := c.Click(ClickRequest{Target: tgt})
	wantDeny(t, err, DenyBlockedRegion)
	assertNoMutation(t, f)
}

func TestDenyWindowNotFound(t *testing.T) {
	c, f := setup(t, nil)
	tgt := goodTarget()
	tgt.WindowTitle = "does not exist"
	_, err := c.Click(ClickRequest{Target: tgt})
	wantDeny(t, err, DenyWindowNotFound)
	assertNoMutation(t, f)
}

func TestDenyWindowAmbiguous(t *testing.T) {
	c, f := setup(t, nil)
	f.windows = append(f.windows, WindowInfo{
		Handle: 0x2000, Title: "notepad - test", Visible: true, Foreground: true,
		Rect: Rect{X: 0, Y: 0, Width: 400, Height: 300},
	})
	_, err := c.Click(ClickRequest{Target: goodTarget()})
	wantDeny(t, err, DenyWindowAmbiguous)
	assertNoMutation(t, f)
}

func TestDenyWindowMinimized(t *testing.T) {
	c, f := setup(t, nil)
	f.windows[0].Minimized = true
	_, err := c.Click(ClickRequest{Target: goodTarget()})
	wantDeny(t, err, DenyWindowMinimized)
	assertNoMutation(t, f)
}

func TestDenyWindowInvisible(t *testing.T) {
	c, f := setup(t, nil)
	f.windows[0].Visible = false
	_, err := c.Click(ClickRequest{Target: goodTarget()})
	wantDeny(t, err, DenyWindowInvisible)
	assertNoMutation(t, f)
}

func TestDenyWindowMoved(t *testing.T) {
	c, f := setup(t, nil)
	// The AI observed (100,200) but the window has since moved.
	f.windows[0].Rect = Rect{X: 103, Y: 200, Width: 800, Height: 600}
	_, err := c.Click(ClickRequest{Target: goodTarget()})
	wantDeny(t, err, DenyWindowMoved)
	assertNoMutation(t, f)
}

func TestAllowWithinTolerance(t *testing.T) {
	c, f := setup(t, nil)
	f.windows[0].Rect = Rect{X: 101, Y: 199, Width: 800, Height: 600}
	if _, err := c.Click(ClickRequest{Target: goodTarget()}); err != nil {
		t.Fatalf("1px drift must pass with tolerance 2: %v", err)
	}
}

func TestDenyOutOfWindow(t *testing.T) {
	c, f := setup(t, nil)
	tgt := goodTarget()
	tgt.MouseX = 800 // == width, outside
	_, err := c.Click(ClickRequest{Target: tgt})
	wantDeny(t, err, DenyOutOfWindow)
	assertNoMutation(t, f)
}

func TestDenyEdgeMargin(t *testing.T) {
	c, f := setup(t, nil)
	tgt := goodTarget()
	tgt.MouseX = 0 // edge_margin is 1, so offset 0 is the resize border
	_, err := c.Click(ClickRequest{Target: tgt})
	wantDeny(t, err, DenyOutOfWindow)
	assertNoMutation(t, f)
}

func TestDenyOutOfScreenMultiMonitor(t *testing.T) {
	c, f := setup(t, nil)
	// A second monitor to the left makes the virtual origin negative, so the
	// check has to handle a rect that does not start at the origin.
	f.screen.Virtual = Rect{X: -1920, Y: 0, Width: 3840, Height: 1080}
	f.screen.Monitors = append(f.screen.Monitors, Monitor{
		Index: 1, Rect: Rect{X: -1920, Y: 0, Width: 1920, Height: 1080}, Dpi: 96, ScalePct: 100,
	})
	// window_x is far past the right edge of the 3840-wide virtual desktop.
	tgt := goodTarget()
	tgt.WindowX = 5000
	_, err := c.Click(ClickRequest{Target: tgt})
	wantDeny(t, err, DenyOutOfScreen)
	assertNoMutation(t, f)
}

func TestDenyOutOfScreenLeftOfVirtualOrigin(t *testing.T) {
	c, f := setup(t, nil)
	f.screen.Virtual = Rect{X: 100, Y: 0, Width: 1920, Height: 1080}
	tgt := goodTarget()
	tgt.WindowX = 0 // resolves to x=40, left of the virtual origin
	_, err := c.Click(ClickRequest{Target: tgt})
	wantDeny(t, err, DenyOutOfScreen)
	assertNoMutation(t, f)
}

func TestAllowOnNegativeCoordinateMonitor(t *testing.T) {
	c, f := setup(t, nil)
	f.screen.Virtual = Rect{X: -1920, Y: 0, Width: 3840, Height: 1080}
	f.screen.Monitors = append(f.screen.Monitors, Monitor{
		Index: 1, Rect: Rect{X: -1920, Y: 0, Width: 1920, Height: 1080}, Dpi: 96, ScalePct: 100,
	})
	f.windows[0].Rect = Rect{X: -1800, Y: 0, Width: 800, Height: 600}
	tgt := Target{WindowTitle: "notepad - test", WindowX: -1800, WindowY: 0, MouseX: 10, MouseY: 10}
	res, err := c.Click(ClickRequest{Target: tgt})
	if err != nil {
		t.Fatalf("negative-origin monitor must be supported: %v", err)
	}
	if res.Plan.Point != (Point{-1790, 10}) {
		t.Fatalf("unexpected point %v", res.Plan.Point)
	}
}

func TestDenyScreenTokenStale(t *testing.T) {
	c, f := setup(t, nil)
	_, err := c.Click(ClickRequest{Target: goodTarget(), ScreenToken: "stale-fingerprint"})
	wantDeny(t, err, DenyScreenChanged)
	assertNoMutation(t, f)
}

func TestAllowWithFreshScreenToken(t *testing.T) {
	c, f := setup(t, nil)
	screen, _ := f.ScreenInfo()
	_, err := c.Click(ClickRequest{Target: goodTarget(), ScreenToken: screen.Fingerprint()})
	if err != nil {
		t.Fatalf("fresh token must pass: %v", err)
	}
}

func TestDenyOccludedByOtherWindow(t *testing.T) {
	c, f := setup(t, nil)
	// A different window is stacked on top of the target point.
	f.overlay[[2]int{140, 260}] = 0x9999
	_, err := c.Click(ClickRequest{Target: goodTarget()})
	wantDeny(t, err, DenyOccluded)
	assertNoMutation(t, f)
}

func TestDenyNothingAtPoint(t *testing.T) {
	c, f := setup(t, nil)
	f.blank[[2]int{140, 260}] = true
	_, err := c.Click(ClickRequest{Target: goodTarget()})
	wantDeny(t, err, DenyOccluded)
	assertNoMutation(t, f)
}

func TestDenyNotForeground(t *testing.T) {
	c, f := setup(t, nil)
	f.windows[0].Foreground = false
	f.screen.Foreground = "chrome"
	_, err := c.Click(ClickRequest{Target: goodTarget()})
	wantDeny(t, err, DenyNotForeground)
	assertNoMutation(t, f)
}

func TestAllowBackgroundWindowWhenAllowed(t *testing.T) {
	c, f := setup(t, func(cfg *Config) { cfg.RequireForeground = false })
	f.windows[0].Foreground = false
	f.screen.Foreground = "chrome"
	if _, err := c.Click(ClickRequest{Target: goodTarget()}); err != nil {
		t.Fatalf("background window must be allowed when require_foreground is off: %v", err)
	}
}

func TestDenySafetyMode(t *testing.T) {
	CheckWriteFunc(func() error { return errors.New("data source is read-only, write operations blocked") })
	t.Cleanup(func() { CheckWriteFunc(nil) })
	c, f := setup(t, nil)
	_, err := c.Click(ClickRequest{Target: goodTarget()})
	wantDeny(t, err, DenySafetyMode)
	assertNoMutation(t, f)
}

func TestDenyModuleDisabled(t *testing.T) {
	c, f := setup(t, func(cfg *Config) { cfg.Enabled = false })
	_, err := c.Click(ClickRequest{Target: goodTarget()})
	wantDeny(t, err, DenyDisabled)
	assertNoMutation(t, f)
}

func TestDenyMouseDisabled(t *testing.T) {
	c, f := setup(t, func(cfg *Config) { cfg.Capabilities.MouseEnabled = false })
	_, err := c.Click(ClickRequest{Target: goodTarget()})
	wantDeny(t, err, DenyDisabled)
	assertNoMutation(t, f)
}

func TestDenyNoMonitors(t *testing.T) {
	c, f := setup(t, nil)
	f.screen.Monitors = nil
	_, err := c.Click(ClickRequest{Target: goodTarget()})
	wantDeny(t, err, DenyNoInteractiveDesk)
	assertNoMutation(t, f)
}

func TestDryRunReturnsTokenWithoutExecuting(t *testing.T) {
	c, f := setup(t, nil)
	res, err := c.Click(ClickRequest{Target: goodTarget(), DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if res.Executed {
		t.Fatal("dry_run must not execute")
	}
	if res.Plan.Token == "" {
		t.Fatal("dry_run should return a plan_token")
	}
	if len(f.mutatingCalls()) != 0 {
		t.Fatalf("dry_run must not mutate, got %v", f.mutatingCalls())
	}
	if res.Plan.Point != (Point{140, 260}) {
		t.Fatalf("dry_run should still report the absolute point, got %v", res.Plan.Point)
	}
}

func TestDenyConfirmRequiredWithoutToken(t *testing.T) {
	c, f := setup(t, func(cfg *Config) { cfg.ConfirmRequired = true })
	_, err := c.Click(ClickRequest{Target: goodTarget()})
	wantDeny(t, err, DenyConfirmRequired)
	assertNoMutation(t, f)
}

func TestConfirmTwoPhaseSucceeds(t *testing.T) {
	c, f := setup(t, func(cfg *Config) { cfg.ConfirmRequired = true })
	preview, err := c.Click(ClickRequest{Target: goodTarget(), DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if _, err := c.Click(ClickRequest{
		Target: goodTarget(), Token: preview.Plan.Token,
	}); err != nil {
		t.Fatalf("confirm phase: %v", err)
	}
	if len(f.moves) != 1 {
		t.Fatalf("expected the confirmed click to execute once, moves=%v", f.moves)
	}
}

func TestDenyTokenReusedForOtherParams(t *testing.T) {
	c, f := setup(t, func(cfg *Config) { cfg.ConfirmRequired = true })
	preview, err := c.Click(ClickRequest{Target: goodTarget(), DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	// Same tool, different coordinates: the token must not be transferable.
	other := goodTarget()
	other.MouseX = 41
	_, err = c.Click(ClickRequest{Target: other, Token: preview.Plan.Token})
	wantDeny(t, err, DenyTokenInvalid)
	assertNoMutation(t, f)
}

func TestDenyTokenReusedForOtherTool(t *testing.T) {
	c, f := setup(t, func(cfg *Config) { cfg.ConfirmRequired = true })
	preview, err := c.Click(ClickRequest{Target: goodTarget(), DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	_, err = c.Scroll(ScrollRequest{Target: goodTarget(), Token: preview.Plan.Token})
	wantDeny(t, err, DenyTokenInvalid)
	assertNoMutation(t, f)
}

func TestDenyTamperedToken(t *testing.T) {
	c, f := setup(t, func(cfg *Config) { cfg.ConfirmRequired = true })
	preview, _ := c.Click(ClickRequest{Target: goodTarget(), DryRun: true})
	tampered := preview.Plan.Token[:len(preview.Plan.Token)-4] + "AAAA"
	_, err := c.Click(ClickRequest{Target: goodTarget(), Token: tampered})
	wantDeny(t, err, DenyTokenInvalid)
	assertNoMutation(t, f)
}

func TestDenyTokenExpired(t *testing.T) {
	c, f := setup(t, func(cfg *Config) { cfg.ConfirmRequired = true })
	preview, err := c.Click(ClickRequest{Target: goodTarget(), DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	restore := nowFunc
	nowFunc = func() time.Time { return restore().Add(2 * time.Hour) }
	defer func() { nowFunc = restore }()
	_, err = c.Click(ClickRequest{Target: goodTarget(), Token: preview.Plan.Token})
	wantDeny(t, err, DenyTokenExpired)
	assertNoMutation(t, f)
}

func TestDenyTokenScreenStale(t *testing.T) {
	c, f := setup(t, func(cfg *Config) { cfg.ConfirmRequired = true })
	preview, err := c.Click(ClickRequest{Target: goodTarget(), DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	// A resolution change invalidates the token even though nothing else moved.
	f.screen.PrimaryWidth = 2560
	f.screen.PrimaryHeight = 1440
	f.screen.Virtual = Rect{X: 0, Y: 0, Width: 2560, Height: 1440}
	_, err = c.Click(ClickRequest{Target: goodTarget(), Token: preview.Plan.Token})
	wantDeny(t, err, DenyTokenScreenStale)
	assertNoMutation(t, f)
}

func TestDragMovesThroughInterpolatedSteps(t *testing.T) {
	c, f := setup(t, nil)
	_, err := c.Drag(DragRequest{
		Target: goodTarget(), FromX: 40, FromY: 60, ToX: 200, ToY: 160,
	})
	if err != nil {
		t.Fatalf("drag: %v", err)
	}
	// One move to the origin, then DragSteps moves to the destination.
	if len(f.moves) != 1+4 {
		t.Fatalf("expected 5 moves (1 + 4 steps), got %d: %v", len(f.moves), f.moves)
	}
	if f.moves[0] != (Point{140, 260}) {
		t.Fatalf("drag must start at the offset point, got %v", f.moves[0])
	}
	if f.moves[len(f.moves)-1] != (Point{300, 360}) {
		t.Fatalf("drag must end at the target point, got %v", f.moves[len(f.moves)-1])
	}
	if len(f.downs) != 1 || len(f.ups) != 1 {
		t.Fatalf("drag needs exactly one press and release, got %v / %v", f.downs, f.ups)
	}
}

func TestDenyDragDestinationOutsideWindow(t *testing.T) {
	c, f := setup(t, nil)
	_, err := c.Drag(DragRequest{
		Target: goodTarget(), FromX: 40, FromY: 60, ToX: 5000, ToY: 60,
	})
	wantDeny(t, err, DenyOutOfWindow)
	assertNoMutation(t, f)
}

func TestDenyDragDestinationOccluded(t *testing.T) {
	c, f := setup(t, nil)
	// The destination is inside the window bounds but another window is on
	// top of it, so the drag must not start.
	f.overlay[[2]int{300, 260}] = 0x8888
	_, err := c.Drag(DragRequest{
		Target: goodTarget(), FromX: 40, FromY: 60, ToX: 200, ToY: 60,
	})
	wantDeny(t, err, DenyOccluded)
	assertNoMutation(t, f)
}

func TestDenyDragSameEndpoints(t *testing.T) {
	c, f := setup(t, nil)
	_, err := c.Drag(DragRequest{Target: goodTarget(), FromX: 40, FromY: 60, ToX: 40, ToY: 60})
	wantDeny(t, err, DenyParam)
	assertNoMutation(t, f)
}

func TestDenyDragDisabled(t *testing.T) {
	c, f := setup(t, func(cfg *Config) { cfg.Capabilities.MouseDrag = false })
	_, err := c.Drag(DragRequest{Target: goodTarget(), FromX: 40, FromY: 60, ToX: 200, ToY: 160})
	wantDeny(t, err, DenyDisabled)
	assertNoMutation(t, f)
}

func TestScrollDirectionSign(t *testing.T) {
	c, f := setup(t, nil)
	if _, err := c.Scroll(ScrollRequest{Target: goodTarget(), Direction: "down", Notches: 3}); err != nil {
		t.Fatalf("scroll down: %v", err)
	}
	if len(f.wheels) != 1 || f.wheels[0] != -3 {
		t.Fatalf("scroll down should send -3, got %v", f.wheels)
	}
	f.wheels = nil
	if _, err := c.Scroll(ScrollRequest{Target: goodTarget(), Direction: "up", Notches: 2}); err != nil {
		t.Fatalf("scroll up: %v", err)
	}
	if len(f.wheels) != 1 || f.wheels[0] != 2 {
		t.Fatalf("scroll up should send 2, got %v", f.wheels)
	}
}

func TestDenyScrollBadDirection(t *testing.T) {
	c, f := setup(t, nil)
	_, err := c.Scroll(ScrollRequest{Target: goodTarget(), Direction: "sideways"})
	wantDeny(t, err, DenyParam)
	assertNoMutation(t, f)
}

func TestDenyScrollTooManyNotches(t *testing.T) {
	c, f := setup(t, nil)
	_, err := c.Scroll(ScrollRequest{Target: goodTarget(), Notches: 500})
	wantDeny(t, err, DenyParam)
	assertNoMutation(t, f)
}

func TestDenyKeyboardDisabledByDefault(t *testing.T) {
	c, f := setup(t, nil)
	_, err := c.TypeText(TextRequest{Target: goodTarget(), Text: "hi"})
	wantDeny(t, err, DenyDisabled)
	_, err = c.PressKey(KeyRequest{Target: goodTarget(), Key: "ENTER"})
	wantDeny(t, err, DenyDisabled)
	assertNoMutation(t, f)
}

func TestKeyboardRequiresTwoPhases(t *testing.T) {
	// The user enabled the keyboard but left RequireConfirm on; the direct
	// call must be refused so an accidental keystroke cannot reach the OS.
	c, f := setup(t, func(cfg *Config) {
		cfg.Capabilities.KeyboardEnabled = true
		cfg.Capabilities.KeyboardRequireConfirm = true
	})
	_, err := c.TypeText(TextRequest{Target: goodTarget(), Text: "hi"})
	wantDeny(t, err, DenyConfirmRequired)
	assertNoMutation(t, f)

	preview, err := c.TypeText(TextRequest{Target: goodTarget(), Text: "hi", DryRun: true})
	if err != nil {
		t.Fatalf("keyboard dry run: %v", err)
	}
	if _, err := c.TypeText(TextRequest{
		Target: goodTarget(), Text: "hi", Token: preview.Plan.Token,
	}); err != nil {
		t.Fatalf("keyboard confirm: %v", err)
	}
	if len(f.texts) != 1 || f.texts[0] != "hi" {
		t.Fatalf("expected the confirmed text once, got %v", f.texts)
	}
}

func TestDenyTextTooLong(t *testing.T) {
	c, f := setup(t, func(cfg *Config) {
		cfg.Capabilities.KeyboardEnabled = true
		cfg.Capabilities.MaxTextLength = 4
	})
	_, err := c.TypeText(TextRequest{Target: goodTarget(), Text: "123456"})
	wantDeny(t, err, DenyTextTooLong)
	assertNoMutation(t, f)
}

func TestDenyKeyNotWhitelisted(t *testing.T) {
	c, f := setup(t, func(cfg *Config) { cfg.Capabilities.KeyboardEnabled = true })
	// A free-form combo string must never be accepted: it could express
	// Win+R and open a shell.
	_, err := c.PressKey(KeyRequest{Target: goodTarget(), Key: "WIN+R"})
	wantDeny(t, err, DenyParam)
	assertNoMutation(t, f)
}

func TestAllowWhitelistedKey(t *testing.T) {
	c, f := setup(t, func(cfg *Config) { cfg.Capabilities.KeyboardEnabled = true })
	if _, err := c.PressKey(KeyRequest{Target: goodTarget(), Key: "enter"}); err != nil {
		t.Fatalf("key press: %v", err)
	}
	if len(f.keys) != 1 || f.keys[0] != "enter" {
		t.Fatalf("expected the key to be forwarded, got %v", f.keys)
	}
}

func TestScreenshotFullScreen(t *testing.T) {
	c, f := setup(t, nil)
	res, err := c.Screenshot(ScreenshotRequest{Scope: "screen"})
	if err != nil {
		t.Fatalf("screenshot: %v", err)
	}
	if len(f.shots) != 1 || f.shots[0] != (Rect{0, 0, 1920, 1080}) {
		t.Fatalf("expected a full virtual-desktop capture, got %v", f.shots)
	}
	if res.Plan.Extra["image"] == nil {
		t.Fatal("screenshot result should carry the image")
	}
}

func TestDenyFullScreenWhenBlocked(t *testing.T) {
	c, f := setup(t, func(cfg *Config) { cfg.Capabilities.AllowFullScreen = false })
	_, err := c.Screenshot(ScreenshotRequest{Scope: "screen"})
	wantDeny(t, err, DenyFullScreenBlocked)
	assertNoMutation(t, f)
}

func TestScreenshotWindowUsesWindowRect(t *testing.T) {
	c, f := setup(t, nil)
	_, err := c.Screenshot(ScreenshotRequest{
		Scope: "window", HasTarget: true, Target: goodTarget(),
	})
	if err != nil {
		t.Fatalf("window screenshot: %v", err)
	}
	if len(f.shots) != 1 || f.shots[0] != (Rect{100, 200, 800, 600}) {
		t.Fatalf("expected the window rect, got %v", f.shots)
	}
}

func TestDenyScreenshotRegionOutsideWindow(t *testing.T) {
	c, f := setup(t, nil)
	tgt := goodTarget()
	tgt.MouseX = 799 // inside the tolerance but on the last column
	_, err := c.Screenshot(ScreenshotRequest{Scope: "region", HasTarget: true, Target: tgt})
	wantDeny(t, err, DenyOutOfWindow)
	assertNoMutation(t, f)
}

func TestDenyScreenshotWindowNeedsTarget(t *testing.T) {
	c, f := setup(t, nil)
	_, err := c.Screenshot(ScreenshotRequest{Scope: "window"})
	wantDeny(t, err, DenyParam)
	assertNoMutation(t, f)
}

func TestDenyScreenshotBadScope(t *testing.T) {
	c, f := setup(t, nil)
	_, err := c.Screenshot(ScreenshotRequest{Scope: "everything"})
	wantDeny(t, err, DenyParam)
	assertNoMutation(t, f)
}

func TestDenyScreenshotWhenDisabled(t *testing.T) {
	c, f := setup(t, func(cfg *Config) { cfg.Capabilities.ScreenshotEnabled = false })
	_, err := c.Screenshot(ScreenshotRequest{Scope: "screen"})
	wantDeny(t, err, DenyDisabled)
	assertNoMutation(t, f)
}

func TestListWindowsFilterAndLimit(t *testing.T) {
	c, f := setup(t, nil)
	f.windows = append(f.windows,
		WindowInfo{Handle: 0x2, Title: "chrome - example", Rect: Rect{0, 0, 100, 100}, Visible: true},
		WindowInfo{Handle: 0x3, Title: "chrome - other", Rect: Rect{0, 0, 100, 100}, Visible: true},
	)
	got, err := c.ListWindows("chrome", 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 chrome windows, got %d", len(got))
	}
	got, err = c.ListWindows("", 2)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("limit 2 must be honoured, got %d", len(got))
	}
}

func TestSubstringTitleMatch(t *testing.T) {
	c, f := setup(t, nil)
	f.windows[0].Title = "Untitled - Notepad"
	// A truncated title still resolves when it is unambiguous.
	_, err := c.Click(ClickRequest{Target: Target{
		WindowTitle: "Notepad", WindowX: 100, WindowY: 200, MouseX: 40, MouseY: 60,
	}})
	if err != nil {
		t.Fatalf("unambiguous substring should match: %v", err)
	}
}

func TestScreenFingerprintChangesWithLayout(t *testing.T) {
	_, f := setup(t, nil)
	before, _ := f.ScreenInfo()
	after := before
	after.Virtual = Rect{0, 0, 2560, 1440}
	if before.Fingerprint() == after.Fingerprint() {
		t.Fatal("fingerprint must change when the display layout changes")
	}
}

func TestRateLimitWindowExpires(t *testing.T) {
	c, _ := setup(t, func(cfg *Config) { cfg.MaxActionsPerMinute = 1 })
	if _, err := c.Click(ClickRequest{Target: goodTarget()}); err != nil {
		t.Fatalf("first click: %v", err)
	}
	restore := nowFunc
	nowFunc = func() time.Time { return restore().Add(2 * time.Minute) }
	defer func() { nowFunc = restore }()
	if _, err := c.Click(ClickRequest{Target: goodTarget()}); err != nil {
		t.Fatalf("after the window slides the limit must reset: %v", err)
	}
}

func TestMoveOnlyNoButton(t *testing.T) {
	c, f := setup(t, nil)
	if _, err := c.Move(MoveRequest{Target: goodTarget()}); err != nil {
		t.Fatalf("move: %v", err)
	}
	if len(f.moves) != 1 {
		t.Fatalf("expected exactly one move, got %v", f.moves)
	}
	if len(f.downs) != 0 || len(f.ups) != 0 {
		t.Fatalf("move must not press any button, got %v / %v", f.downs, f.ups)
	}
}

// assertNoMutation is the invariant that makes the whole design safe: a
// rejected call must leave the desktop untouched.
func assertNoMutation(t *testing.T, f *fakeBackend) {
	t.Helper()
	if got := f.mutatingCalls(); len(got) != 0 {
		t.Fatalf("rejected call reached the backend: %v", got)
	}
}

func TestRectHelpers(t *testing.T) {
	// A monitor left of the primary one gives a negative origin; the helpers
	// must not assume a rect starts at 0.
	r := Rect{X: -1920, Y: 0, Width: 1920, Height: 1080}
	if r.Right() != 0 || r.Bottom() != 1080 {
		t.Fatalf("unexpected edges: right=%d bottom=%d", r.Right(), r.Bottom())
	}
	if !r.Contains(-1920, 0) {
		t.Fatal("left/top edge must be inside")
	}
	if r.Contains(0, 0) {
		t.Fatal("right edge is exclusive")
	}
	if r.Contains(-1921, 0) {
		t.Fatal("points left of the rect must be outside")
	}
	if r.Contains(-100, 1080) {
		t.Fatal("bottom edge is exclusive")
	}
}

func TestDenyErrorMentionsSuggestion(t *testing.T) {
	c, f := setup(t, nil)
	f.windows[0].Rect = Rect{X: 500, Y: 200, Width: 800, Height: 600}
	_, err := c.Click(ClickRequest{Target: goodTarget()})
	var de *DenyError
	if !errors.As(err, &de) {
		t.Fatalf("expected DenyError, got %v", err)
	}
	// The AI must be told how to recover, not just that it failed.
	if de.Suggest == "" {
		t.Fatal("a deny must carry a recovery suggestion")
	}
	if de.Passed == "" || de.Real == "" {
		t.Fatalf("a coordinate deny should echo both positions, got passed=%q real=%q", de.Passed, de.Real)
	}
}
