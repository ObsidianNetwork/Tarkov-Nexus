//go:build windows

package mapwindow

import (
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procFindWindowExW       = user32.NewProc("FindWindowExW")
	procGetWindowProcessID  = user32.NewProc("GetWindowThreadProcessId")
	procGetWindowRect       = user32.NewProc("GetWindowRect")
	procGetDpiForWindow     = user32.NewProc("GetDpiForWindow")
	procIsIconic            = user32.NewProc("IsIconic")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
	procEnumDisplayMonitors = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW     = user32.NewProc("GetMonitorInfoW")
)

const (
	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010

	monitorInfoFPrimary = 0x1
)

type w32Rect struct {
	Left, Top, Right, Bottom int32
}

func (r w32Rect) toRect() Rect {
	return Rect{X: int(r.Left), Y: int(r.Top), W: int(r.Right - r.Left), H: int(r.Bottom - r.Top)}
}

type monitorInfo struct {
	CbSize    uint32
	RcMonitor w32Rect
	RcWork    w32Rect
	DwFlags   uint32
}

// FindWindow returns this process's top-level window registered under className.
func FindWindow(className string) (HWND, error) {
	cls, err := windows.UTF16PtrFromString(className)
	if err != nil {
		return 0, err
	}
	pid := windows.GetCurrentProcessId()
	var h uintptr
	for {
		h, _, _ = procFindWindowExW.Call(0, h, uintptr(unsafe.Pointer(cls)), 0)
		if h == 0 {
			return 0, fmt.Errorf("mapwindow: no window with class %q in process %d", className, pid)
		}
		var owner uint32
		thread, _, _ := procGetWindowProcessID.Call(h, uintptr(unsafe.Pointer(&owner)))
		if thread != 0 && owner == pid {
			return HWND(h), nil
		}
	}
}

// WindowRect returns the window's outer rectangle in physical screen pixels.
func WindowRect(h HWND) (Rect, error) {
	var r w32Rect
	ok, _, e := procGetWindowRect.Call(uintptr(h), uintptr(unsafe.Pointer(&r)))
	if ok == 0 {
		return Rect{}, fmt.Errorf("mapwindow: GetWindowRect: %w", e)
	}
	return r.toRect(), nil
}

// WindowDPI returns the owned window's current per-monitor pixel scale.
func WindowDPI(h HWND) (uint32, error) {
	if err := procGetDpiForWindow.Find(); err != nil {
		return 0, fmt.Errorf("mapwindow: GetDpiForWindow unavailable: %w", err)
	}
	dpi, _, _ := procGetDpiForWindow.Call(uintptr(h))
	if dpi == 0 {
		return 0, fmt.Errorf("mapwindow: GetDpiForWindow returned no DPI for window %#x", h)
	}
	return uint32(dpi), nil
}

// IsMinimised reports whether the window is iconic (minimised).
func IsMinimised(h HWND) (bool, error) {
	v, _, _ := procIsIconic.Call(uintptr(h))
	return v != 0, nil
}

// SetWindowRect moves and resizes the window without changing z-order or
// stealing focus.
func SetWindowRect(h HWND, r Rect) error {
	ok, _, e := procSetWindowPos.Call(
		uintptr(h), 0,
		uintptr(int32(r.X)), uintptr(int32(r.Y)), uintptr(int32(r.W)), uintptr(int32(r.H)),
		swpNoZOrder|swpNoActivate,
	)
	if ok == 0 {
		return fmt.Errorf("mapwindow: SetWindowPos: %w", e)
	}
	return nil
}

// Monitors returns every monitor's work area (taskbar excluded) and the index
// of the primary monitor.
//
// The enumeration callback is registered once: windows.NewCallback allocates
// a process-lifetime slot from a small fixed pool and never frees it, so
// creating one per call would eventually exhaust the pool and panic.
func Monitors() (rects []Rect, primary int, err error) {
	enumMu.Lock()
	defer enumMu.Unlock()

	enumState = enumScan{primary: -1}
	enumOnce.Do(func() { enumCB = windows.NewCallback(enumMonitor) })
	ok, _, e := procEnumDisplayMonitors.Call(0, 0, enumCB, 0)
	if ok == 0 {
		return nil, 0, fmt.Errorf("mapwindow: EnumDisplayMonitors: %w", e)
	}
	if len(enumState.rects) == 0 {
		return nil, 0, fmt.Errorf("mapwindow: no monitors enumerated")
	}
	primary = enumState.primary
	if primary < 0 {
		primary = 0
	}
	return enumState.rects, primary, nil
}

type enumScan struct {
	rects   []Rect
	primary int
}

var (
	enumMu    sync.Mutex // serialises Monitors and guards enumState
	enumState enumScan
	enumOnce  sync.Once
	enumCB    uintptr
)

// enumMonitor is the EnumDisplayMonitors callback. It runs on the calling
// thread, inside the procEnumDisplayMonitors.Call, while enumMu is held.
func enumMonitor(hMonitor, hdc uintptr, lprc *w32Rect, lparam uintptr) uintptr {
	mi := monitorInfo{CbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
	ok, _, _ := procGetMonitorInfoW.Call(hMonitor, uintptr(unsafe.Pointer(&mi)))
	if ok == 0 {
		return 1 // skip this monitor, keep enumerating
	}
	if mi.DwFlags&monitorInfoFPrimary != 0 {
		enumState.primary = len(enumState.rects)
	}
	enumState.rects = append(enumState.rects, mi.RcWork.toRect())
	return 1
}
