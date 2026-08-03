"""
系统托盘：ctypes 直接调 Shell_NotifyIconW（零额外依赖）。
- 隐藏窗口 + 消息循环（独立线程）
- 右键菜单：编辑配置 / 重启程序 / 关于 / 退出
- stop_event 置位时退出消息循环
"""
import os
import sys
import ctypes
from ctypes import wintypes, byref, sizeof, WINFUNCTYPE, Structure, POINTER

from win32_api import user32

# 常量
WM_TRAYICON = 0x0400 + 1
WM_COMMAND = 0x0111
WM_DESTROY = 0x0002
WM_RBUTTONUP = 0x0205

NIM_ADD = 0
NIM_DELETE = 1
NIF_ICON = 0x0002
NIF_MESSAGE = 0x0001
NIF_TIP = 0x0004

MF_STRING = 0x0000
MF_SEPARATOR = 0x0800
TPM_LEFTALIGN = 0x0000
TPM_BOTTOMALIGN = 0x0020

IDM_CONFIG = 1001
IDM_RESTART = 1002
IDM_ABOUT = 1003
IDM_EXIT = 1004

IMAGE_ICON = 1
LR_LOADFROMFILE = 0x0010
LR_DEFAULTSIZE = 0x0040
IDI_APPLICATION = 32512

MB_ICONINFORMATION = 0x00000040
MB_OK = 0x00000000


class NOTIFYICONDATA(Structure):
    _fields_ = [
        ("cbSize", wintypes.DWORD),
        ("hWnd", wintypes.HWND),
        ("uID", wintypes.UINT),
        ("uFlags", wintypes.UINT),
        ("uCallbackMessage", wintypes.UINT),
        ("hIcon", wintypes.HANDLE),
        ("szTip", wintypes.WCHAR * 128),
        ("_pad", ctypes.c_byte * 384),
    ]


class WNDCLASSEX(Structure):
    _fields_ = [
        ("cbSize", wintypes.UINT),
        ("style", wintypes.UINT),
        ("lpfnWndProc", WINFUNCTYPE(ctypes.c_longlong, wintypes.HWND, wintypes.UINT, wintypes.WPARAM, wintypes.LPARAM)),
        ("cbClsExtra", wintypes.INT),
        ("cbWndExtra", wintypes.INT),
        ("hInstance", wintypes.HINSTANCE),
        ("hIcon", wintypes.HANDLE),
        ("hCursor", wintypes.HANDLE),
        ("hbrBackground", wintypes.HBRUSH),
        ("lpszMenuName", wintypes.LPCWSTR),
        ("lpszClassName", wintypes.LPCWSTR),
        ("hIconSm", wintypes.HANDLE),
    ]


def _load_icon(script_dir):
    """优先加载脚本目录的 icon.ico，否则用系统应用图标。"""
    icon_path = os.path.join(script_dir, "icon.ico")
    if os.path.exists(icon_path):
        h = user32.LoadImageW(0, icon_path, IMAGE_ICON, 0, 0, LR_LOADFROMFILE | LR_DEFAULTSIZE)
        if h:
            return h
    return user32.LoadIconW(0, IDI_APPLICATION)


class TrayManager:
    def __init__(self, stop_event, script_dir):
        self.stop_event = stop_event
        self.script_dir = script_dir
        self.hwnd = None
        self._wnd_proc = None
        self._wnd_class = None

    def run(self):
        WNDPROC = WINFUNCTYPE(ctypes.c_longlong, wintypes.HWND, wintypes.UINT, wintypes.WPARAM, wintypes.LPARAM)

        def wnd_proc(hwnd, msg, wparam, lparam):
            if msg == WM_TRAYICON and lparam == WM_RBUTTONUP:
                self._show_menu(hwnd)
            elif msg == WM_COMMAND:
                self._on_command(wparam & 0xFFFF)
            elif msg == WM_DESTROY:
                pass
            return user32.DefWindowProcW(hwnd, msg, wparam, lparam)

        self._wnd_proc = WNDPROC(wnd_proc)

        h_inst = ctypes.windll.kernel32.GetModuleHandleW(0)
        cls = WNDCLASSEX()
        cls.cbSize = sizeof(WNDCLASSEX)
        cls.lpfnWndProc = self._wnd_proc
        cls.hInstance = h_inst
        cls.lpszClassName = "IMEIndicatorTrayClass"
        user32.RegisterClassExW(byref(cls))
        self._wnd_class = cls  # 保活

        hwnd = user32.CreateWindowExW(
            0, cls.lpszClassName, "IME Indicator Tray", 0,
            0, 0, 0, 0, None, None, h_inst, None
        )
        self.hwnd = hwnd

        nid = NOTIFYICONDATA()
        nid.cbSize = sizeof(NOTIFYICONDATA)
        nid.hWnd = hwnd
        nid.uID = 1
        nid.uFlags = NIF_ICON | NIF_MESSAGE | NIF_TIP
        nid.uCallbackMessage = WM_TRAYICON
        nid.hIcon = _load_icon(self.script_dir)
        nid.szTip = "输入指示器 (IME Indicator)"  # 直接赋值，ctypes 自动填充并补结束符
        ctypes.windll.shell32.Shell_NotifyIconW(NIM_ADD, byref(nid))
        self._nid = nid  # 保活

        # 消息循环（独立线程运行）
        msg = wintypes.MSG()
        while user32.GetMessageW(byref(msg), 0, 0, 0):
            user32.TranslateMessage(byref(msg))
            user32.DispatchMessageW(byref(msg))

    def _show_menu(self, hwnd):
        menu = user32.CreatePopupMenu()
        user32.AppendMenuW(menu, MF_STRING, IDM_CONFIG, "编辑配置")
        user32.AppendMenuW(menu, MF_STRING, IDM_RESTART, "重启程序")
        user32.AppendMenuW(menu, MF_STRING, IDM_ABOUT, "关于")
        user32.AppendMenuW(menu, MF_SEPARATOR, 0, None)
        user32.AppendMenuW(menu, MF_STRING, IDM_EXIT, "退出")
        user32.SetForegroundWindow(hwnd)
        pt = wintypes.POINT()
        user32.GetCursorPos(byref(pt))
        user32.TrackPopupMenu(menu, TPM_LEFTALIGN | TPM_BOTTOMALIGN, pt.x, pt.y, 0, hwnd, None)
        user32.DestroyMenu(menu)

    def _on_command(self, cmd_id):
        if cmd_id == IDM_CONFIG:
            cfg_path = os.path.join(self.script_dir, "config.py")
            try:
                os.startfile(cfg_path)
            except Exception:
                pass
        elif cmd_id == IDM_ABOUT:
            user32.MessageBoxW(None, "IME Indicator\n输入法中英状态指示器", "关于", MB_ICONINFORMATION | MB_OK)
        elif cmd_id == IDM_RESTART:
            self.stop_event.set()
            # 重启：用同一解释器重跑当前进程
            try:
                os.execv(sys.executable, [sys.executable] + sys.argv)
            except Exception:
                pass
        elif cmd_id == IDM_EXIT:
            self.stop_event.set()
            user32.PostQuitMessage(0)

    def destroy(self):
        if self.hwnd:
            try:
                ctypes.windll.shell32.Shell_NotifyIconW(NIM_DELETE, byref(self._nid))
            except Exception:
                pass
            user32.DestroyWindow(self.hwnd)
