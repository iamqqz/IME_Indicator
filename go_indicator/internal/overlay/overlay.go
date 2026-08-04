// 分层置顶透明悬浮窗：用预渲染的 32bpp 预乘 alpha DIB + UpdateLayeredWindow 绘制彩色圆点。
// 不依赖 GDI+，纯 Win32，确定性更强；颜色为 #RRGGBBAA 经 config 解析成 0xAARRGGBB。
package overlay

import (
	"fmt"
	"math"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"imeqiao/internal/logging"
	"imeqiao/internal/win32"
)

const className = "IMEIndicatorOverlayClass"

// wndProc 仅处理 WM_DESTROY，其余交 DefWindowProcW。必须在包级创建一次。
// 回调参数统一用 uintptr（NewCallback 仅保证 uintptr 安全），内部转换。
var wndProc = windows.NewCallback(func(hwnd, msg, wParam, lParam uintptr) uintptr {
	if uint32(msg) == win32.WM_DESTROY {
		return 0
	}
	r, _, _ := win32.ProcDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
})

func registerClass() {
	hInst, _, _ := win32.ProcGetModuleHandleW.Call(0)
	clsName, _ := windows.UTF16PtrFromString(className)
	wc := win32.WNDCLASSEX{
		CbSize:      uint32(unsafe.Sizeof(win32.WNDCLASSEX{})),
		LpfnWndProc: wndProc,
		Instance:    win32.HINSTANCE(hInst),
		ClassName:   clsName,
	}
	win32.ProcRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
}

// IndicatorOverlay 单个悬浮窗
type IndicatorOverlay struct {
	hwnd    win32.HWND
	name    string
	size    int32
	colorCN uint32
	colorEN uint32
	offsetX int32
	offsetY int32

	memDC  win32.HDC
	cnBmp  win32.HBITMAP
	enBmp  win32.HBITMAP
	oldBmp win32.HGDIOBJ

	lastDrawErrLog time.Time // 限流：绘制失败日志最多每 5s 一条
	lastDiagLog    time.Time // 限流：诊断日志最多每 2s 一条
}

// New 创建悬浮窗并预渲染中/英两种颜色的圆点
func New(name string, size int32, colorCN, colorEN uint32, offsetX, offsetY int32) *IndicatorOverlay {
	registerClass()

	screenDC, _, _ := win32.ProcGetDC.Call(0)
	memDC, _, _ := win32.ProcCreateCompatibleDC.Call(screenDC)
	win32.ProcReleaseDC.Call(0, screenDC)

	cnBmp, _ := createDIB(memDC, size, colorCN)
	enBmp, _ := createDIB(memDC, size, colorEN)
	if cnBmp == 0 || enBmp == 0 {
		logging.Warn("overlay: 创建圆点 DIB 失败，圆点将不可见", "name", name)
	}

	title, _ := windows.UTF16PtrFromString("Indicator_" + name)
	clsName, _ := windows.UTF16PtrFromString(className)
	hInst, _, _ := win32.ProcGetModuleHandleW.Call(0)

	exStyle := uint32(win32.WS_EX_LAYERED | win32.WS_EX_TRANSPARENT | win32.WS_EX_TOPMOST | win32.WS_EX_NOACTIVATE)
	hwnd, _, _ := win32.ProcCreateWindowExW.Call(
		uintptr(exStyle),
		uintptr(unsafe.Pointer(clsName)),
		uintptr(unsafe.Pointer(title)),
		uintptr(win32.WS_POPUP),
		0, 0, uintptr(size), uintptr(size),
		0, 0,
		uintptr(hInst),
		0,
	)
	if hwnd == 0 {
		logging.Error("overlay: CreateWindowExW 失败 (hwnd=0)，悬浮窗不可用", "name", name)
	}

	return &IndicatorOverlay{
		hwnd:    win32.HWND(hwnd),
		name:    name,
		size:    size,
		colorCN: colorCN,
		colorEN: colorEN,
		offsetX: offsetX,
		offsetY: offsetY,
		memDC:   win32.HDC(memDC),
		cnBmp:   win32.HBITMAP(cnBmp),
		enBmp:   win32.HBITMAP(enBmp),
	}
}

// createDIB 创建指定颜色的预乘 alpha 圆点 DIB，返回 HBITMAP（失败返回 0）
func createDIB(memDC win32.HDC, size int32, rgba uint32) (win32.HBITMAP, unsafe.Pointer) {
	bmi := win32.BITMAPINFO{
		Header: win32.BITMAPINFOHEADER{
			Size:     uint32(unsafe.Sizeof(win32.BITMAPINFOHEADER{})),
			Width:    int32(size),
			Height:   -int32(size), // 自上而下
			Planes:   1,
			BitCount: 32,
			// Compression 默认 0 (BI_RGB)
		},
	}
	var bits unsafe.Pointer
	hBmp, _, _ := win32.ProcCreateDIBSection.Call(
		uintptr(memDC),
		uintptr(unsafe.Pointer(&bmi)),
		0, // DIB_RGB_COLORS
		uintptr(unsafe.Pointer(&bits)),
		0,
		0,
	)
	if hBmp == 0 {
		return 0, nil
	}
	renderCircle(bits, size, rgba)
	return win32.HBITMAP(hBmp), bits
}

// renderCircle 用 2x2 超采样抗锯齿绘制实心圆，写入预乘 alpha 的 BGRA 像素
func renderCircle(bits unsafe.Pointer, size int32, rgba uint32) {
	a := float64((rgba>>24)&0xFF) / 255.0
	cr := float64((rgba >> 16) & 0xFF)
	cg := float64((rgba >> 8) & 0xFF)
	cb := float64(rgba & 0xFF)

	r := float64(size) / 2.0
	cx := float64(size-1) / 2.0
	cy := float64(size-1) / 2.0
	step := 0.5 // 子采样偏移

	px := (*[1 << 30]byte)(bits)[:size*size*4]
	for y := int32(0); y < size; y++ {
		for x := int32(0); x < size; x++ {
			cover := 0.0
			for sy := 0; sy < 2; sy++ {
				for sx := 0; sx < 2; sx++ {
					dx := float64(x) + float64(sx)*step - cx
					dy := float64(y) + float64(sy)*step - cy
					d := sqrt(dx*dx + dy*dy)
					if d <= r-0.5 {
						cover += 1
					} else if d < r+0.5 {
						cover += (r + 0.5 - d)
					}
				}
			}
			cover /= 4.0
			alpha := cover * a
			// 预乘
			pr := uint8(cr * alpha)
			pg := uint8(cg * alpha)
			pb := uint8(cb * alpha)
			pa := uint8(alpha * 255.0)
			off := (uintptr(y)*uintptr(size) + uintptr(x)) * 4
			px[off+0] = pb
			px[off+1] = pg
			px[off+2] = pr
			px[off+3] = pa
		}
	}
}

func sqrt(v float64) float64 {
	return math.Sqrt(v)
}

// Update 更新位置与颜色
func (o *IndicatorOverlay) Update(x, y int32, isChinese bool, caretH int32) {
	bmp := o.cnBmp
	if !isChinese {
		bmp = o.enBmp
	}
	win32.ProcSelectObject.Call(uintptr(o.memDC), uintptr(bmp))

	dest := win32.POINT{
		X: x + o.offsetX - o.size/2,
		Y: y + caretH + o.offsetY - o.size/2,
	}
	src := win32.POINT{X: 0, Y: 0}
	sz := win32.SIZE{CX: o.size, CY: o.size}
	blend := win32.BLENDFUNCTION{
		BlendOp:             win32.AC_SRC_OVER,
		SourceConstantAlpha: 255,
		AlphaFormat:         win32.AC_SRC_ALPHA,
	}
	// 关键修正：hdcDest 传每帧新鲜的 screen DC（GetDC/ReleaseDC），与能正常工作的
	// Python 参考一致。之前传 0(NULL) 启动时能显示，但切换前台窗口后分层窗口会
	// 从 DWM 合成树掉出 → 圆点永久消失。
	screenDC, _, _ := win32.ProcGetDC.Call(0)
	ulwRet := win32.CallR(win32.ProcUpdateLayeredWindow,
		uintptr(o.hwnd),
		screenDC,
		uintptr(unsafe.Pointer(&dest)),
		uintptr(unsafe.Pointer(&sz)),
		uintptr(o.memDC),
		uintptr(unsafe.Pointer(&src)),
		0,
		uintptr(unsafe.Pointer(&blend)),
		win32.ULW_ALPHA,
	)
	win32.ProcReleaseDC.Call(0, screenDC)
	if ulwRet == 0 {
		// 绘制失败会导致圆点冻结/不动，是常见"不稳定"症状，但限流以免刷屏
		now := time.Now()
		if now.Sub(o.lastDrawErrLog) > 5*time.Second {
			o.lastDrawErrLog = now
			logging.Warn("overlay: UpdateLayeredWindow 失败（圆点可能冻结），hwnd=0 表示窗口未创建", "hwnd", o.hwnd)
		}
	}

	// 保持最顶层 + 每帧重新断言可见性。
	// WS_EX_LAYERED 弹出窗在切换前台窗口/虚拟桌面时可能被系统（DWM）悄悄隐藏，
	// 而 Show/Hide 仅在状态切换时调用一次，故此处用 SWP_SHOWWINDOW 强制窗口重新
	// 进入合成树——否则一旦被隐藏就再也回不来（Python 参考实现正是此写法）。
	// 注意：必须在 UpdateLayeredWindow 之后调用（与 Python 顺序一致）。
	swpRet := win32.CallR(win32.ProcSetWindowPos,
		uintptr(o.hwnd),
		uintptr(win32.HwndTopMost()),
		0, 0, 0, 0,
		win32.SWP_NOMOVE|win32.SWP_NOSIZE|win32.SWP_NOACTIVATE|win32.SWP_SHOWWINDOW,
	)

	// 诊断：每 2 秒记录一次窗口实际可见性、定位坐标与屏幕矩形，用于定位“切窗口后圆点消失”
	now := time.Now()
	if now.Sub(o.lastDiagLog) > 2*time.Second {
		o.lastDiagLog = now
		visible := win32.CallOK(win32.ProcIsWindowVisible, uintptr(o.hwnd))
		var rc win32.RECT
		win32.ProcGetWindowRect.Call(uintptr(o.hwnd), uintptr(unsafe.Pointer(&rc)))
		logging.Info("overlay 诊断",
			"name", o.name, "hwnd", o.hwnd, "visible", visible,
			"cursor", fmt.Sprintf("%d,%d", x, y),
			"dest", fmt.Sprintf("%d,%d", dest.X, dest.Y),
			"rect", fmt.Sprintf("L=%d T=%d R=%d B=%d", rc.Left, rc.Top, rc.Right, rc.Bottom),
			"ulw", ulwRet, "swp", swpRet)
	}

	// 处理本窗口消息
	var msg win32.MSG
	for win32.CallOK(win32.ProcPeekMessageW, uintptr(unsafe.Pointer(&msg)), uintptr(o.hwnd), 0, 0, 1 /*PM_REMOVE*/) {
		win32.ProcTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		win32.ProcDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

// Show 显示窗口
func (o *IndicatorOverlay) Show() {
	win32.ProcShowWindow.Call(uintptr(o.hwnd), win32.SW_SHOW)
}

// Hide 隐藏窗口
func (o *IndicatorOverlay) Hide() {
	win32.ProcShowWindow.Call(uintptr(o.hwnd), win32.SW_HIDE)
}

// Cleanup 清理 GDI 资源与窗口
func (o *IndicatorOverlay) Cleanup() {
	if o.oldBmp != 0 {
		win32.ProcSelectObject.Call(uintptr(o.memDC), uintptr(o.oldBmp))
	}
	if o.cnBmp != 0 {
		win32.ProcDeleteObject.Call(uintptr(o.cnBmp))
	}
	if o.enBmp != 0 {
		win32.ProcDeleteObject.Call(uintptr(o.enBmp))
	}
	if o.memDC != 0 {
		win32.ProcDeleteDC.Call(uintptr(o.memDC))
	}
	if o.hwnd != 0 {
		win32.ProcDestroyWindow.Call(uintptr(o.hwnd))
	}
}
