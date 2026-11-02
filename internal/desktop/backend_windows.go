//go:build windows

package desktop

import (
	"fmt"
	"image"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	shcore32 = windows.NewLazySystemDLL("shcore.dll")

	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procSetProcessDPIAware            = user32.NewProc("SetProcessDPIAware")
	procGetDpiForSystem               = user32.NewProc("GetDpiForSystem")
	procGetSystemMetrics              = user32.NewProc("GetSystemMetrics")
	procEnumDisplayMonitors           = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW               = user32.NewProc("GetMonitorInfoW")
	// GetDpiForMonitor is exported by shcore.dll, not user32.dll.
	procGetDpiForMonitor         = shcore32.NewProc("GetDpiForMonitor")
	procGetWindowLongPtrW        = user32.NewProc("GetWindowLongPtrW")
	procGetWindowLongW           = user32.NewProc("GetWindowLongW")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procGetWindowTextLengthW     = user32.NewProc("GetWindowTextLengthW")
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
	procGetClassNameW            = user32.NewProc("GetClassNameW")
	procGetWindowRect            = user32.NewProc("GetWindowRect")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procIsIconic                 = user32.NewProc("IsIconic")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procWindowFromPoint          = user32.NewProc("WindowFromPoint")
	procGetAncestor              = user32.NewProc("GetAncestor")
	procGetCursorPos             = user32.NewProc("GetCursorPos")
	procSetCursorPos             = user32.NewProc("SetCursorPos")
	procSendInput                = user32.NewProc("SendInput")
	procGetDC                    = user32.NewProc("GetDC")
	procReleaseDC                = user32.NewProc("ReleaseDC")
	procAttachThreadInput        = user32.NewProc("AttachThreadInput")

	// GetDeviceCaps is exported by gdi32.dll, not user32.dll.
	procGetDeviceCaps = gdi32.NewProc("GetDeviceCaps")

	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject           = gdi32.NewProc("SelectObject")
	procBitBlt                 = gdi32.NewProc("BitBlt")
	procGetDIBits              = gdi32.NewProc("GetDIBits")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
	procDeleteObject           = gdi32.NewProc("DeleteObject")
)

// System metrics indices.
const (
	smCXScreen        = 0
	smCYScreen        = 1
	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCXVirtualScreen = 78
	smCYVirtualScreen = 79
)

// Window styles and ancestors.
const (
	gwlExStyle                     = -20
	gaRoot                         = 2
	processQueryLimitedInformation = 0x1000
)

// Input types and flags.
const (
	inputMouse    = 0
	inputKeyboard = 1

	moUsefMove       = 0x0001
	moUsefLeftDown   = 0x0002
	moUsefLeftUp     = 0x0004
	moUsefRightDown  = 0x0008
	moUsefRightUp    = 0x0010
	moUsefMiddleDown = 0x0020
	moUsefMiddleUp   = 0x0040
	moUsefWheel      = 0x0800
	moUsefHWheel     = 0x1000

	keyEventfKeyUp   = 0x0002
	keyEventfUnicode = 0x0004

	wheelDelta = 120
)

// GDI constants.
const (
	srccopy      = 0x00CC0020
	captureBlt   = 0x40000000
	dibRGBColors = 0
	biRGB        = 0
)

type winPoint struct {
	X int32
	Y int32
}

type winRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type monitorInfoEx struct {
	Size    uint32
	Monitor winRect
	Work    winRect
	Flags   uint32
	Device  [32]uint16
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type mouseInput struct {
	DX, DY                 int32
	MouseData, Flags, Time uint32
	ExtraInfo              uintptr
}

type keyboardInput struct {
	Vk, Scan    uint16
	Flags, Time uint32
	ExtraInfo   uintptr
}

type hardwareInput struct {
	Msg            uint32
	ParamL, ParamH uint16
}

// input mirrors the Win32 INPUT struct, whose second field is an anonymous
// union. mouseInput is the largest member (32 bytes on x64, 24 on x86), so the
// union is modelled as mouseInput and the other payloads are written into the
// same bytes through a typed overlay. Go's own alignment rules reproduce the
// Win32 padding: mouseInput's uintptr forces the union to start at offset 8 on
// x64, which is exactly what the C struct does.
//
// desktop_windows_test.go asserts these sizes so a layout regression fails the
// build's test run rather than silently corrupting every input event.
type input struct {
	Type uint32
	Mi   mouseInput
}

func (i *input) setKeyboard(ki keyboardInput) {
	*(*keyboardInput)(unsafe.Pointer(&i.Mi)) = ki
}

// namedKeys maps the MCP key whitelist to virtual-key codes.
var namedKeys = map[string]uint16{
	"ENTER":     0x0D,
	"TAB":       0x09,
	"ESC":       0x1B,
	"BACKSPACE": 0x08,
	"DELETE":    0x2E,
	"SPACE":     0x20,
	"UP":        0x26,
	"DOWN":      0x28,
	"LEFT":      0x25,
	"RIGHT":     0x27,
	"HOME":      0x24,
	"END":       0x23,
	"PAGEUP":    0x21,
	"PAGEDOWN":  0x22,
	"F1":        0x70,
	"F2":        0x71,
	"F3":        0x72,
	"F4":        0x73,
	"F5":        0x74,
	"F6":        0x75,
	"F7":        0x76,
	"F8":        0x77,
	"F9":        0x78,
	"F10":       0x79,
	"F11":       0x7A,
	"F12":       0x7B,
}

const (
	vkShift   = 0x10
	vkControl = 0x11
	vkMenu    = 0x12
)

var (
	dpiOnce sync.Once

	// The EnumWindows and EnumDisplayMonitors callbacks must be created once.
	// NewCallback reserves a slot in a process-wide table; creating one per
	// call would eventually exhaust it.
	enumWindowsOnce  sync.Once
	enumWindowsProc  uintptr
	enumMonitorsOnce sync.Once
	enumMonitorsProc uintptr

	// processNames caches PID -> exe name; resolving it per window per call
	// would open a process handle for every enumerated window.
	nameCacheMu sync.Mutex
	nameCache   = map[uint32]string{}
)

func platformAvailable() bool { return true }

func newBackend(dpiAware bool) (Backend, error) {
	// The process-wide DPI mode can only be set once, so the switch runs under
	// a sync.Once; later callers reuse whatever the first call established.
	dpiOnce.Do(func() {
		enableDPIAwareness(dpiAware)
	})
	return &winBackend{}, nil
}

// enableDPIAwareness makes the process per-monitor DPI aware. Without it,
// GetWindowRect returns virtualized coordinates on a scaled display while
// SetCursorPos expects physical pixels, so every computed point is offset on
// 125%/150% screens.
func enableDPIAwareness(enabled bool) bool {
	if !enabled {
		return false
	}
	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2, available since Win10 1703.
	// The value is a pseudo-handle constant, passed as a pointer-sized value;
	// -4 is the documented encoding for PER_MONITOR_AWARE_V2.
	if err := procSetProcessDpiAwarenessContext.Find(); err == nil {
		const perMonitorAwareV2 = ^uintptr(3) // -4
		if ret, _, _ := procSetProcessDpiAwarenessContext.Call(perMonitorAwareV2); ret != 0 {
			return true
		}
	}
	if err := procSetProcessDPIAware.Find(); err == nil {
		if ret, _, _ := procSetProcessDPIAware.Call(); ret != 0 {
			return true
		}
	}
	return false
}

type winBackend struct{}

func (w *winBackend) ScreenInfo() (ScreenInfo, error) {
	primaryW, _, _ := procGetSystemMetrics.Call(smCXScreen)
	primaryH, _, _ := procGetSystemMetrics.Call(smCYScreen)
	vx, _, _ := procGetSystemMetrics.Call(smXVirtualScreen)
	vy, _, _ := procGetSystemMetrics.Call(smYVirtualScreen)
	vw, _, _ := procGetSystemMetrics.Call(smCXVirtualScreen)
	vh, _, _ := procGetSystemMetrics.Call(smCYVirtualScreen)

	dpi := systemDPI()
	info := ScreenInfo{
		PrimaryWidth:  int(primaryW),
		PrimaryHeight: int(primaryH),
		Dpi:           dpi,
		ScalePct:      dpi * 100 / 96,
		Virtual: Rect{
			X:      int(int32(vx)),
			Y:      int(int32(vy)),
			Width:  int(vw),
			Height: int(vh),
		},
	}

	monitors, err := listMonitors()
	if err != nil {
		return ScreenInfo{}, err
	}
	info.Monitors = monitors

	if x, y, err := w.CursorPos(); err == nil {
		info.CursorX, info.CursorY = x, y
	}
	if fg, err := w.foregroundWindow(); err == nil {
		info.Foreground = fg.Title
	}
	return info, nil
}

func systemDPI() int {
	// GetDpiForSystem exists since Windows 10 1607; below that the DC-based
	// fallback is used.
	if err := procGetDpiForSystem.Find(); err == nil {
		if dpi, _, _ := procGetDpiForSystem.Call(); dpi != 0 {
			return int(dpi)
		}
	}
	if dpi := deviceDPI(); dpi > 0 {
		return dpi
	}
	return 96
}

// deviceDPI reads LOGPIXELSX from a screen DC. GetDeviceCaps is in gdi32.dll.
func deviceDPI() int {
	hdc, _, _ := procGetDC.Call(0)
	if hdc == 0 {
		return 0
	}
	defer procReleaseDC.Call(0, hdc)
	// LOGPIXELSX = 88
	if dpi, _, _ := procGetDeviceCaps.Call(hdc, 88); dpi != 0 {
		return int(dpi)
	}
	return 0
}

func listMonitors() ([]Monitor, error) {
	enumMonitorsOnce.Do(func() {
		enumMonitorsProc = windows.NewCallback(monitorEnumProc)
	})
	// The callback cannot take a Go pointer, so it appends to this package-level
	// slice. It has to be reset per call or repeated calls would accumulate.
	monitorSink = nil
	ret, _, _ := procEnumDisplayMonitors.Call(0, 0, enumMonitorsProc, 0)
	if ret == 0 {
		return nil, fmt.Errorf("EnumDisplayMonitors failed")
	}
	return monitorSink, nil
}

// monitorEnumProc is a windows.NewCallback target, so it may only use types
// that are safe to move between the Go and Windows stacks: no Go pointers,
// no closures over Go state. Results are stashed in a package-level collector.
var monitorSink []Monitor

func monitorEnumProc(hMonitor, hdc, lprc, lparam uintptr) uintptr {
	var mi monitorInfoEx
	mi.Size = uint32(unsafe.Sizeof(mi))
	if ret, _, _ := procGetMonitorInfoW.Call(hMonitor, uintptr(unsafe.Pointer(&mi))); ret == 0 {
		return 1
	}
	primary := mi.Flags&1 != 0
	dpi := systemDPI()
	// GetDpiForMonitor lives in shcore.dll and reports the per-monitor scale.
	// MDT_EFFECTIVE_DPI = 0. Absent on very old builds, hence the probe.
	if err := procGetDpiForMonitor.Find(); err == nil {
		var x, y uint32
		if ret, _, _ := procGetDpiForMonitor.Call(hMonitor, 0,
			uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&y))); ret == 0 && x > 0 {
			dpi = int(x)
		}
	}
	monitorSink = append(monitorSink, Monitor{
		Index:    len(monitorSink),
		Primary:  primary,
		Device:   windows.UTF16ToString(mi.Device[:]),
		Rect:     toRect(mi.Monitor),
		Dpi:      dpi,
		ScalePct: dpi * 100 / 96,
	})
	return 1
}

func toRect(r winRect) Rect {
	return Rect{
		X:      int(r.Left),
		Y:      int(r.Top),
		Width:  int(r.Right - r.Left),
		Height: int(r.Bottom - r.Top),
	}
}

func (w *winBackend) ListWindows() ([]WindowInfo, error) {
	all, err := w.enumWindows()
	if err != nil {
		return nil, err
	}
	// window_title is mandatory in the tool schema, so offering an untitled
	// window would only invite a call that cannot succeed.
	out := all[:0]
	for _, win := range all {
		if strings.TrimSpace(win.Title) == "" {
			continue
		}
		out = append(out, win)
	}
	return out, nil
}

// enumWindows lists visible top-level windows front to back, including
// untitled ones. An untitled popup still occludes whatever is beneath it, so
// the hit test must see it even though the AI is never offered it as a target.
func (w *winBackend) enumWindows() ([]WindowInfo, error) {
	enumWindowsOnce.Do(func() {
		enumWindowsProc = windows.NewCallback(windowEnumProc)
	})
	windowSink = nil
	ret, _, _ := procEnumWindows.Call(enumWindowsProc, 0)
	if ret == 0 {
		return nil, fmt.Errorf("EnumWindows failed")
	}
	return windowSink, nil
}

var windowSink []WindowInfo

// windowEnumProc is a windows.NewCallback target; see monitorEnumProc.
func windowEnumProc(hwnd, lparam uintptr) uintptr {
	visible, _, _ := procIsWindowVisible.Call(hwnd)
	if visible == 0 {
		return 1
	}
	// Skip tool/overlay windows so the AI is not offered a floating shell
	// surface as a click target.
	if getWindowLong(hwnd, gwlExStyle)&toolWindowMask != 0 {
		return 1
	}

	var r winRect
	if ret, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); ret == 0 {
		return 1
	}
	rect := toRect(r)
	if rect.Width <= 0 || rect.Height <= 0 {
		return 1
	}
	// The title is intentionally not filtered here: an untitled popup still
	// occludes whatever is beneath it, so the hit test has to see it.
	// ListWindows drops untitled entries before the AI is offered them.
	title := windowText(hwnd)
	minimized, _, _ := procIsIconic.Call(hwnd)
	foreground, _, _ := procGetForegroundWindow.Call()
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))

	windowSink = append(windowSink, WindowInfo{
		Handle:     hwnd,
		Title:      title,
		Class:      windowClass(hwnd),
		Rect:       rect,
		Visible:    true,
		Minimized:  minimized != 0,
		Foreground: hwnd == foreground,
		PID:        pid,
		Process:    processName(pid),
	})
	return 1
}

// toolWindowMask is WS_EX_TOOLWINDOW (0x00000080).
const toolWindowMask = 0x00000080

// GetWindowLongPtrW only exists on 64-bit Windows; 32-bit builds must use
// GetWindowLongW. The style values read here fit in 32 bits either way.
func getWindowLong(hwnd uintptr, index int32) uintptr {
	if unsafe.Sizeof(uintptr(0)) == 8 {
		ret, _, _ := procGetWindowLongPtrW.Call(hwnd, uintptr(index))
		return ret
	}
	ret, _, _ := procGetWindowLongW.Call(hwnd, uintptr(index))
	return ret
}

func windowText(hwnd uintptr) string {
	length, _, _ := procGetWindowTextLengthW.Call(hwnd)
	if length == 0 {
		return ""
	}
	buf := make([]uint16, length+1)
	ret, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if ret == 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

func windowClass(hwnd uintptr) string {
	buf := make([]uint16, 256)
	ret, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if ret == 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

func processName(pid uint32) string {
	nameCacheMu.Lock()
	if name, ok := nameCache[pid]; ok {
		nameCacheMu.Unlock()
		return name
	}
	nameCacheMu.Unlock()

	name := ""
	h, err := windows.OpenProcess(processQueryLimitedInformation, false, pid)
	if err == nil {
		buf := make([]uint16, windows.MAX_LONG_PATH)
		size := uint32(len(buf))
		if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err == nil {
			full := windows.UTF16ToString(buf[:size])
			if idx := strings.LastIndexAny(full, `\/`); idx >= 0 {
				name = full[idx+1:]
			} else {
				name = full
			}
		}
		windows.CloseHandle(h)
	}
	nameCacheMu.Lock()
	if len(nameCache) > 512 {
		nameCache = map[uint32]string{}
	}
	nameCache[pid] = name
	nameCacheMu.Unlock()
	return name
}

// WindowAtPoint reports which top-level window would receive a click at the
// given screen point. It is the core of the occlusion check, so it must not
// silently answer "nothing" when the API is merely unavailable.
func (w *winBackend) WindowAtPoint(x, y int) (WindowInfo, bool, error) {
	if info, ok := w.windowFromPoint(x, y); ok {
		return info, true, nil
	}
	// WindowFromPoint returns NULL when the calling thread is not attached to
	// the input desktop (a common setup for MCP servers launched from an IDE
	// or a shell). Answering "nothing here" in that case would make the
	// occlusion check reject every operation, so fall back to a z-order scan:
	// EnumWindows returns top-level windows front to back, so the first
	// non-minimized window whose rect contains the point is the one a click
	// would land on.
	return w.topmostAtPoint(x, y)
}

func (w *winBackend) windowFromPoint(x, y int) (WindowInfo, bool) {
	pt := winPoint{X: int32(x), Y: int32(y)}
	hwnd, _, _ := procWindowFromPoint.Call(uintptr(unsafe.Pointer(&pt)))
	if hwnd == 0 {
		return WindowInfo{}, false
	}
	// WindowFromPoint returns the deepest child; the click is delivered to the
	// top-level owner, so the comparison must use GA_ROOT.
	root, _, _ := procGetAncestor.Call(hwnd, gaRoot)
	if root == 0 {
		root = hwnd
	}
	visible, _, _ := procIsWindowVisible.Call(root)
	if visible == 0 {
		return WindowInfo{}, false
	}
	return w.describe(root)
}

func (w *winBackend) topmostAtPoint(x, y int) (WindowInfo, bool, error) {
	wins, err := w.enumWindows()
	if err != nil {
		return WindowInfo{}, false, err
	}
	// EnumWindows walks the Z-order front to back, so the first window whose
	// rect contains the point is the one a click would land on.
	for _, win := range wins {
		if win.Minimized {
			continue
		}
		if win.Rect.Contains(x, y) {
			return win, true, nil
		}
	}
	return WindowInfo{}, false, nil
}

// describe reads the fields the validation chain needs for one window handle.
func (w *winBackend) describe(hwnd uintptr) (WindowInfo, bool) {
	var r winRect
	if ret, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); ret == 0 {
		return WindowInfo{}, false
	}
	minimized, _, _ := procIsIconic.Call(hwnd)
	foreground, _, _ := procGetForegroundWindow.Call()
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return WindowInfo{
		Handle:     hwnd,
		Title:      windowText(hwnd),
		Class:      windowClass(hwnd),
		Rect:       toRect(r),
		Visible:    true,
		Minimized:  minimized != 0,
		Foreground: hwnd == foreground,
		PID:        pid,
		Process:    processName(pid),
	}, true
}

func (w *winBackend) foregroundWindow() (WindowInfo, error) {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return WindowInfo{}, fmt.Errorf("no foreground window")
	}
	var r winRect
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return WindowInfo{Handle: hwnd, Title: windowText(hwnd), Rect: toRect(r), Foreground: true}, nil
}

func (w *winBackend) SetForeground(hwnd uintptr) error {
	// SetForegroundWindow is refused when the caller is not the active
	// process, so route through the standard attach-and-attach dance.
	foreground, _, _ := procGetForegroundWindow.Call()
	threadID, _, _ := procGetWindowThreadProcessId.Call(foreground, 0)
	currentThread := uintptr(windows.GetCurrentThreadId())
	procAttachThreadInput.Call(currentThread, threadID, 1)
	defer procAttachThreadInput.Call(currentThread, threadID, 0)
	if ret, _, _ := procSetForegroundWindow.Call(hwnd); ret == 0 {
		return fmt.Errorf("SetForegroundWindow(%d) failed: %w", hwnd, syscall.Errno(0))
	}
	return nil
}

func (w *winBackend) CursorPos() (int, int, error) {
	var pt winPoint
	if ret, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt))); ret == 0 {
		return 0, 0, fmt.Errorf("GetCursorPos failed")
	}
	return int(pt.X), int(pt.Y), nil
}

func (w *winBackend) MoveTo(x, y int) error {
	if ret, _, _ := procSetCursorPos.Call(uintptr(int32(x)), uintptr(int32(y))); ret == 0 {
		return fmt.Errorf("SetCursorPos(%d,%d) failed", x, y)
	}
	return nil
}

func (w *winBackend) MouseDown(button string) error {
	flag, err := downFlag(button)
	if err != nil {
		return err
	}
	return w.sendMouse(0, 0, 0, flag)
}

func (w *winBackend) MouseUp(button string) error {
	flag, err := upFlag(button)
	if err != nil {
		return err
	}
	return w.sendMouse(0, 0, 0, flag)
}

func (w *winBackend) MouseWheel(horizontal bool, notches int) error {
	// MOUSEEVENTF_WHEEL takes a signed WHEEL_DELTA multiple where positive
	// means up (or right for HWHEEL); the sign must survive as a 32-bit
	// two's-complement value, so the conversion goes through int32.
	flag := uint32(moUsefWheel)
	if horizontal {
		flag = uint32(moUsefHWheel)
	}
	delta := int32(notches) * wheelDelta
	if delta > 0x7FFFFFFF {
		delta = 0x7FFFFFFF
	}
	if delta < -0x80000000 {
		delta = -0x80000000
	}
	return w.sendMouse(0, 0, uint32(delta), flag)
}

func (w *winBackend) sendMouse(dx, dy int32, data, flags uint32) error {
	in := input{Type: inputMouse}
	in.Mi = mouseInput{DX: dx, DY: dy, MouseData: data, Flags: flags}
	return sendInputs([]input{in})
}

func downFlag(button string) (uint32, error) {
	switch button {
	case "left":
		return moUsefLeftDown, nil
	case "right":
		return moUsefRightDown, nil
	case "middle":
		return moUsefMiddleDown, nil
	default:
		return 0, fmt.Errorf("unsupported mouse button %q", button)
	}
}

func upFlag(button string) (uint32, error) {
	switch button {
	case "left":
		return moUsefLeftUp, nil
	case "right":
		return moUsefRightUp, nil
	case "middle":
		return moUsefMiddleUp, nil
	default:
		return 0, fmt.Errorf("unsupported mouse button %q", button)
	}
}

// sendInputs submits a batch atomically. A partial batch would leave the mouse
// in a pressed state, so a short count is treated as a hard failure.
func sendInputs(batch []input) error {
	if len(batch) == 0 {
		return nil
	}
	size := uintptr(unsafe.Sizeof(input{}))
	sent, _, err := procSendInput.Call(uintptr(len(batch)), uintptr(unsafe.Pointer(&batch[0])), size)
	if sent != uintptr(len(batch)) {
		return fmt.Errorf("SendInput sent %d/%d events: %w", sent, len(batch), err)
	}
	return nil
}

func (w *winBackend) TypeText(text string) error {
	runes := []rune(text)
	batch := make([]input, 0, len(runes)*4)
	emit := func(code uint16) {
		down := input{Type: inputKeyboard}
		down.setKeyboard(keyboardInput{Scan: code, Flags: keyEventfUnicode})
		up := input{Type: inputKeyboard}
		up.setKeyboard(keyboardInput{Scan: code, Flags: keyEventfUnicode | keyEventfKeyUp})
		batch = append(batch, down, up)
	}
	for _, r := range runes {
		// KEYEVENTF_UNICODE takes a UTF-16 code unit, so anything above the
		// BMP must be split into a surrogate pair. Chinese and most other
		// scripts stay in the BMP and go through as a single unit.
		if r > 0xFFFF {
			r -= 0x10000
			emit(uint16(0xD800 + (r >> 10)))
			emit(uint16(0xDC00 + (r & 0x3FF)))
			continue
		}
		emit(uint16(r))
	}
	if len(batch) == 0 {
		return nil
	}
	// Send in chunks: one giant INPUT array is a large stack allocation and
	// some targets drop bursts.
	const chunk = 128
	for i := 0; i < len(batch); i += chunk {
		end := i + chunk
		if end > len(batch) {
			end = len(batch)
		}
		if err := sendInputs(batch[i:end]); err != nil {
			return err
		}
		// A tiny gap between chunks keeps receiving apps from dropping events.
		time.Sleep(2 * time.Millisecond)
	}
	return nil
}

func (w *winBackend) PressKey(key string, mods Mods) error {
	vk, ok := namedKeys[strings.ToUpper(strings.TrimSpace(key))]
	if !ok {
		return fmt.Errorf("key %q is not in the allowed set", key)
	}
	var batch []input
	addMod := func(code uint16) {
		down := input{Type: inputKeyboard}
		down.setKeyboard(keyboardInput{Vk: code})
		up := input{Type: inputKeyboard}
		up.setKeyboard(keyboardInput{Vk: code, Flags: keyEventfKeyUp})
		batch = append(batch, down, up)
	}
	// Release in reverse order of press so a stuck modifier cannot survive.
	if mods.Ctrl {
		addMod(vkControl)
	}
	if mods.Alt {
		addMod(vkMenu)
	}
	if mods.Shift {
		addMod(vkShift)
	}
	keyDown := input{Type: inputKeyboard}
	keyDown.setKeyboard(keyboardInput{Vk: vk})
	keyUp := input{Type: inputKeyboard}
	keyUp.setKeyboard(keyboardInput{Vk: vk, Flags: keyEventfKeyUp})
	batch = append(batch, keyDown, keyUp)
	if mods.Shift {
		addMod(vkShift)
	}
	if mods.Alt {
		addMod(vkMenu)
	}
	if mods.Ctrl {
		addMod(vkControl)
	}
	return sendInputs(batch)
}

func (w *winBackend) Screenshot(rect Rect) (image.Image, error) {
	if rect.Width <= 0 || rect.Height <= 0 {
		return nil, fmt.Errorf("invalid capture rect %s", rect.String())
	}
	// GetDC(NULL) is the whole-screen DC, so a region capture is a BitBlt
	// with a source offset rather than a window DC.
	screenDC, _, _ := procGetDC.Call(0)
	if screenDC == 0 {
		return nil, fmt.Errorf("GetDC(NULL) failed")
	}
	defer procReleaseDC.Call(0, screenDC)

	memDC, _, _ := procCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC failed")
	}
	defer procDeleteDC.Call(memDC)

	bitmap, _, _ := procCreateCompatibleBitmap.Call(screenDC,
		uintptr(int32(rect.Width)), uintptr(int32(rect.Height)))
	if bitmap == 0 {
		return nil, fmt.Errorf("CreateCompatibleBitmap(%dx%d) failed", rect.Width, rect.Height)
	}
	defer procDeleteObject.Call(bitmap)

	old, _, _ := procSelectObject.Call(memDC, bitmap)
	if old != 0 {
		defer procSelectObject.Call(memDC, old)
	}

	// CAPTUREBLT is required to include layered/DWM-composited windows;
	// without it most modern windows come out blank.
	if ret, _, _ := procBitBlt.Call(memDC, 0, 0,
		uintptr(int32(rect.Width)), uintptr(int32(rect.Height)),
		screenDC, uintptr(int32(rect.X)), uintptr(int32(rect.Y)),
		srccopy|captureBlt); ret == 0 {
		return nil, fmt.Errorf("BitBlt failed for %s", rect.String())
	}

	var bi bitmapInfoHeader
	bi.Size = uint32(unsafe.Sizeof(bi))
	bi.Width = int32(rect.Width)
	bi.Height = -int32(rect.Height) // negative height = top-down rows
	bi.Planes = 1
	bi.BitCount = 32
	bi.Compression = biRGB

	stride := rect.Width * 4
	buf := make([]byte, stride*rect.Height)
	if ret, _, _ := procGetDIBits.Call(memDC, bitmap, 0, uintptr(rect.Height),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bi)), dibRGBColors); ret == 0 {
		return nil, fmt.Errorf("GetDIBits failed for %s", rect.String())
	}

	img := image.NewRGBA(image.Rect(0, 0, rect.Width, rect.Height))
	copy(img.Pix, buf)
	return img, nil
}
