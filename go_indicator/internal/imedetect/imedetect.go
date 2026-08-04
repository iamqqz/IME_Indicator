// IME 中英状态检测与设置（IMM32 API）。
// get 逻辑与 rust_indicator/ime_detector.rs 一致；set 逻辑来自 ime_bridge.go。
package imedetect

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"imeqiao/internal/logging"
	"imeqiao/internal/win32"
)

func getForegroundWindow() win32.HWND {
	r, _, _ := win32.ProcGetForegroundWindow.Call()
	return win32.HWND(r)
}

func getWindowThreadProcessId(hwnd win32.HWND) (tid, pid uint32) {
	var p uint32
	tidR, _, _ := win32.ProcGetWindowThreadProcessId.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&p)))
	return uint32(tidR), p
}

// getFocusedWindow 返回当前焦点窗口（优先 GUITHREADINFO 的 focus/active）。
func getFocusedWindow() win32.HWND {
	fore := getForegroundWindow()
	if fore == 0 {
		return 0
	}
	_, pid := getWindowThreadProcessId(fore)
	_ = pid
	var gui win32.GUITHREADINFO
	gui.CbSize = uint32(unsafe.Sizeof(gui))
	if win32.CallOK(win32.ProcGetGUIThreadInfo, 0, uintptr(unsafe.Pointer(&gui))) {
		if gui.HwndFocus != 0 {
			return gui.HwndFocus
		}
		if gui.HwndActive != 0 {
			return gui.HwndActive
		}
	}
	return fore
}

func getIMEWindow(hwnd win32.HWND) win32.HWND {
	r, _, _ := win32.ProcImmGetDefaultIMEWnd.Call(uintptr(hwnd))
	return win32.HWND(r)
}

// sendImeMessage 发送 WM_IME_CONTROL 并返回 (是否成功, 结果值)
func sendImeMessage(imeWnd win32.HWND, command uintptr, lparam uintptr) (bool, int) {
	var result uintptr
	r, _, _ := win32.ProcSendMessageTimeoutW.Call(
		uintptr(imeWnd),
		win32.WM_IME_CONTROL,
		command,
		lparam,
		win32.SMTO_ABORTIFHUNG,
		500,
		uintptr(unsafe.Pointer(&result)),
	)
	if r != 0 {
		return true, int(result)
	}
	return false, 0
}

// IsChineseMode 返回 true 表示当前为中文输入模式
func IsChineseMode() bool {
	hwnd := getFocusedWindow()
	if hwnd == 0 {
		logging.Debug("IsChineseMode: 无前台/焦点窗口")
		return false
	}
	imeWnd := getIMEWindow(hwnd)
	if imeWnd == 0 {
		logging.Debug("IsChineseMode: 无 IME 窗口（非输入焦点，视为英文）")
		return false
	}
	ok, open := sendImeMessage(imeWnd, win32.IMC_GETOPENSTATUS, 0)
	if !ok {
		// 超时/失败：与"确实英文(open==0)"区分，避免排查时误判
		logging.Debug("IsChineseMode: IMC_GETOPENSTATUS 超时/失败")
		return false
	}
	if open == 0 {
		return false // 真正英文：未开启输入法
	}
	ok2, conv := sendImeMessage(imeWnd, win32.IMC_GETCONVERSIONMODE, 0)
	if !ok2 {
		logging.Debug("IsChineseMode: IMC_GETCONVERSIONMODE 超时/失败")
		return false
	}
	return (conv & int(win32.IME_CMODE_NATIVE)) != 0
}

// SetMode 绝对设置中/英文输入模式
func SetMode(chinese bool) bool {
	hwnd := getFocusedWindow()
	if hwnd == 0 {
		return false
	}
	imeWnd := getIMEWindow(hwnd)
	if imeWnd == 0 {
		return false
	}
	var open, conv uintptr
	if chinese {
		open, conv = 1, 1025
	} else {
		open, conv = 0, 1024
	}
	ok1, _ := sendImeMessage(imeWnd, win32.IMC_SETOPENSTATUS, open)
	if !ok1 {
		logging.Warn("SetMode: IMC_SETOPENSTATUS 失败", "chinese", chinese)
	}
	ok2, _ := sendImeMessage(imeWnd, win32.IMC_SETCONVERSIONMODE, conv)
	if !ok2 {
		logging.Warn("SetMode: IMC_SETCONVERSIONMODE 失败", "chinese", chinese)
	}
	return ok1 && ok2
}

// GetForegroundProcessName 返回前台窗口所属进程名（小写，含 .exe），用于 SET 白名单校验。
func GetForegroundProcessName() string {
	hwnd := getForegroundWindow()
	if hwnd == 0 {
		return ""
	}
	_, pid := getWindowThreadProcessId(hwnd)
	if pid == 0 {
		return ""
	}
	const PROCESS_QUERY_INFORMATION = 0x0400
	const PROCESS_VM_READ = 0x0010
	h, _, _ := win32.ProcOpenProcess.Call(PROCESS_QUERY_INFORMATION|PROCESS_VM_READ, 0, uintptr(pid))
	if h == 0 {
		return ""
	}
	defer win32.ProcCloseHandle.Call(h)
	buf := make([]uint16, 260)
	n := uint32(len(buf))
	r, _, _ := win32.ProcQueryFullProcessImageNameW.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		return ""
	}
	name := windows.UTF16ToString(buf[:n])
	idx := strings.LastIndexAny(name, "\\/")
	if idx >= 0 {
		name = name[idx+1:]
	}
	return strings.ToLower(name)
}
