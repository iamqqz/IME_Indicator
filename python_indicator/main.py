"""
IME Indicator（输入法中英状态指示器）入口。

子命令：
  （无参数）       常驻进程：检测器 + 托盘 + 常驻 IPC
  get|zh|en       一次性查询/设置（WSL 的“exe”桥接方式用）
  --client        stdio <-> TCP 中继子进程（WSL 为 NAT 网络模式时使用）

说明：常驻进程在 Windows 侧监听 TCP 127.0.0.1:<port>。WSL 内 nvim 的接入方式取决于
.wslconfig 的 networkingMode：mirrored（+ hostAddressLoopback）下 WSL 与 Windows 共享
loopback，nvim 可用 sockconnect 直连本端口，无需中继；NAT（默认）下 127.0.0.1 不互通，
须用同脚本的 --client 子进程（本身运行在 Windows 侧）做 stdio 中继。
对所有“拿不到文本光标位置”的窗口（含 Windows Terminal 等自渲染终端）不特殊处理，
统一走兜底：在鼠标位置绘制状态标记；同时经 IPC 广播 IME 状态，供 nvim 自行上色。
"""
import os
import sys
import time
import ctypes
import threading
import socket

from ctypes import wintypes, byref

from win32_api import user32
import config
import ime_detector
import caret_detector as caret_detector_mod
import cursor_detector as cursor_detector_mod
import overlay
import tray as tray_mod
import ipc_server

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))


def run_oneshot(cmd):
    """一次性 get/zh/en，供 WSL 的 exe 桥接方式调用。"""
    if cmd == "get":
        print("chinese" if ime_detector.is_chinese_mode() else "english")
    elif cmd == "zh":
        print("set chinese ok" if ime_detector.set_mode(True) else "set chinese failed")
    elif cmd == "en":
        print("set english ok" if ime_detector.set_mode(False) else "set english failed")


def run_client(cfg):
    """--client：连本机 loopback IPC，把 stdin 转发给守护、守护响应经 stdout 回传。"""
    s = socket.create_connection(("127.0.0.1", cfg.IPC_PORT))
    if cfg.IPC_BIND != "loopback" and cfg.IPC_TOKEN:
        s.sendall(("AUTH " + cfg.IPC_TOKEN + "\n").encode("utf-8"))

    def pipe_in():
        data = sys.stdin.buffer.read(1)
        while data:
            try:
                s.sendall(data)
            except OSError:
                break
            data = sys.stdin.buffer.read(1)
        try:
            s.shutdown(socket.SHUT_WR)
        except OSError:
            pass

    threading.Thread(target=pipe_in, daemon=True).start()
    while True:
        data = s.recv(4096)
        if not data:
            break
        sys.stdout.buffer.write(data)
        sys.stdout.buffer.flush()
    sys.exit(0)


def main():
    # 高 DPI 感知
    try:
        ctypes.windll.shcore.SetProcessDpiAwareness(2)  # PROCESS_PER_MONITOR_DPI_AWARE
    except Exception:
        try:
            user32.SetProcessDPIAware()
        except Exception:
            pass

    # 单实例（命名 Mutex）
    mutex = ctypes.windll.kernel32.CreateMutexW(None, False, "Global\\IMEQiao")
    if ctypes.windll.kernel32.GetLastError() == 183:  # ERROR_ALREADY_EXISTS
        print("IME Indicator 已在运行")
        sys.exit(0)

    hub = ipc_server.Hub()
    server = ipc_server.Server(config, hub).start()

    stop_event = threading.Event()

    # 初始化检测器
    caret_det = caret_detector_mod.CaretDetector()
    cursor_det = cursor_detector_mod.CursorDetector(config.MOUSE_TARGET_CURSORS)

    caret_overlay = None
    if config.CARET_ENABLE:
        caret_overlay = overlay.IndicatorOverlay(
            "Caret", config.CARET_SIZE, config.CARET_COLOR_CN,
            config.CARET_COLOR_EN, config.CARET_OFFSET_X, config.CARET_OFFSET_Y,
        )
    mouse_overlay = None
    # 鼠标 overlay：MOUSE_MODE 为 follow / fallback 时都需要在鼠标位置绘制
    if config.MOUSE_MODE != "off":
        mouse_overlay = overlay.IndicatorOverlay(
            "Mouse", config.MOUSE_SIZE, config.MOUSE_COLOR_CN,
            config.MOUSE_COLOR_EN, config.MOUSE_OFFSET_X, config.MOUSE_OFFSET_Y,
        )

    print("IME Indicator (输入法综合状态提示) 已启动。")
    if config.TRAY_ENABLE:
        print(" - 系统托盘: 已启用")
    if config.CARET_ENABLE:
        print(f" - 文本光标提示: 已启用 (大小:{config.CARET_SIZE})")
    if config.MOUSE_MODE != "off":
        print(f" - 鼠标标记: 模式={config.MOUSE_MODE} (大小:{config.MOUSE_SIZE})")
    if server:
        print(f" - IPC 监听: 127.0.0.1:{config.IPC_PORT}")
    print("托盘右键菜单可退出；或按 Ctrl+C。")

    # 托盘线程（独立消息循环）
    tray_thread = None
    if config.TRAY_ENABLE:
        tray = tray_mod.TrayManager(stop_event, SCRIPT_DIR)
        tray_thread = threading.Thread(target=tray.run, daemon=True)
        tray_thread.start()

    last_state = 0.0
    chinese = False
    prev_mode = None
    caret_active = False
    mouse_active = False

    try:
        while not stop_event.is_set():
            now = time.time()

            # --- A. 状态检测 ---
            if now - last_state >= config.STATE_POLL_INTERVAL:
                chinese = ime_detector.is_chinese_mode()

                # 能否拿到文本光标位置：直接实测（Windows Terminal 等自渲染终端
                # 会自然返回 None，无需特殊处理，统一走下面的鼠标兜底）。
                cp = None
                if config.CARET_ENABLE:
                    cp = caret_det.get_caret_pos()
                caret_unavailable = (cp is None)

                # Caret：能拿到就在光标处画
                if config.CARET_ENABLE:
                    should = (not caret_unavailable) and (chinese or config.CARET_SHOW_EN)
                    if should != caret_active:
                        caret_active = should
                        if should:
                            caret_overlay.show()
                        else:
                            caret_overlay.hide()

                # Mouse：由 MOUSE_MODE 决定以哪种方式在鼠标位置显示
                #  follow  —— 跟随鼠标：悬停在目标光标形状上时显示
                #  fallback—— 兜底：拿不到文本光标时（含 Windows Terminal 等）显示
                if mouse_overlay is not None:
                    follow = (config.MOUSE_MODE == "follow"
                              and cursor_det.is_target_cursor()
                              and (chinese or config.MOUSE_SHOW_EN))
                    fallback = (config.MOUSE_MODE == "fallback"
                                and config.CARET_ENABLE
                                and caret_unavailable
                                and (chinese or config.CARET_SHOW_EN))
                    should = follow or fallback
                    if should != mouse_active:
                        mouse_active = should
                        if should:
                            mouse_overlay.show()
                        else:
                            mouse_overlay.hide()

                # 状态变化 -> 广播
                mode_changed = chinese != prev_mode
                if mode_changed:
                    prev_mode = chinese
                    if server:
                        server.broadcast_mode(chinese)

                last_state = now

            # --- B. 坐标追踪 ---
            if config.CARET_ENABLE and caret_active:
                cp = caret_det.get_caret_pos()
                if cp:
                    caret_overlay.update(cp[0], cp[1], chinese, cp[2])
            if mouse_overlay is not None and mouse_active:
                m_pt = wintypes.POINT()
                if user32.GetCursorPos(byref(m_pt)):
                    mouse_overlay.update(m_pt.x, m_pt.y, chinese)

            time.sleep(config.TRACK_POLL_INTERVAL)
    except KeyboardInterrupt:
        print("\n正在停止...")
    finally:
        if server:
            server.close()
        if caret_overlay:
            caret_overlay.cleanup()
        if mouse_overlay:
            mouse_overlay.cleanup()
        stop_event.set()
        try:
            user32.PostQuitMessage(0)
        except Exception:
            pass


USAGE = """用法: python main.py [子命令]

  （无参数）    常驻进程：检测器 + 托盘 + 常驻 IPC
  get|zh|en    一次性查询/设置输入法状态
  --client     stdio <-> TCP 中继子进程（WSL 为 NAT 网络模式时使用）
"""


if __name__ == "__main__":
    if len(sys.argv) > 1:
        arg = sys.argv[1]
        if arg in ("get", "zh", "en"):
            run_oneshot(arg)
            sys.exit(0)
        if arg == "--client":
            run_client(config)
            sys.exit(0)
        # 未知参数不得落到常驻模式：否则拼错子命令会静默多出一个托盘进程
        sys.stderr.write("未知参数: %s\n\n%s" % (arg, USAGE))
        sys.exit(2)
    main()
