package win32

// ============================================================
// 常量
// ============================================================

// 窗口样式 / 扩展样式
const (
	WS_POPUP            = 0x80000000
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_EX_LAYERED       = 0x00080000
	WS_EX_TRANSPARENT   = 0x00000020
	WS_EX_TOPMOST       = 0x00000008
	WS_EX_NOACTIVATE    = 0x08000000
	WS_EX_TOOLWINDOW    = 0x00000080
	CW_USEDEFAULT       = 0x80000000
)

// 显示 / 位置
const (
	SW_SHOW        = 5
	SW_HIDE        = 0
	SWP_NOMOVE     = 0x0002
	SWP_NOSIZE     = 0x0001
	SWP_NOACTIVATE = 0x0010
	ULW_ALPHA      = 0x00000002
	AC_SRC_OVER    = 0
	AC_SRC_ALPHA   = 1
)

// 消息
const (
	WM_DESTROY     = 0x0002
	WM_COMMAND     = 0x0111
	WM_USER        = 0x0400
	WM_TRAYICON    = WM_USER + 1
	WM_RBUTTONUP   = 0x0205
	WM_IME_CONTROL = 0x0283
)

// 托盘
const (
	NIM_ADD     = 0x00000000
	NIM_DELETE  = 0x00000001
	NIF_ICON    = 0x00000002
	NIF_MESSAGE = 0x00000001
	NIF_TIP     = 0x00000004
)

// IME
const (
	IMC_GETOPENSTATUS     = 0x0005
	IMC_SETOPENSTATUS     = 0x0006
	IMC_GETCONVERSIONMODE = 0x0001
	IMC_SETCONVERSIONMODE = 0x0002
	IME_CMODE_NATIVE      = 0x0001
	CFS_POINT             = 0x0002
)

// 其他
const (
	SMTO_ABORTIFHUNG              = 0x0002
	CURSOR_SHOWING                = 0x00000001
	OBJID_CARET                   = 0xFFFFFFF8 // 作为 uint32
	PROCESS_PER_MONITOR_DPI_AWARE = 2
	IMAGE_ICON                    = 1
	IDI_APPLICATION               = 32512 // MAKEINTRESOURCE(32512)
	LR_DEFAULTCOLOR               = 0x00000000
	LR_LOADFROMFILE               = 0x00000010
	LR_DEFAULTSIZE                = 0x00000040
	MB_ICONINFORMATION            = 0x00000040
	MB_OK                         = 0x00000000
	MF_STRING                     = 0x00000000
	MF_SEPARATOR                  = 0x0800
	TPM_LEFTALIGN                 = 0x0000
	TPM_BOTTOMALIGN               = 0x0020
)

// CLSCTX_ALL（CoCreateInstance 用）
const CLSCTX_ALL = 0x17

// HWND_TOPMOST = (HWND)-2
var hwndTopMost = HWND(^uintptr(0) - 1)

// HwndTopMost 返回 HWND_TOPMOST
func HwndTopMost() HWND { return hwndTopMost }

// ============================================================
// 自定义结构（x/sys/windows 缺失或需精确控制布局者）
// ============================================================

// BLENDFUNCTION（UpdateLayeredWindow 使用）
type BLENDFUNCTION struct {
	BlendOp             byte
	BlendFlags          byte
	SourceConstantAlpha byte
	AlphaFormat         byte
}

// COMPOSITIONFORM（IME 组合窗口位置）
type COMPOSITIONFORM struct {
	DwStyle      uint32
	PtCurrentPos POINT
	RcArea       RECT
}

// NOTIFYICONDATAW（精简，足够托盘使用）
type NOTIFYICONDATA struct {
	CbSize           uint32
	HWnd             HWND
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            HICON
	SzTip            [128]uint16
	// 以下字段在较新结构中存在，但本项目未使用；保持对齐用填充
	// 某些 Windows 版本要求结构足够大，故补到 512 字节安全区
	_ [392]byte
}
