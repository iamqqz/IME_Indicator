import ctypes
from win32_api import (
    user32, imm32, GUITHREADINFO, WM_IME_CONTROL, 
    IMC_GETOPENSTATUS, IMC_GETCONVERSIONMODE, IME_CMODE_NATIVE
)
from ctypes import byref, sizeof, wintypes

# SET 用命令（与 Go/Rust 版一致）
IMC_SETOPENSTATUS = 0x0006
IMC_SETCONVERSIONMODE = 0x0002

# kernel32：用于按前台窗口取进程名（SET 白名单校验）
_kernel32 = ctypes.windll.kernel32
_kernel32.OpenProcess.argtypes = [wintypes.DWORD, wintypes.BOOL, wintypes.DWORD]
_kernel32.OpenProcess.restype = wintypes.HANDLE
_kernel32.QueryFullProcessImageNameW.argtypes = [wintypes.HANDLE, wintypes.DWORD, wintypes.LPWSTR, ctypes.POINTER(wintypes.DWORD)]
_kernel32.QueryFullProcessImageNameW.restype = wintypes.BOOL
_kernel32.CloseHandle.argtypes = [wintypes.HANDLE]
_kernel32.CloseHandle.restype = wintypes.BOOL

def get_focused_window():
    """获取当前焦点窗口"""
    fore_hwnd = user32.GetForegroundWindow()
    if not fore_hwnd:
        return 0
    
    thread_id = user32.GetWindowThreadProcessId(fore_hwnd, None)
    gui_info = GUITHREADINFO()
    gui_info.cbSize = sizeof(GUITHREADINFO)
    
    if user32.GetGUIThreadInfo(thread_id, byref(gui_info)):
        if gui_info.hwndFocus:
            return gui_info.hwndFocus
        if gui_info.hwndActive:
            return gui_info.hwndActive
    
    return fore_hwnd

def get_ime_window(hwnd):
    """获取 IME 默认窗口句柄"""
    return imm32.ImmGetDefaultIMEWnd(hwnd)

def send_message_timeout(hwnd, msg, wparam, lparam, timeout_ms=500):
    """带超时的消息发送"""
    result = wintypes.DWORD()
    ret = user32.SendMessageTimeoutW(
        hwnd, msg, wparam, lparam,
        0x2,  # SMTO_ABORTIFHUNG
        timeout_ms,
        byref(result)
    )
    if ret:
        return result.value
    return None

def is_chinese_mode():
    """检测是否为中文输入模式"""
    hwnd = get_focused_window()
    ime_hwnd = get_ime_window(hwnd)
    if not ime_hwnd:
        return False
    
    # 获取 IME 开启状态
    open_status = send_message_timeout(ime_hwnd, WM_IME_CONTROL, IMC_GETOPENSTATUS, 0)
    if not open_status:
        return False
        
    # 获取转换模式并检测是否包含 NATIVE 标志 (中文)
    conversion_mode = send_message_timeout(ime_hwnd, WM_IME_CONTROL, IMC_GETCONVERSIONMODE, 0)
    if conversion_mode is not None:
        return bool(conversion_mode & IME_CMODE_NATIVE)
    return False

def get_foreground_process_name():
    """返回前台窗口所属进程名（小写，含 .exe），用于 SET 白名单校验。失败返回空串。"""
    hwnd = user32.GetForegroundWindow()
    if not hwnd:
        return ""
    pid = wintypes.DWORD()
    user32.GetWindowThreadProcessId(hwnd, byref(pid))
    if not pid.value:
        return ""
    PROCESS_QUERY_INFORMATION = 0x0400
    PROCESS_VM_READ = 0x0010
    h = _kernel32.OpenProcess(PROCESS_QUERY_INFORMATION | PROCESS_VM_READ, False, pid.value)
    if not h:
        return ""
    try:
        buf = ctypes.create_unicode_buffer(260)
        size = wintypes.DWORD(260)
        if _kernel32.QueryFullProcessImageNameW(h, 0, buf, byref(size)):
            return buf.value.rsplit("\\", 1)[-1].lower()
        return ""
    finally:
        _kernel32.CloseHandle(h)

def set_mode(chinese):
    """绝对设置中/英文输入模式。沿用 OPEN=1/CONV=1025（中文）、OPEN=0/CONV=1024（英文）。"""
    hwnd = get_focused_window()
    if not hwnd:
        return False
    ime_hwnd = get_ime_window(hwnd)
    if not ime_hwnd:
        return False
    open_status = 1 if chinese else 0
    conv_mode = 1025 if chinese else 1024
    r1 = send_message_timeout(ime_hwnd, WM_IME_CONTROL, IMC_SETOPENSTATUS, open_status)
    r2 = send_message_timeout(ime_hwnd, WM_IME_CONTROL, IMC_SETCONVERSIONMODE, conv_mode)
    return (r1 is not None) and (r2 is not None)
