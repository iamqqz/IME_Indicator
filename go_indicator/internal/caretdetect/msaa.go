package caretdetect

import (
	"fmt"
	"unsafe"

	"imeqiao/internal/win32"
)

func fgWindow() win32.HWND {
	r, _, _ := win32.ProcGetForegroundWindow.Call()
	return win32.HWND(r)
}

// getPosViaIme 第四级：IME 组合窗口位置
func (d *CaretDetector) getPosViaIme() (CaretPos, bool) {
	hwnd := fgWindow()
	if hwnd == 0 {
		return CaretPos{}, false
	}
	hImc, _, _ := win32.ProcImmGetContext.Call(uintptr(hwnd))
	if win32.HIMC(hImc) == 0 {
		return CaretPos{}, false
	}
	var comp win32.COMPOSITIONFORM
	ok, _, _ := win32.ProcImmGetCompositionWindow.Call(uintptr(hwnd), uintptr(hImc), uintptr(unsafe.Pointer(&comp)))
	win32.ProcImmReleaseContext.Call(uintptr(hwnd), uintptr(hImc))
	if ok != 0 && (comp.DwStyle&win32.CFS_POINT) != 0 {
		pt := comp.PtCurrentPos
		win32.ProcClientToScreen.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&pt)))
		return CaretPos{pt.X, pt.Y, 20}, true
	}
	return CaretPos{}, false
}

// variantChildSelf 构造 CHILDID_SELF 的 VARIANT（VT_I4 = 0）
type variantChildSelf struct {
	vt  uint16
	_   [6]byte
	val int64
}

// getPosViaMsaaFallback 第五级：MSAA AccessibleObjectFromWindow(OBJID_CARET) + 回退 GUITHREADINFO
func (d *CaretDetector) getPosViaMsaaFallback() (CaretPos, bool) {
	hwnd := fgWindow()
	if hwnd == 0 {
		d.LastUIAError = "MSAA:NoHwnd"
		return CaretPos{}, false
	}

	var pAcc uintptr
	hr, _, _ := win32.ProcAccessibleObjectFromWindow.Call(
		uintptr(hwnd),
		uintptr(win32.OBJID_CARET),
		uintptr(unsafe.Pointer(&win32.IID_IAccessible)),
		uintptr(unsafe.Pointer(&pAcc)),
	)
	if hr == 0 && pAcc != 0 {
		acc := win32.NewComObject(pAcc)
		defer win32.Release(acc)

		var x, y, w, h int32
		v := variantChildSelf{vt: 3} // VT_I4
		r := acc.Call(win32.SlotAccessible_accLocation,
			uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&y)),
			uintptr(unsafe.Pointer(&w)), uintptr(unsafe.Pointer(&h)),
			uintptr(unsafe.Pointer(&v)))
		if r == 0 && (x != 0 || y != 0) {
			return CaretPos{x, y, h}, true
		}
		d.LastUIAError = fmt.Sprintf("MSAA:Loc:%X", uint32(r))
	}

	// 回退 GUITHREADINFO
	var gui win32.GUITHREADINFO
	gui.CbSize = uint32(unsafe.Sizeof(gui))
	if !win32.CallOK(win32.ProcGetGUIThreadInfo, 0, uintptr(unsafe.Pointer(&gui))) {
		d.LastUIAError = "MSAA:GuiFail"
		return CaretPos{}, false
	}
	var target win32.HWND
	switch {
	case gui.HwndCaret != 0:
		target = gui.HwndCaret
	case gui.HwndFocus != 0:
		target = gui.HwndFocus
	case gui.HwndActive != 0:
		target = gui.HwndActive
	default:
		return CaretPos{}, false
	}
	if gui.RcRect.Left != 0 || gui.RcRect.Top != 0 {
		pt := win32.POINT{X: gui.RcRect.Left, Y: gui.RcRect.Top}
		win32.ProcClientToScreen.Call(uintptr(target), uintptr(unsafe.Pointer(&pt)))
		if pt.X > -1000 && pt.Y > -1000 {
			h := gui.RcRect.Bottom - gui.RcRect.Top
			return CaretPos{pt.X, pt.Y, h}, true
		}
	}
	return CaretPos{}, false
}
