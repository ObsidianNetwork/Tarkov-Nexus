//go:build !windows

package mapwindow

func FindWindow(className string) (HWND, error)        { return 0, ErrUnsupported }
func WindowRect(h HWND) (Rect, error)                  { return Rect{}, ErrUnsupported }
func WindowDPI(h HWND) (uint32, error)                 { return 0, ErrUnsupported }
func IsMinimised(h HWND) (bool, error)                 { return false, ErrUnsupported }
func SetWindowRect(h HWND, r Rect) error               { return ErrUnsupported }
func Monitors() (rects []Rect, primary int, err error) { return nil, 0, ErrUnsupported }
