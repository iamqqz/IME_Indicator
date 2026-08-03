// 鼠标光标形状检测：比对当前光标句柄与配置的目标光标共享句柄集合。
package cursordetect

import (
	"unsafe"

	"imeqiao/internal/win32"
)

// CursorDetector 鼠标形状检测器
type CursorDetector struct {
	targetHandles map[win32.HCURSOR]struct{}
}

// New 由目标系统光标 ID（如 32513=I-Beam, 32512=Normal）构建检测器
func New(targetCursorIDs []uint32) *CursorDetector {
	handles := map[win32.HCURSOR]struct{}{}
	for _, cid := range targetCursorIDs {
		r, _, _ := win32.ProcLoadCursorW.Call(0, uintptr(cid))
		h := win32.HCURSOR(r)
		if h != 0 {
			handles[h] = struct{}{}
		}
	}
	return &CursorDetector{targetHandles: handles}
}

// IsTargetCursor 当前光标是否为目标形状之一
func (c *CursorDetector) IsTargetCursor() bool {
	var ci win32.CURSORINFO
	ci.CbSize = uint32(unsafe.Sizeof(ci))
	if !win32.CallOK(win32.ProcGetCursorInfo, uintptr(unsafe.Pointer(&ci))) {
		return false
	}
	if ci.Flags&win32.CURSOR_SHOWING == 0 {
		return false
	}
	_, ok := c.targetHandles[ci.HCursor]
	return ok
}
