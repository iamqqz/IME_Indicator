from win32_api import OCR_IBEAM, OCR_NORMAL

# ============ 通用设置 ============
# 状态检测间隔 (秒)
STATE_POLL_INTERVAL = 0.1  # 100ms
# 位置追踪间隔 (秒)
TRACK_POLL_INTERVAL = 0.01 # 10ms

# ============ 1. 文本光标提示 (Caret Indicator) ============
CARET_ENABLE = True           # 是否启用光标提示
CARET_COLOR_CN = "#FF7800A0"  # 中文颜色 (橙色, 透明度 A0)
CARET_COLOR_EN = "#0078FF30"  # 英文颜色 (蓝色, 透明度 30)
CARET_SIZE = 8                # 提示器大小
CARET_OFFSET_X = 0            # 提示器 X 偏移
CARET_OFFSET_Y = 0            # 提示器 Y 偏移 (为 0 时紧贴光标底部)
CARET_SHOW_EN = True          # 英文状态下是否也显示

# ============ 2. 鼠标标记 (Mouse Indicator) ============
# 鼠标位置的输入法状态标记模式（三选一）：
#   "off"      —— 不显示鼠标标记
#   "follow"   —— 跟随鼠标：鼠标悬停在目标光标形状(I-Beam/箭头)上时显示
#   "fallback" —— 兜底：拿不到文本光标位置(如 Windows Terminal)时，在鼠标位置显示
MOUSE_MODE = "fallback"        # off | follow | fallback
MOUSE_COLOR_CN = "#FF7800C8"  # 中文颜色
MOUSE_COLOR_EN = "#0078FF30"  # 英文颜色
MOUSE_SIZE = 8                # 提示器大小
MOUSE_OFFSET_X = 0            # 提示器 X 偏移
MOUSE_OFFSET_Y = 24           # 提示器 Y 偏移
MOUSE_SHOW_EN = True         # 英文状态下是否也显示

# 仅在以下鼠标形状时显示 (OCR_IBEAM: I型光标, OCR_NORMAL: 标准箭头)
MOUSE_TARGET_CURSORS = [OCR_IBEAM, OCR_NORMAL]

# ============ 3. 托盘 ============
TRAY_ENABLE = True           # 是否显示系统托盘图标（false 时完全后台，只能任务管理器结束）

# ============ 4. 常驻 IPC 桥接 ============
# 默认在 Windows 侧监听 TCP loopback。
# WSL 内 nvim 的接入方式取决于 .wslconfig 的 networkingMode：
#   mirrored（+ hostAddressLoopback）：WSL 内可直连 127.0.0.1，nvim 用 sockconnect 直接连本端口；
#   NAT（默认）：127.0.0.1 不互通，需用同脚本的 --client 子进程做 stdio↔TCP 中继。
IPC_ENABLE = True
IPC_PORT = 51234  # 原 45123 在本机被系统预留，无法绑定；51234 实测可用
IPC_BIND = "loopback"         # loopback | wsl | all（非 loopback 需设 TOKEN）
IPC_TOKEN = ""

# SET 安全白名单：仅当焦点窗口进程名（小写）在此列表时才允许设置输入法，
# 否则返回 ERR not-foreground（防止 nvim 后台 SET 改到别的窗口）。
FOREGROUND_WHITELIST = ["windowsterminal.exe", "wsl.exe", "conhost.exe", "neovide.exe"]
