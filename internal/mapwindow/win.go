package mapwindow

import "errors"

// HWND is a native window handle. Zero is never a valid window.
type HWND uintptr

// ErrUnsupported is returned by every native call on non-Windows builds.
var ErrUnsupported = errors.New("mapwindow: native window placement is only supported on Windows")

// PlaceExactly is SetWindowRect with a one-shot read-back. Moving onto a
// monitor with a different DPI can raise WM_DPICHANGED, which Wails answers
// with Windows' suggested (rescaled) rect; a second SetWindowPos on the final
// monitor sticks. Cheap insurance; a no-op when the first call already held.
func PlaceExactly(h HWND, want Rect) error {
	if err := SetWindowRect(h, want); err != nil {
		return err
	}
	got, err := WindowRect(h)
	if err != nil || got == want {
		return err
	}
	return SetWindowRect(h, want)
}
