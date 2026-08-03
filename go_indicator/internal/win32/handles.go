package win32

import "golang.org/x/sys/windows"

// 句柄类型统一用 uintptr 别名，便于与 LazyProc 的参数互转。
type (
	HANDLE    = uintptr
	HWND      = uintptr
	HINSTANCE = uintptr
	HICON     = uintptr
	HMENU     = uintptr
	HDC       = uintptr
	HBITMAP   = uintptr
	HBRUSH    = uintptr
	HCURSOR   = uintptr
	HIMC      = uintptr
	HMODULE   = uintptr
	HGDIOBJ   = uintptr
)

// GUID 复用 x/sys/windows 的定义（CoCreateInstance 等需要）
type GUID = windows.GUID

// POINT / RECT / SIZE
type POINT struct {
	X, Y int32
}
type RECT struct {
	Left, Top, Right, Bottom int32
}
type SIZE struct {
	CX, CY int32
}

// MSG（与 Win32 MSG 布局一致）
type MSG struct {
	Hwnd    HWND
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}

// WNDCLASSEX
type WNDCLASSEX struct {
	CbSize      uint32
	Style       uint32
	LpfnWndProc uintptr
	ClsExtra    int32
	WndClsExtra int32
	Instance    HINSTANCE
	Icon        HICON
	Cursor      HCURSOR
	Background  HBRUSH
	MenuName    *uint16
	ClassName   *uint16
	IconSm      HICON
}

// CURSORINFO
type CURSORINFO struct {
	CbSize      uint32
	Flags       uint32
	HCursor     HCURSOR
	PtScreenPos POINT
}

// GUITHREADINFO
type GUITHREADINFO struct {
	CbSize        uint32
	Flags         uint32
	HwndActive    HWND
	HwndFocus     HWND
	HwndCapture   HWND
	HwndMenuOwner HWND
	HwndMoveSize  HWND
	HwndCaret     HWND
	RcRect        RECT
}

// BITMAPINFOHEADER / BITMAPINFO
type BITMAPINFOHEADER struct {
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
type BITMAPINFO struct {
	Header BITMAPINFOHEADER
	// Colors 占位（本项目仅用 Header）
	Colors [1]struct{ B, G, R, A byte }
}
