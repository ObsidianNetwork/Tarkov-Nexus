//go:build windows

package mapwindow

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestFindWindow_RejectsOtherProcess(t *testing.T) {
	// Given a matching hidden window owned only by a helper process.
	class := "MapWindowTest-" + filepath.Base(filepath.Dir(t.TempDir()))
	foreign := startWindowHelper(t, class)

	// When resolving the class from this process.
	got, err := FindWindow(class)

	// Then the foreign window cannot be used for placement or polling.
	if got != 0 || err == nil {
		t.Fatalf("FindWindow = (%#x, %v), want missing local window; foreign=%#x", got, err, foreign)
	}
}

func TestFindWindow_SelectsCurrentProcess(t *testing.T) {
	// Given local and foreign windows with the same class, foreign first.
	class := "MapWindowTest-" + filepath.Base(filepath.Dir(t.TempDir()))
	local := createHiddenWindow(t, class)
	foreign := startWindowHelper(t, class)
	cls, err := windows.UTF16PtrFromString(class)
	if err != nil {
		t.Fatal(err)
	}
	first, _, _ := user32.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(cls)), 0)
	if HWND(first) != foreign {
		t.Fatalf("fixture must put foreign window first: got %#x, want %#x", first, foreign)
	}

	// When resolving this process's map window.
	got, err := FindWindow(class)

	// Then the search continues past the foreign match to the local window.
	if err != nil || got != local {
		t.Fatalf("FindWindow = (%#x, %v), want local %#x; foreign=%#x", got, err, local, foreign)
	}
}

func TestMapWindowHelperProcess(t *testing.T) {
	class := os.Getenv("MAPWINDOW_TEST_CLASS")
	if class == "" {
		return
	}
	hwnd := createHiddenWindow(t, class)
	if _, err := fmt.Fprintln(os.Stdout, hwnd); err != nil {
		t.Fatal(err)
	}
	var stop [1]byte
	if _, err := os.Stdin.Read(stop[:]); err != nil {
		t.Fatal(err)
	}
}

func startWindowHelper(t *testing.T, class string) HWND {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMapWindowHelperProcess$")
	cmd.Env = append(os.Environ(), "MAPWINDOW_TEST_CLASS="+class)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := stdin.Write([]byte{1}); err != nil {
			t.Error(err)
		}
		if err := stdin.Close(); err != nil {
			t.Error(err)
		}
		if err := cmd.Wait(); err != nil {
			t.Errorf("window helper: %v", err)
		}
	})
	var hwnd HWND
	if _, err := fmt.Fscanln(stdout, &hwnd); err != nil {
		t.Fatalf("window helper readiness: %v", err)
	}
	return hwnd
}

type windowClass struct {
	style      uint32
	wndProc    uintptr
	classExtra int32
	winExtra   int32
	instance   windows.Handle
	icon       windows.Handle
	cursor     windows.Handle
	background windows.Handle
	menuName   *uint16
	className  *uint16
}

func createHiddenWindow(t *testing.T, class string) HWND {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	cls, err := windows.UTF16PtrFromString(class)
	if err != nil {
		t.Fatal(err)
	}
	var instance windows.Handle
	if err := windows.GetModuleHandleEx(windows.GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT, nil, &instance); err != nil {
		t.Fatal(err)
	}
	wc := windowClass{wndProc: user32.NewProc("DefWindowProcW").Addr(), instance: instance, className: cls}
	atom, _, callErr := user32.NewProc("RegisterClassW").Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		t.Fatalf("RegisterClassW: %v", callErr)
	}
	t.Cleanup(func() {
		ok, _, err := user32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(cls)), uintptr(instance))
		if ok == 0 {
			t.Errorf("UnregisterClassW: %v", err)
		}
	})
	hwnd, _, callErr := user32.NewProc("CreateWindowExW").Call(
		0x08000000, uintptr(unsafe.Pointer(cls)), 0, 0, 0, 0, 10, 10, 0, 0, uintptr(instance), 0,
	)
	if hwnd == 0 {
		t.Fatalf("CreateWindowExW: %v", callErr)
	}
	t.Cleanup(func() {
		ok, _, err := user32.NewProc("DestroyWindow").Call(hwnd)
		if ok == 0 {
			t.Errorf("DestroyWindow: %v", err)
		}
	})
	return HWND(hwnd)
}
