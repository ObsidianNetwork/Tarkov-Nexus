//go:build windows

package autoscreenshot

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	processSnapshotFlag = 0x00000002
	inputKeyboard       = 1
	keyEventKeyUp       = 0x0002
)

type processEntry32 struct {
	Size              uint32
	Usage             uint32
	ProcessID         uint32
	DefaultHeapID     uintptr
	ModuleID          uint32
	Threads           uint32
	ParentProcessID   uint32
	PriorityClassBase int32
	Flags             uint32
	ExeFile           [windows.MAX_PATH]uint16
}

type keyboardInput struct {
	VirtualKey uint16
	ScanCode   uint16
	Flags      uint32
	Time       uint32
	ExtraInfo  uintptr
}

// INPUT contains the keyboard event in the first member of the native union.
// The 32-byte payload gives the union its Windows x64 size/alignment.
type input struct {
	Type    uint32
	Padding uint32
	Data    [32]byte
}

var (
	kernel32                     = windows.NewLazySystemDLL("kernel32.dll")
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procCreateToolhelpSnapshot   = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32First           = kernel32.NewProc("Process32FirstW")
	procProcess32Next            = kernel32.NewProc("Process32NextW")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	procSendInput                = user32.NewProc("SendInput")
	procGetAsyncKeyState         = user32.NewProc("GetAsyncKeyState")
)

type windowsDriver struct {
	mu         sync.Mutex
	cacheAt    time.Time
	cachedPIDs map[uint32]struct{}
}

func NewPlatformDriver() Driver { return &windowsDriver{} }

func (d *windowsDriver) GameRunning() (bool, error) {
	pids, err := d.gameProcessIDs()
	return len(pids) != 0, err
}

func (d *windowsDriver) GameForeground() (bool, error) {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return false, nil
	}
	var pid uint32
	threadID, _, _ := procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if threadID == 0 || pid == 0 {
		return false, nil
	}
	pids, err := d.gameProcessIDs()
	if err != nil {
		return false, err
	}
	_, ok := pids[pid]
	return ok, nil
}

func (d *windowsDriver) TapKey(vk uint16) error {
	if vk == 0 {
		return errors.New("no screenshot key is configured")
	}
	state, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
	if uint16(state)&0x8000 != 0 {
		return fmt.Errorf("virtual key %d is physically held down", vk)
	}
	down := makeInput(vk, false)
	up := makeInput(vk, true)
	inputs := [2]input{down, up}
	sent, _, callErr := procSendInput.Call(uintptr(len(inputs)), uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(input{}))
	if sent == uintptr(len(inputs)) {
		return nil
	}
	// If Windows accepted key-down but not key-up, make a best-effort release.
	if sent == 1 {
		procSendInput.Call(1, uintptr(unsafe.Pointer(&up)), unsafe.Sizeof(input{}))
	}
	if callErr != windows.ERROR_SUCCESS {
		return fmt.Errorf("SendInput accepted %d of %d events: %w (input can also be blocked by UIPI)", sent, len(inputs), callErr)
	}
	return fmt.Errorf("SendInput accepted %d of %d events (input can be blocked by UIPI)", sent, len(inputs))
}

func makeInput(vk uint16, keyUp bool) input {
	event := input{Type: inputKeyboard}
	keyboard := keyboardInput{VirtualKey: vk}
	if keyUp {
		keyboard.Flags = keyEventKeyUp
	}
	*(*keyboardInput)(unsafe.Pointer(&event.Data[0])) = keyboard
	return event
}

func (d *windowsDriver) gameProcessIDs() (map[uint32]struct{}, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if time.Since(d.cacheAt) < time.Second && d.cachedPIDs != nil {
		return d.cachedPIDs, nil
	}
	processes, err := enumerateProcesses()
	if err != nil {
		return nil, err
	}
	d.cachedPIDs = processes
	d.cacheAt = time.Now()
	return processes, nil
}

func enumerateProcesses() (map[uint32]struct{}, error) {
	handle, _, callErr := procCreateToolhelpSnapshot.Call(processSnapshotFlag, 0)
	if handle == uintptr(windows.InvalidHandle) {
		return nil, fmt.Errorf("CreateToolhelp32Snapshot: %w", callErr)
	}
	defer windows.CloseHandle(windows.Handle(handle))

	entry := processEntry32{Size: uint32(unsafe.Sizeof(processEntry32{}))}
	ok, _, callErr := procProcess32First.Call(handle, uintptr(unsafe.Pointer(&entry)))
	if ok == 0 {
		return nil, fmt.Errorf("Process32FirstW: %w", callErr)
	}
	result := make(map[uint32]struct{})
	for {
		if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), "EscapeFromTarkov.exe") {
			result[entry.ProcessID] = struct{}{}
		}
		entry.Size = uint32(unsafe.Sizeof(processEntry32{}))
		ok, _, callErr = procProcess32Next.Call(handle, uintptr(unsafe.Pointer(&entry)))
		if ok == 0 {
			if callErr == windows.ERROR_NO_MORE_FILES {
				break
			}
			return nil, fmt.Errorf("Process32NextW: %w", callErr)
		}
	}
	return result, nil
}
