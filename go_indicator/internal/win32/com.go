package win32

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ============================================================
// COM 基础：对象指针 + vtable 调用
// ============================================================

// comObject 表示一个 COM 接口指针：其内存布局第一个字就是 vtable 指针。
// 必须保留原始的 COM 接口指针 p，因为 COM 方法调用时要把它作为 this（rcx）传入，
// 而不是 Go 包装结构体自身的地址。
type ComObject struct {
	ptr uintptr // 原始 COM 接口指针（= this）
}

// NewComObject 由 CoCreateInstance 返回的接口指针（指向 vtable 指针）构造。
func NewComObject(p uintptr) *ComObject {
	if p == 0 {
		return nil
	}
	return &ComObject{ptr: p}
}

// Call 调用第 slot 个虚方法（IUnknown 前三槽为 QueryInterface/AddRef/Release）。
// 64 位 Windows x64 调用约定：COM 方法第一个参数 rcx 必须是接口指针 this，
// 其后才是用户参数。因此这里把 o.ptr 作为 a1(this)，用户参数依次右移。
func (o *ComObject) Call(slot int, args ...uintptr) uintptr {
	if o == nil || o.ptr == 0 {
		return 0
	}
	// vet 认可的指针算术：unsafe.Pointer(uintptr(base) + offset)
	vtbl := *(*uintptr)(unsafe.Pointer(o.ptr))
	fnPtr := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(slot)*unsafe.Sizeof(uintptr(0))))
	this := o.ptr
	var a1, a2, a3, a4, a5, a6 uintptr
	a1 = this
	n := 1
	if len(args) > 0 {
		a2 = args[0]
		n = 2
	}
	if len(args) > 1 {
		a3 = args[1]
		n = 3
	}
	if len(args) > 2 {
		a4 = args[2]
		n = 4
	}
	if len(args) > 3 {
		a5 = args[3]
		n = 5
	}
	if len(args) > 4 {
		a6 = args[4]
		n = 6
	}
	if len(args) > 5 {
		// 当前所有调用点最多 5 个用户参数（accLocation），超出则无法用 Syscall6 传递。
		return 0
	}
	r1, _, _ := syscall.Syscall6(fnPtr, uintptr(n), a1, a2, a3, a4, a5, a6)
	return r1
}

// Release 调用 Release（槽 2），释放接口引用。
func Release(obj *ComObject) {
	if obj != nil && obj.ptr != 0 {
		obj.Call(2)
	}
}

// ============================================================
// GUID 常量（对照 Windows SDK uiautomationclient.h / oleacc.idl）
// ============================================================

// CLSID_CUIAutomation
var CLSID_CUIAutomation = windows.GUID{
	Data1: 0xff48dba4, Data2: 0x60ef, Data3: 0x4201,
	Data4: [8]byte{0xaa, 0x87, 0x54, 0x10, 0x3e, 0xef, 0x59, 0x4e},
}

// IID_IUIAutomation
var IID_IUIAutomation = windows.GUID{
	Data1: 0x30cbe57d, Data2: 0xd9d0, Data3: 0x452a,
	Data4: [8]byte{0xab, 0x13, 0x7a, 0xc5, 0xac, 0x48, 0x25, 0xee},
}

// IID_IUIAutomationElement
var IID_IUIAutomationElement = windows.GUID{
	Data1: 0xd22108aa, Data2: 0x8ac5, Data3: 0x49a5,
	Data4: [8]byte{0x83, 0x7b, 0x37, 0xbb, 0xb3, 0xd2, 0xa8, 0x49},
}

// IID_IUIAutomationTextPattern
var IID_IUIAutomationTextPattern = windows.GUID{
	Data1: 0x32eba289, Data2: 0x3583, Data3: 0x42c9,
	Data4: [8]byte{0x9c, 0x59, 0x3b, 0x6d, 0x9a, 0x1b, 0xc4, 0xc3},
}

// IID_IUIAutomationTextPattern2
var IID_IUIAutomationTextPattern2 = windows.GUID{
	Data1: 0x9c2f6f6c, Data2: 0xe3d1, Data3: 0x4cf2,
	Data4: [8]byte{0x83, 0x7a, 0x06, 0x3c, 0x3a, 0xe6, 0x43, 0x1c},
}

// IID_IAccessible
var IID_IAccessible = windows.GUID{
	Data1: 0x618736e0, Data2: 0x3c3d, Data3: 0x11cf,
	Data4: [8]byte{0x81, 0x0c, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71},
}

// ============================================================
// vtable 槽位（对照 SDK 接口声明顺序，务必精确）
// ============================================================

const (
	// IUIAutomation
	SlotIUIAutomation_GetFocusedElement = 8
	// IUIAutomationElement
	SlotElement_GetCurrentPatternAs = 14
	// IUIAutomationTextPattern
	SlotTextPattern_GetSelection = 5
	// IUIAutomationTextPattern2
	SlotTextPattern2_GetCaretRange = 10
	// IUIAutomationTextRange
	SlotTextRange_GetBoundingRectangles = 10
	SlotTextRange_ExpandToEnclosingUnit = 6
	// IAccessible
	SlotAccessible_accLocation = 22
)

// UIA 模式 ID
const (
	UIA_TextPatternId  = 10014
	UIA_TextPattern2Id = 10025
	TextUnit_Character = 0
)
