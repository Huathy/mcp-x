//go:build !windows

package desktop

import (
	"errors"
	"image"
)

func platformAvailable() bool { return false }

func newBackend(dpiAware bool) (Backend, error) {
	return unsupportedBackend{}, nil
}

type unsupportedBackend struct{}

func (unsupportedBackend) ScreenInfo() (ScreenInfo, error) {
	return ScreenInfo{}, ErrUnsupportedPlatform
}

func (unsupportedBackend) ListWindows() ([]WindowInfo, error) {
	return nil, ErrUnsupportedPlatform
}

func (unsupportedBackend) WindowAtPoint(x, y int) (WindowInfo, bool, error) {
	return WindowInfo{}, false, ErrUnsupportedPlatform
}

func (unsupportedBackend) SetForeground(hwnd uintptr) error { return ErrUnsupportedPlatform }

func (unsupportedBackend) CursorPos() (int, int, error) {
	return 0, 0, ErrUnsupportedPlatform
}

func (unsupportedBackend) MoveTo(x, y int) error { return ErrUnsupportedPlatform }

func (unsupportedBackend) MouseDown(button string) error { return ErrUnsupportedPlatform }

func (unsupportedBackend) MouseUp(button string) error { return ErrUnsupportedPlatform }

func (unsupportedBackend) MouseWheel(horizontal bool, notches int) error {
	return ErrUnsupportedPlatform
}

func (unsupportedBackend) TypeText(text string) error { return ErrUnsupportedPlatform }

func (unsupportedBackend) PressKey(key string, mods Mods) error { return ErrUnsupportedPlatform }

func (unsupportedBackend) Screenshot(rect Rect) (image.Image, error) {
	return nil, errors.Join(ErrUnsupportedPlatform, errNoSession)
}

// errNoSession is kept separate so the message can mention both causes.
var errNoSession = errors.New("需要 Windows 交互式桌面会话（Session 1）")
