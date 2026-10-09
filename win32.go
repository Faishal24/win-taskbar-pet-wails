package main

import (
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// All Win32 access goes through lazy-loaded system DLLs — no CGO, no MSVC,
// no extra dependencies. Mirrors the old koffi host.ts surface.

var (
	modUser32  = windows.NewLazySystemDLL("user32.dll")
	modShell32 = windows.NewLazySystemDLL("shell32.dll")

	procFindWindowW                  = modUser32.NewProc("FindWindowW")
	procSetWindowPos                 = modUser32.NewProc("SetWindowPos")
	procSetWindowLongPtrW            = modUser32.NewProc("SetWindowLongPtrW")
	procGetWindowLongPtrW            = modUser32.NewProc("GetWindowLongPtrW")
	procGetWindowRect                = modUser32.NewProc("GetWindowRect")
	procGetCursorPos                 = modUser32.NewProc("GetCursorPos")
	procGetAsyncKeyState             = modUser32.NewProc("GetAsyncKeyState")
	procGetSystemMetrics             = modUser32.NewProc("GetSystemMetrics")
	procSetClassLongPtrW            = modUser32.NewProc("SetClassLongPtrW")
	procInvalidateRect              = modUser32.NewProc("InvalidateRect")
	procEnumWindows                 = modUser32.NewProc("EnumWindows")
	procGetWindowThreadProcessId    = modUser32.NewProc("GetWindowThreadProcessId")
	procGetClassNameW               = modUser32.NewProc("GetClassNameW")
	procSHQueryUserNotificationState = modShell32.NewProc("SHQueryUserNotificationState")

	modOle32             = windows.NewLazySystemDLL("ole32.dll")
	procCoInitializeEx   = modOle32.NewProc("CoInitializeEx")
	procCoCreateInstance = modOle32.NewProc("CoCreateInstance")
	procCoUninitialize   = modOle32.NewProc("CoUninitialize")
)

const (
	wsExTransparent = 0x00000020
	wsExToolwindow  = 0x00000080
	wsExNoactivate  = 0x08000000
	wsExLayered     = 0x00080000
	gwlExstyle      = -20

	hwndTopmost = ^uintptr(0) // (HWND)-1

	swpNoMove      = 0x0002
	swpNoSize      = 0x0001
	swpNoActivate  = 0x0010
	swpShowWindow  = 0x0040
	swpNoZOrder    = 0x0004
	swpFrameChange = 0x0020

	vkLButton = 1
	vkRButton = 2

	smCxScreen = 0
	smCyScreen = 1
	smXVirtualScreen = 76
	smYVirtualScreen = 77
	smCxVirtualScreen = 78
	smCyVirtualScreen = 79
)

type winPoint struct{ X, Y int32 }
type winRect struct{ Left, Top, Right, Bottom int32 }

func u32neg(v int32) uintptr { return uintptr(uint32(v)) }

func findWindow(className string) uintptr {
	c, err := windows.UTF16PtrFromString(className)
	if err != nil {
		return 0
	}
	r, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(c)), 0)
	return r
}

// findWindowByTitle matches lpWindowName (class NULL). Wails names the
// window by title, not by class — this is the one we want.
const (
	gclpHbrBackground = -10
)

// clearClassBackground removes the window class background brush (wails
// paints it opaque: white by default, or whatever BackgroundColour was
// set). With no background to paint, DWM composites the window surface
// with per-pixel alpha and the transparent webview shows the desktop.
func clearClassBackground(hwnd uintptr) {
	procSetClassLongPtrW.Call(hwnd, u32neg(gclpHbrBackground), 0)
	procInvalidateRect.Call(hwnd, 0, 1)
}

// deleteTaskbarTab removes our window's button from the taskbar via
// ITaskbarList. WS_EX_TOOLWINDOW alone does not do it on Win11 (the button
// appears anyway) — this is the canonical removal. Idempotent: safe to
// call repeatedly (e.g. after every show, Explorer re-adds buttons on
// show cycles).
func deleteTaskbarTab(hwnd uintptr) {
	const COINIT_APARTMENTTHREADED = 0x2
	const CLSCTX_INPROC_SERVER = 0x1
	var clsid = windows.GUID{
		Data1: 0x56FDF344, Data2: 0xFD6D, Data3: 0x11D0,
		Data4: [8]byte{0x95, 0x8A, 0x00, 0x60, 0x97, 0xC9, 0xA0, 0x90},
	}
	var iid = windows.GUID{
		Data1: 0x56FDF342, Data2: 0xFD6D, Data3: 0x11D0,
		Data4: [8]byte{0x95, 0x8A, 0x00, 0x60, 0x97, 0xC9, 0xA0, 0x90},
	}
	procCoInitializeEx.Call(0, COINIT_APARTMENTTHREADED)
	var obj uintptr
	r, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsid)), 0, CLSCTX_INPROC_SERVER,
		uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&obj)))
	if r != 0 || obj == 0 {
		procCoUninitialize.Call()
		return
	}
	vtbl := *(**[8]uintptr)(unsafe.Pointer(obj))
	syscall.SyscallN(vtbl[3], obj)          // HrInit
	syscall.SyscallN(vtbl[5], obj, hwnd)    // DeleteTab
	syscall.SyscallN(vtbl[2], obj)          // Release
	procCoUninitialize.Call()
}

// enumWindows calls fn for each top-level window; return false to stop.
func enumWindows(fn func(hwnd uintptr) bool) {
	cb := windows.NewCallback(func(h, _ uintptr) uintptr {
		if fn(h) {
			return 1
		}
		return 0
	})
	procEnumWindows.Call(cb, 0)
}

func windowPID(hwnd uintptr) int {
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return int(pid)
}

func windowClass(hwnd uintptr) string {
	var buf [128]uint16
	n, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), 128)
	if n == 0 {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

// ownMainWindow finds our overlay window without depending on the native
// window title (UIA names can reflect the document title instead). It
// matches our own PID plus the wailsWindow class, falling back to the
// largest top-level window we own (systray helper windows are tiny).
func ownMainWindow() uintptr {
	self := os.Getpid()
	var found, biggest uintptr
	var biggestArea int32
	enumWindows(func(h uintptr) bool {
		if windowPID(h) != self {
			return true
		}
		if windowClass(h) == "wailsWindow" {
			found = h
			return false
		}
		if r, ok := windowRect(h); ok {
			if area := (r.Right - r.Left) * (r.Bottom - r.Top); area > biggestArea {
				biggestArea, biggest = area, h
			}
		}
		return true
	})
	if found != 0 {
		return found
	}
	return biggest
}

func findWindowByTitle(title string) uintptr {
	t, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return 0
	}
	r, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(t)))
	return r
}

func setExStyle(hwnd uintptr, ex uintptr) {
	procSetWindowLongPtrW.Call(hwnd, u32neg(gwlExstyle), ex)
}

// applyBaseStyles makes the overlay behave: no taskbar button, never takes
// focus, click-through by default (backend toggles transparency on hover).
func applyBaseStyles(hwnd uintptr) {
	ex, _, _ := procGetWindowLongPtrW.Call(hwnd, u32neg(gwlExstyle), 0)
	ex |= wsExToolwindow | wsExNoactivate | wsExTransparent | wsExLayered
	procSetWindowLongPtrW.Call(hwnd, u32neg(gwlExstyle), ex)
	// Re-assert frame so the style change applies immediately.
	procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoZOrder|swpFrameChange)
}

func setTransparent(hwnd uintptr, on bool) {
	ex, _, _ := procGetWindowLongPtrW.Call(hwnd, u32neg(gwlExstyle), 0)
	if on {
		ex |= wsExTransparent
	} else {
		ex &^= wsExTransparent
	}
	procSetWindowLongPtrW.Call(hwnd, u32neg(gwlExstyle), ex)
}

func placeWindow(hwnd uintptr, x, y, w, h int32) {
	procSetWindowPos.Call(hwnd, hwndTopmost,
		uintptr(uint32(x)), uintptr(uint32(y)), uintptr(uint32(w)), uintptr(uint32(h)),
		swpNoActivate|swpShowWindow)
}

// moveTop re-inserts the window at the top of the topmost band without
// moving/resizing it. Taskbar interactions sink us behind the opaque
// taskbar; this undoes it ( polled — see watchLoop ).
func moveTop(hwnd uintptr) {
	procSetWindowPos.Call(hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
}

func windowRect(hwnd uintptr) (winRect, bool) {
	var r winRect
	ret, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return r, ret != 0
}

func cursorPos() (int32, int32, bool) {
	var p winPoint
	ret, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return p.X, p.Y, ret != 0
}

func buttonDown(vk int) bool {
	r, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
	return r&0x8000 != 0
}

func screenSize() (int32, int32) {
	w, _, _ := procGetSystemMetrics.Call(smCxScreen)
	h, _, _ := procGetSystemMetrics.Call(smCyScreen)
	return int32(w), int32(h)
}

// virtualScreen spans all monitors (for the roam-everywhere strip).
func virtualScreen() (x, y, w, h int32) {
	vx, _, _ := procGetSystemMetrics.Call(smXVirtualScreen)
	vy, _, _ := procGetSystemMetrics.Call(smYVirtualScreen)
	vw, _, _ := procGetSystemMetrics.Call(smCxVirtualScreen)
	vh, _, _ := procGetSystemMetrics.Call(smCyVirtualScreen)
	return int32(vx), int32(vy), int32(vw), int32(vh)
}

// QueryUserNotificationState values that cover the screen.
const (
	qunsRunningD3DFullScreen = 3
	qunsPresentationMode    = 4
)

// fullscreenActive reports whether a fullscreen game/video or slideshow is
// running (canonical shell query — no fragile rect comparisons).
func fullscreenActive() bool {
	var state int32
	r, _, _ := procSHQueryUserNotificationState.Call(uintptr(unsafe.Pointer(&state)))
	if r != 0 {
		return false
	}
	return state == qunsRunningD3DFullScreen || state == qunsPresentationMode
}
