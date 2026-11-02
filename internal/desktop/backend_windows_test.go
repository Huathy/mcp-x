//go:build windows

package desktop

import (
	"sync"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// These assertions guard the Win32 struct layouts. A mismatch here would not
// fail to compile; it would silently corrupt every synthetic input event, so
// the sizes are pinned explicitly.

// INPUT is { DWORD type; 4 bytes padding; union }, where the union is 32 bytes
// on x64 (MOUSEINPUT with ULONG_PTR) and 24 on x86.
func TestInputStructLayout(t *testing.T) {
	if got := unsafe.Sizeof(input{}); got != 40 {
		t.Errorf("sizeof(INPUT) = %d, want 40 on 64-bit Windows", got)
	}
	if got := unsafe.Sizeof(mouseInput{}); got != 32 {
		t.Errorf("sizeof(MOUSEINPUT) = %d, want 32 on 64-bit Windows", got)
	}
	if got := unsafe.Sizeof(keyboardInput{}); got != 24 {
		t.Errorf("sizeof(KEYBDINPUT) = %d, want 24 on 64-bit Windows", got)
	}
	// The union must start at offset 8: 4-byte type plus alignment padding.
	var in input
	if off := unsafe.Offsetof(in.Mi); off != 8 {
		t.Errorf("INPUT union offset = %d, want 8", off)
	}
}

// The keyboard overlay writes into the same bytes as the mouse payload, so
// the shared region must be at least as large as the biggest member.
func TestKeyboardOverlayFitsInUnion(t *testing.T) {
	if unsafe.Sizeof(keyboardInput{}) > unsafe.Sizeof(mouseInput{}) {
		t.Error("keyboard overlay is larger than the union it is written into")
	}
}

// Backend construction must succeed on Windows and report the platform as
// available so the MCP layer registers the tools.
func TestBackendAvailableOnWindows(t *testing.T) {
	if !Available() {
		t.Fatal("Available() must be true on Windows")
	}
	b, err := NewBackend(true)
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	if b == nil {
		t.Fatal("NewBackend returned a nil backend")
	}
}

// The DPI switch is process-wide and can only be set once per process, so the
// result is cached; this just asserts the call path does not panic.
func TestDPIAwarenessIsAppliedOnce(t *testing.T) {
	dpiOnce = sync.Once{}
	first := enableDPIAwareness(true)
	second := enableDPIAwareness(true)
	// A second direct call may fail because the process is already aware, but
	// it must not panic; the cached value is what the backend uses.
	_ = first
	_ = second
}

// Every dynamically-resolved Win32 entry point must exist in the DLL it is
// bound to. A wrong DLL binding compiles fine and panics only at call time,
// so the names are asserted up front.
func TestWin32EntryPointsResolve(t *testing.T) {
	procs := map[string]*windows.LazyProc{
		"user32: SetProcessDpiAwarenessContext": procSetProcessDpiAwarenessContext,
		"user32: SetProcessDPIAware":            procSetProcessDPIAware,
		"user32: GetDpiForSystem":               procGetDpiForSystem,
		"user32: GetSystemMetrics":              procGetSystemMetrics,
		"user32: EnumDisplayMonitors":           procEnumDisplayMonitors,
		"user32: GetMonitorInfoW":               procGetMonitorInfoW,
		"user32: GetWindowLongPtrW":             procGetWindowLongPtrW,
		"user32: GetWindowLongW":                procGetWindowLongW,
		"user32: EnumWindows":                   procEnumWindows,
		"user32: GetWindowTextLengthW":          procGetWindowTextLengthW,
		"user32: GetWindowTextW":                procGetWindowTextW,
		"user32: GetClassNameW":                 procGetClassNameW,
		"user32: GetWindowRect":                 procGetWindowRect,
		"user32: IsWindowVisible":               procIsWindowVisible,
		"user32: IsIconic":                      procIsIconic,
		"user32: GetForegroundWindow":           procGetForegroundWindow,
		"user32: GetWindowThreadProcessId":      procGetWindowThreadProcessId,
		"user32: SetForegroundWindow":           procSetForegroundWindow,
		"user32: WindowFromPoint":               procWindowFromPoint,
		"user32: GetAncestor":                   procGetAncestor,
		"user32: GetCursorPos":                  procGetCursorPos,
		"user32: SetCursorPos":                  procSetCursorPos,
		"user32: SendInput":                     procSendInput,
		"user32: GetDC":                         procGetDC,
		"user32: ReleaseDC":                     procReleaseDC,
		"user32: AttachThreadInput":             procAttachThreadInput,
		// GetDeviceCaps is in gdi32.dll; binding it to user32 panics on call.
		"gdi32: GetDeviceCaps":          procGetDeviceCaps,
		"gdi32: CreateCompatibleDC":     procCreateCompatibleDC,
		"gdi32: CreateCompatibleBitmap": procCreateCompatibleBitmap,
		"gdi32: SelectObject":           procSelectObject,
		"gdi32: BitBlt":                 procBitBlt,
		"gdi32: GetDIBits":              procGetDIBits,
		"gdi32: DeleteDC":               procDeleteDC,
		"gdi32: DeleteObject":           procDeleteObject,
	}
	for name, proc := range procs {
		if proc == nil {
			t.Errorf("%s is not bound", name)
			continue
		}
		if err := proc.Find(); err != nil {
			t.Errorf("%s does not resolve: %v", name, err)
		}
	}
}

// GetDpiForMonitor is a documented shcore.dll export. It is probed at runtime
// for older builds, so a missing one is tolerated, but a wrong DLL is not.
func TestDpiForMonitorIsBoundToShcore(t *testing.T) {
	if procGetDpiForMonitor == nil {
		t.Fatal("GetDpiForMonitor must be bound")
	}
	if err := procGetDpiForMonitor.Find(); err != nil {
		t.Skipf("shcore.dll/GetDpiForMonitor unavailable on this build: %v", err)
	}
}

// A live call proves the display and window APIs work in an interactive
// session, which is the precondition for the whole module. It is skipped in
// headless or service (Session 0) contexts.
func TestScreenInfoReportsLayout(t *testing.T) {
	b, err := NewBackend(true)
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	info, err := b.ScreenInfo()
	if err != nil {
		t.Skipf("no interactive desktop: %v", err)
	}
	if len(info.Monitors) == 0 {
		t.Fatal("ScreenInfo must report at least one monitor in an interactive session")
	}
	if info.Virtual.Width <= 0 || info.Virtual.Height <= 0 {
		t.Errorf("virtual desktop must have a positive size, got %s", info.Virtual.String())
	}
	if info.Dpi <= 0 {
		t.Errorf("DPI must be positive, got %d", info.Dpi)
	}
	fp := info.Fingerprint()
	if fp == "" {
		t.Error("fingerprint must not be empty")
	}
}

func TestListWindowsReturnsTitledWindows(t *testing.T) {
	b, err := NewBackend(true)
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	wins, err := b.ListWindows()
	if err != nil {
		t.Skipf("no interactive desktop: %v", err)
	}
	for _, w := range wins {
		if w.Handle == 0 {
			t.Error("window handle must be non-zero")
		}
		if w.Title == "" {
			t.Error("untitled windows must be filtered out; window_title is required")
		}
		if w.Rect.Width <= 0 || w.Rect.Height <= 0 {
			t.Errorf("window %q has a degenerate rect %s", w.Title, w.Rect.String())
		}
	}
}

// The hit test must return the top-level owner, matching what a click at that
// point would actually be delivered to.
//
// WindowFromPoint returns NULL when the process is not attached to the input
// desktop, which is a normal setup for an MCP server started from an IDE. The
// backend therefore falls back to a Z-order scan; this test accepts either
// path but requires that a point inside a visible window resolves to *some*
// real window rather than nothing.
func TestWindowAtPointResolvesToAWindow(t *testing.T) {
	b, err := NewBackend(true)
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	wins, err := b.ListWindows()
	if err != nil || len(wins) == 0 {
		t.Skipf("no interactive desktop or no windows: %v", err)
	}
	known := make(map[uintptr]bool, len(wins))
	for _, w := range wins {
		known[w.Handle] = true
	}

	// Probe the centre of every visible window; a point that another window
	// covers may legitimately resolve to that other window, but it must never
	// resolve to nothing when the desktop reports windows at all.
	resolved := 0
	for _, target := range wins {
		if target.Minimized {
			continue
		}
		x := target.Rect.X + target.Rect.Width/2
		y := target.Rect.Y + target.Rect.Height/2
		got, ok, err := b.WindowAtPoint(x, y)
		if err != nil {
			t.Fatalf("WindowAtPoint(%d,%d): %v", x, y, err)
		}
		if !ok {
			continue
		}
		resolved++
		if got.Handle == 0 {
			t.Errorf("point (%d,%d) resolved to a zero handle", x, y)
		}
	}
	if resolved == 0 {
		t.Error("no probe point resolved to a window; the hit test is not usable " +
			"in this desktop context")
	}
}

// The Z-order fallback must return the foreground window for a point inside
// it, since nothing can be above the foreground window.
func TestHitTestResolvesForegroundWindow(t *testing.T) {
	b, err := NewBackend(true)
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	wins, err := b.ListWindows()
	if err != nil {
		t.Skipf("no interactive desktop: %v", err)
	}
	var fg *WindowInfo
	for i := range wins {
		if wins[i].Foreground && !wins[i].Minimized {
			fg = &wins[i]
			break
		}
	}
	if fg == nil {
		t.Skip("no foreground window in this session")
	}
	x := fg.Rect.X + fg.Rect.Width/2
	y := fg.Rect.Y + fg.Rect.Height/2
	got, ok, err := b.WindowAtPoint(x, y)
	if err != nil || !ok {
		t.Skipf("hit test unavailable at the foreground window: %v", err)
	}
	if got.Handle != fg.Handle {
		t.Errorf("point inside the foreground window resolved to %q (hwnd %d), want %q (hwnd %d)",
			got.Title, got.Handle, fg.Title, fg.Handle)
	}
}

func TestCursorPosIsReadable(t *testing.T) {
	b, err := NewBackend(true)
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	x, y, err := b.CursorPos()
	if err != nil {
		t.Skipf("no interactive desktop: %v", err)
	}
	if x < 0 || y < 0 {
		t.Errorf("cursor position should be non-negative, got (%d,%d)", x, y)
	}
}
