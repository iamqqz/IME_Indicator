// Go 调 Win32 的懒加载 DLL 与过程声明集中处。
// 仅声明本项目实际用到的 API，签名按本项目需要定义。过程名已导出供其他包调用。
package win32

import "golang.org/x/sys/windows"

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	imm32    = windows.NewLazySystemDLL("imm32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	shcore   = windows.NewLazySystemDLL("shcore.dll")
	oleacc   = windows.NewLazySystemDLL("oleacc.dll")
	oleaut32 = windows.NewLazySystemDLL("oleaut32.dll")
	ole32    = windows.NewLazySystemDLL("ole32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
)

var (
	// user32
	ProcGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	ProcGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	ProcGetGUIThreadInfo         = user32.NewProc("GetGUIThreadInfo")
	ProcClientToScreen           = user32.NewProc("ClientToScreen")
	ProcCreateWindowExW          = user32.NewProc("CreateWindowExW")
	ProcRegisterClassExW         = user32.NewProc("RegisterClassExW")
	ProcDefWindowProcW           = user32.NewProc("DefWindowProcW")
	ProcShowWindow               = user32.NewProc("ShowWindow")
	ProcSetWindowPos             = user32.NewProc("SetWindowPos")
	ProcUpdateLayeredWindow      = user32.NewProc("UpdateLayeredWindow")
	ProcGetMessageW              = user32.NewProc("GetMessageW")
	ProcTranslateMessage         = user32.NewProc("TranslateMessage")
	ProcDispatchMessageW         = user32.NewProc("DispatchMessageW")
	ProcPeekMessageW             = user32.NewProc("PeekMessageW")
	ProcPostQuitMessage          = user32.NewProc("PostQuitMessage")
	ProcDestroyWindow            = user32.NewProc("DestroyWindow")
	ProcGetCursorPos             = user32.NewProc("GetCursorPos")
	ProcGetCursorInfo            = user32.NewProc("GetCursorInfo")
	ProcLoadCursorW              = user32.NewProc("LoadCursorW")
	ProcCreatePopupMenu          = user32.NewProc("CreatePopupMenu")
	ProcAppendMenuW              = user32.NewProc("AppendMenuW")
	ProcTrackPopupMenu           = user32.NewProc("TrackPopupMenu")
	ProcSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	ProcDestroyMenu              = user32.NewProc("DestroyMenu")
	ProcMessageBoxW              = user32.NewProc("MessageBoxW")
	ProcLoadImageW               = user32.NewProc("LoadImageW")
	ProcLoadIconW                = user32.NewProc("LoadIconW")
	ProcSetProcessDPIAware       = user32.NewProc("SetProcessDPIAware")
	ProcSendMessageTimeoutW      = user32.NewProc("SendMessageTimeoutW")
	ProcGetWindowTextW           = user32.NewProc("GetWindowTextW")
	ProcGetWindowTextLengthW     = user32.NewProc("GetWindowTextLengthW")
	// GetDC / ReleaseDC 由 user32.dll 导出（gdi32 不含，懒加载会 panic）
	ProcGetDC                      = user32.NewProc("GetDC")
	ProcReleaseDC                  = user32.NewProc("ReleaseDC")
	ProcOpenProcess                = kernel32.NewProc("OpenProcess")
	ProcQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
	ProcCloseHandle                = kernel32.NewProc("CloseHandle")

	// gdi32 (注意：GetDC / ReleaseDC 由 user32.dll 导出，见上方 user32 段)
	ProcCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	ProcCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	ProcSelectObject       = gdi32.NewProc("SelectObject")
	ProcDeleteObject       = gdi32.NewProc("DeleteObject")
	ProcDeleteDC           = gdi32.NewProc("DeleteDC")

	// imm32
	ProcImmGetDefaultIMEWnd     = imm32.NewProc("ImmGetDefaultIMEWnd")
	ProcImmGetContext           = imm32.NewProc("ImmGetContext")
	ProcImmReleaseContext       = imm32.NewProc("ImmReleaseContext")
	ProcImmGetCompositionWindow = imm32.NewProc("ImmGetCompositionWindow")

	// shell32
	ProcShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	ProcShellExecuteW    = shell32.NewProc("ShellExecuteW")

	// shcore
	ProcSetProcessDpiAwareness = shcore.NewProc("SetProcessDpiAwareness")

	// oleacc
	ProcAccessibleObjectFromWindow = oleacc.NewProc("AccessibleObjectFromWindow")

	// oleaut32
	ProcSafeArrayGetLBound    = oleaut32.NewProc("SafeArrayGetLBound")
	ProcSafeArrayGetUBound    = oleaut32.NewProc("SafeArrayGetUBound")
	ProcSafeArrayAccessData   = oleaut32.NewProc("SafeArrayAccessData")
	ProcSafeArrayUnaccessData = oleaut32.NewProc("SafeArrayUnaccessData")
	ProcSafeArrayDestroy      = oleaut32.NewProc("SafeArrayDestroy")
	ProcSafeArrayGetElement   = oleaut32.NewProc("SafeArrayGetElement")

	// ole32 (COM)
	ProcCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	ProcCoCreateInstance = ole32.NewProc("CoCreateInstance")
	ProcCoUninitialize   = ole32.NewProc("CoUninitialize")

	// kernel32
	ProcCreateMutexW     = kernel32.NewProc("CreateMutexW")
	ProcGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

// CallR 调用过程并返回第一个返回值（r1）。
func CallR(proc *windows.LazyProc, args ...uintptr) uintptr {
	r, _, _ := proc.Call(args...)
	return r
}

// CallOK 调用过程并返回是否成功（r1 != 0）。
func CallOK(proc *windows.LazyProc, args ...uintptr) bool {
	r, _, _ := proc.Call(args...)
	return r != 0
}
