// 系统托盘：注册托盘图标 + 右键菜单（编辑配置 / 重启 / 关于 / 退出）。
package tray

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
	"imeqiao/internal/assets"
	"imeqiao/internal/config"
	"imeqiao/internal/win32"
)

const (
	wmTrayIcon = win32.WM_USER + 1
	idmConfig  = 1001
	idmRestart = 1002
	idmAbout   = 1003
	idmExit    = 1004
)

// gOnRestart 由 main 注入：托盘"重启程序"在拉起新实例前调用，用于释放全局互斥体。
var gOnRestart func()

var trayWndProc = windows.NewCallback(func(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch uint32(msg) {
	case wmTrayIcon:
		if uint32(lParam) == win32.WM_RBUTTONUP {
			showContextMenu(hwnd)
		}
		return 0
	case win32.WM_COMMAND:
		id := uint32(wParam)
		switch id {
		case idmExit:
			win32.ProcPostQuitMessage.Call(0)
		case idmRestart:
			restartApp()
			win32.ProcPostQuitMessage.Call(0)
		case idmConfig:
			openConfig()
		case idmAbout:
			showAbout()
		}
		return 0
	case win32.WM_DESTROY:
		win32.ProcPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := win32.ProcDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return r
})

// TrayManager 托盘管理器
type TrayManager struct {
	hwnd win32.HWND
}

// New 创建托盘窗口并注册图标。onRestart 在"重启程序"时先于拉起新实例调用。
func New(onRestart func()) *TrayManager {
	gOnRestart = onRestart
	hInst, _, _ := win32.ProcGetModuleHandleW.Call(0)
	clsName, _ := windows.UTF16PtrFromString("IMETrayWindowClass")
	wc := win32.WNDCLASSEX{
		CbSize:      uint32(unsafe.Sizeof(win32.WNDCLASSEX{})),
		LpfnWndProc: trayWndProc,
		Instance:    win32.HINSTANCE(hInst),
		ClassName:   clsName,
	}
	if rc, _, _ := win32.ProcRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); rc == 0 {
		fmt.Fprintln(os.Stderr, "tray: RegisterClassExW 失败 (rc=0)")
	}

	title, _ := windows.UTF16PtrFromString("IME Indicator Tray")
	hwnd, _, _ := win32.ProcCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(clsName)),
		uintptr(unsafe.Pointer(title)),
		win32.WS_OVERLAPPEDWINDOW,
		uintptr(win32.CW_USEDEFAULT), uintptr(win32.CW_USEDEFAULT),
		uintptr(win32.CW_USEDEFAULT), uintptr(win32.CW_USEDEFAULT),
		0, 0,
		uintptr(hInst),
		0,
	)
	if hwnd == 0 {
		fmt.Fprintln(os.Stderr, "tray: CreateWindowExW 失败 (hwnd=0)")
	}

	icon := loadIcon()
	if icon == 0 {
		fmt.Fprintln(os.Stderr, "tray: loadIcon 返回 0 (无图标句柄)")
	}
	nid := win32.NOTIFYICONDATA{
		HWnd:             win32.HWND(hwnd),
		UID:              1,
		UFlags:           win32.NIF_ICON | win32.NIF_MESSAGE | win32.NIF_TIP,
		UCallbackMessage: wmTrayIcon,
		HIcon:            icon,
	}
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	copyTip(&nid)
	if r, _, _ := win32.ProcShellNotifyIconW.Call(win32.NIM_ADD, uintptr(unsafe.Pointer(&nid))); r == 0 {
		fmt.Fprintln(os.Stderr, "tray: Shell_NotifyIconW(NIM_ADD) 失败 (r=0)")
	}

	return &TrayManager{hwnd: win32.HWND(hwnd)}
}

func copyTip(nid *win32.NOTIFYICONDATA) {
	tip := windows.StringToUTF16("输入指示器 (IME Indicator)")
	n := len(tip)
	if n > len(nid.SzTip)-1 {
		n = len(nid.SzTip) - 1
	}
	copy(nid.SzTip[:n], tip[:n])
}

// RunMessageLoop 阻塞运行消息循环（主线程）
func (t *TrayManager) RunMessageLoop() {
	var msg win32.MSG
	for win32.CallOK(win32.ProcGetMessageW, uintptr(unsafe.Pointer(&msg)), 0, 0, 0) {
		win32.ProcTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		win32.ProcDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

// Destroy 删除托盘图标并销毁窗口
func (t *TrayManager) Destroy() {
	nid := win32.NOTIFYICONDATA{HWnd: t.hwnd, UID: 1}
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	win32.ProcShellNotifyIconW.Call(win32.NIM_DELETE, uintptr(unsafe.Pointer(&nid)))
	win32.ProcDestroyWindow.Call(uintptr(t.hwnd))
}

// loadIcon 优先 exe 同目录 icon.ico（LoadImageW 原生支持），失败回落嵌入资源 ID 1，
// 再失败回落系统默认应用图标。
func loadIcon() win32.HICON {
	if exe, err := os.Executable(); err == nil {
		icoPath := filepath.Join(filepath.Dir(exe), "icon.ico")
		if _, err := os.Stat(icoPath); err == nil {
			if h := loadIconFromFile(icoPath); h != 0 {
				return h
			}
		}
	}
	// 资源 ID 1（MAKEINTRESOURCE(1)，由 build.sh 经 rsrc 编入 .syso）
	hInst, _, _ := win32.ProcGetModuleHandleW.Call(0)
	r, _, _ := win32.ProcLoadImageW.Call(
		uintptr(hInst),
		uintptr(1),
		win32.IMAGE_ICON, 0, 0, win32.LR_DEFAULTCOLOR,
	)
	if r != 0 {
		return win32.HICON(r)
	}
	// 兜底：系统默认应用图标
	if r2, _, _ := win32.ProcLoadIconW.Call(0, uintptr(win32.IDI_APPLICATION)); r2 != 0 {
		return win32.HICON(r2)
	}
	return 0
}

func loadIconFromFile(path string) win32.HICON {
	p, _ := windows.UTF16PtrFromString(path)
	r, _, _ := win32.ProcLoadImageW.Call(
		0,
		uintptr(unsafe.Pointer(p)),
		win32.IMAGE_ICON, 0, 0,
		win32.LR_LOADFROMFILE|win32.LR_DEFAULTSIZE,
	)
	return win32.HICON(r)
}

func showContextMenu(hwnd win32.HWND) {
	menu, _, _ := win32.ProcCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	appendMenu := func(id uint32, text string) {
		t, _ := windows.UTF16PtrFromString(text)
		win32.ProcAppendMenuW.Call(menu, win32.MF_STRING, uintptr(id), uintptr(unsafe.Pointer(t)))
	}
	appendMenu(idmConfig, "编辑配置 (Config)")
	appendMenu(idmRestart, "重启程序 (Restart)")
	appendMenu(idmAbout, "关于 (About)")
	win32.ProcAppendMenuW.Call(menu, win32.MF_SEPARATOR, 0, 0)
	appendMenu(idmExit, "退出 (Exit)")

	var pos win32.POINT
	win32.ProcGetCursorPos.Call(uintptr(unsafe.Pointer(&pos)))
	win32.ProcSetForegroundWindow.Call(uintptr(hwnd))
	win32.ProcTrackPopupMenu.Call(
		menu, win32.TPM_LEFTALIGN|win32.TPM_BOTTOMALIGN,
		uintptr(pos.X), uintptr(pos.Y), 0, uintptr(hwnd), 0,
	)
	win32.ProcDestroyMenu.Call(menu)
}

func openConfig() {
	path := config.GetConfigPath()
	p, _ := windows.UTF16PtrFromString(path)
	win32.ProcShellExecuteW.Call(0, uintptr(unsafe.Pointer(mustUTF16Ptr("open"))), uintptr(unsafe.Pointer(p)), 0, 0, win32.SW_SHOW)
}

func showAbout() {
	content := assets.About
	if exe, err := os.Executable(); err == nil {
		aboutPath := filepath.Join(filepath.Dir(exe), "assets", "about.txt")
		if b, err := os.ReadFile(aboutPath); err == nil {
			content = string(b)
		}
	}
	title, _ := windows.UTF16PtrFromString("关于 输入指示器")
	text, _ := windows.UTF16PtrFromString(content)
	win32.ProcMessageBoxW.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), win32.MB_ICONINFORMATION|win32.MB_OK)
}

func restartApp() {
	if gOnRestart != nil {
		gOnRestart() // 先释放全局互斥体，否则新实例会因已在运行而自杀
	}
	var buf [512]uint16
	n, _ := windows.GetModuleFileName(windows.Handle(0), &buf[0], uint32(len(buf)))
	if n > 0 {
		p := uintptr(unsafe.Pointer(&buf[0]))
		win32.ProcShellExecuteW.Call(0, uintptr(unsafe.Pointer(mustUTF16Ptr("open"))), p, 0, 0, win32.SW_SHOW)
	}
}

func mustUTF16Ptr(s string) *uint16 {
	p, _ := windows.UTF16PtrFromString(s)
	return p
}
