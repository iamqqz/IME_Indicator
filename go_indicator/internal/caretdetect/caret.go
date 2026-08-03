// 文本光标位置检测：多级回退策略。
// 与 rust_indicator/caret_detector.rs 对应：
//  1. GetGUIThreadInfo（原生，记事本等）
//  2. UIA TextPattern2.GetCaretRange（VS Code）
//  3. UIA TextPattern.GetSelection（Chrome）
//  4. IME 组合窗口
//  5. MSAA + GUITHREADINFO 兜底
package caretdetect

import (
	"unsafe"

	"golang.org/x/sys/windows"
	"imeqiao/internal/win32"
)

// DetectionSource 检测来源
type DetectionSource int

const (
	SourceNone DetectionSource = iota
	SourceGuiInfo
	SourceUiAutomation
	SourceUiaCaretRange
	SourceIme
	SourceMsaaFallback
)

// CaretPos 光标位置 (x, y, height)
type CaretPos = [3]int32

// CaretDetector 文本光标检测器
type CaretDetector struct {
	automation   *win32.ComObject
	LastSource   DetectionSource
	LastUIAError string
}

// New 创建检测器并初始化 COM / UI Automation
func New() *CaretDetector {
	d := &CaretDetector{LastSource: SourceNone}
	// 初始化 COM（UIA 要求 STA；忽略 RPC_E_CHANGED_MODE 等已初始化错误）
	_ = windows.CoInitializeEx(0, 2) // COINIT_APARTMENTTHREADED = 2
	var p uintptr
	hr := win32.CallR(win32.ProcCoCreateInstance,
		uintptr(unsafe.Pointer(&win32.CLSID_CUIAutomation)),
		0,
		win32.CLSCTX_ALL,
		uintptr(unsafe.Pointer(&win32.IID_IUIAutomation)),
		uintptr(unsafe.Pointer(&p)),
	)
	if hr == 0 && p != 0 {
		d.automation = win32.NewComObject(p)
	}
	return d
}

// Cleanup 释放 COM
func (d *CaretDetector) Cleanup() {
	if d.automation != nil {
		win32.Release(d.automation)
		d.automation = nil
	}
	windows.CoUninitialize()
}

// GetCaretPos 返回光标位置；按五级优先级返回第一个成功结果
func (d *CaretDetector) GetCaretPos() (CaretPos, bool) {
	if pos, ok := d.getPosViaGuiInfo(); ok {
		d.LastSource = SourceGuiInfo
		return pos, true
	}
	if pos, ok := d.getPosViaUiaCaretRange(); ok {
		d.LastSource = SourceUiaCaretRange
		return pos, true
	}
	if pos, ok := d.getPosViaUiaSelection(); ok {
		d.LastSource = SourceUiAutomation
		return pos, true
	}
	if pos, ok := d.getPosViaIme(); ok {
		d.LastSource = SourceIme
		return pos, true
	}
	if pos, ok := d.getPosViaMsaaFallback(); ok {
		d.LastSource = SourceMsaaFallback
		return pos, true
	}
	d.LastSource = SourceNone
	return CaretPos{}, false
}

// getPosViaGuiInfo 第一级：GetGUIThreadInfo
func (d *CaretDetector) getPosViaGuiInfo() (CaretPos, bool) {
	var gui win32.GUITHREADINFO
	gui.CbSize = uint32(unsafe.Sizeof(gui))
	if !win32.CallOK(win32.ProcGetGUIThreadInfo, 0, uintptr(unsafe.Pointer(&gui))) {
		return CaretPos{}, false
	}
	if gui.HwndCaret == 0 {
		return CaretPos{}, false
	}
	pt := win32.POINT{X: gui.RcRect.Left, Y: gui.RcRect.Top}
	win32.ProcClientToScreen.Call(uintptr(gui.HwndCaret), uintptr(unsafe.Pointer(&pt)))
	h := gui.RcRect.Bottom - gui.RcRect.Top
	return CaretPos{pt.X, pt.Y, h}, true
}
