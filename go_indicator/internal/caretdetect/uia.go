package caretdetect

import (
	"fmt"
	"unsafe"

	"imeqiao/internal/win32"
)

// getFocusedElement 取当前焦点元素；返回 (element, true)
func (d *CaretDetector) getFocusedElement() (*win32.ComObject, bool) {
	if d.automation == nil {
		d.LastUIAError = "NoAutomation"
		return nil, false
	}
	var pElem uintptr
	hr := d.automation.Call(win32.SlotIUIAutomation_GetFocusedElement, uintptr(unsafe.Pointer(&pElem)))
	if hr != 0 || pElem == 0 {
		d.LastUIAError = fmt.Sprintf("Focus:%X", uint32(hr))
		return nil, false
	}
	return win32.NewComObject(pElem), true
}

// getPattern 用 GetCurrentPatternAs 取得指定模式接口
func (d *CaretDetector) getPattern(el *win32.ComObject, iid *win32.GUID, patternID int32, tag string) (*win32.ComObject, bool) {
	var pPat uintptr
	hr := el.Call(win32.SlotElement_GetCurrentPatternAs, uintptr(patternID), uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&pPat)))
	if hr != 0 || pPat == 0 {
		d.LastUIAError = fmt.Sprintf("%s:Pat:%X", tag, uint32(hr))
		return nil, false
	}
	return win32.NewComObject(pPat), true
}

// rectsFromRange 读取文本范围的边界矩形 [left, top, width, height, ...]
func (d *CaretDetector) rectsFromRange(rng *win32.ComObject, tag string) (CaretPos, bool) {
	var psa uintptr
	hr := rng.Call(win32.SlotTextRange_GetBoundingRectangles, uintptr(unsafe.Pointer(&psa)))
	if hr != 0 || psa == 0 {
		d.LastUIAError = fmt.Sprintf("%s:Rect:%X", tag, uint32(hr))
		return CaretPos{}, false
	}
	defer win32.ProcSafeArrayDestroy.Call(psa)

	var lb, ub int32
	win32.ProcSafeArrayGetLBound.Call(psa, 1, uintptr(unsafe.Pointer(&lb)))
	win32.ProcSafeArrayGetUBound.Call(psa, 1, uintptr(unsafe.Pointer(&ub)))
	count := ub - lb + 1
	if count < 4 {
		d.LastUIAError = fmt.Sprintf("%s:Cnt:%d", tag, count)
		return CaretPos{}, false
	}
	var data unsafe.Pointer
	if !win32.CallOK(win32.ProcSafeArrayAccessData, psa, uintptr(unsafe.Pointer(&data))) || data == nil {
		d.LastUIAError = fmt.Sprintf("%s:Access", tag)
		return CaretPos{}, false
	}
	defer win32.ProcSafeArrayUnaccessData.Call(psa)
	doubles := unsafe.Slice((*float64)(data), int(count))
	left := int32(doubles[0])
	top := int32(doubles[1])
	height := int32(doubles[3])
	return CaretPos{left, top, height}, true
}

// getPosViaUiaCaretRange 第二级：UIA TextPattern2.GetCaretRange（VS Code）
func (d *CaretDetector) getPosViaUiaCaretRange() (CaretPos, bool) {
	el, ok := d.getFocusedElement()
	if !ok {
		return CaretPos{}, false
	}
	defer win32.Release(el)

	tp2, ok := d.getPattern(el, &win32.IID_IUIAutomationTextPattern2, win32.UIA_TextPattern2Id, "Car")
	if !ok {
		return CaretPos{}, false
	}
	defer win32.Release(tp2)

	var isActive int32
	var pRange uintptr
	hr := tp2.Call(win32.SlotTextPattern2_GetCaretRange, uintptr(unsafe.Pointer(&isActive)), uintptr(unsafe.Pointer(&pRange)))
	if hr != 0 || pRange == 0 {
		d.LastUIAError = fmt.Sprintf("Car:Range:%X", uint32(hr))
		return CaretPos{}, false
	}
	rng := win32.NewComObject(pRange)
	defer win32.Release(rng)
	return d.rectsFromRange(rng, "Car")
}

// getPosViaUiaSelection 第三级：UIA TextPattern.GetSelection（Chrome）
func (d *CaretDetector) getPosViaUiaSelection() (CaretPos, bool) {
	el, ok := d.getFocusedElement()
	if !ok {
		return CaretPos{}, false
	}
	defer win32.Release(el)

	tp, ok := d.getPattern(el, &win32.IID_IUIAutomationTextPattern, win32.UIA_TextPatternId, "Sel")
	if !ok {
		return CaretPos{}, false
	}
	defer win32.Release(tp)

	var psa uintptr
	hr := tp.Call(win32.SlotTextPattern_GetSelection, uintptr(unsafe.Pointer(&psa)))
	if hr != 0 || psa == 0 {
		d.LastUIAError = fmt.Sprintf("Sel:Sel:%X", uint32(hr))
		return CaretPos{}, false
	}
	defer win32.ProcSafeArrayDestroy.Call(psa)

	var lb, ub int32
	win32.ProcSafeArrayGetLBound.Call(psa, 1, uintptr(unsafe.Pointer(&lb)))
	win32.ProcSafeArrayGetUBound.Call(psa, 1, uintptr(unsafe.Pointer(&ub)))
	if ub < lb {
		d.LastUIAError = "Sel:Empty"
		return CaretPos{}, false
	}
	var pRange uintptr
	if !win32.CallOK(win32.ProcSafeArrayGetElement, psa, uintptr(0), uintptr(unsafe.Pointer(&pRange))) || pRange == 0 {
		d.LastUIAError = "Sel:Elem"
		return CaretPos{}, false
	}
	rng := win32.NewComObject(pRange)
	defer win32.Release(rng)
	return d.rectsFromRange(rng, "Sel")
}
