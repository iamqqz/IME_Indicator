# CODEBUDDY.md

This file provides guidance to CodeBuddy Code when working with code in this repository.

## 概述

本仓库是一个 **Windows 平台的输入法（IME）中英状态指示器**：在文本光标或鼠标指针旁绘制一个彩色小圆点，指示当前是中文还是英文输入模式。仓库包含两份实现：

- **`go_indicator/` (推荐 / 主实现)**：Go + Win32 API（零 cgo），编译为单一独立 `.exe`（约 2.5MB，内嵌图标与 manifest），带系统托盘，并内置 **IPC（TCP loopback）常驻桥接**。这是日常使用和维护的对象。
- **`rust_indicator/` (参考实现)**：Rust 版，逻辑与 Go 版一致，作为对照/迁移来源。Go 版在 Windows 验证达标后可删除。
- **`python_indicator/` (参考实现)**：原始 Python + ctypes 实现，仅作为学习 Win32 API 调用的参考，不要求与其保持同步。

三个实现刻意保持一致的模块划分。修改 Go 版时可参考 Rust 版理解同一逻辑；Go 版的模块名与 Rust 一一对应。

## 重要：平台限制

本程序 **只能运行在 Windows 上**（直接调用 Win32 API：COM / UI Automation / IMM32 / GDI / 系统托盘）。

- **编译可在 WSL/Linux 上交叉完成**：`GOOS=windows GOARCH=amd64 go build`（详见下方“常用命令”）。Go 版不依赖 cgo，因此无需 Windows 工具链即可产出 exe；可在本环境用交叉编译验证“能否编译”。
- **运行时行为只能在 Windows 验证**：托盘 / 光标跟随 / OSC 12 联动等必须通过 Windows 手动运行验证，无法在本环境运行，仓库也无 CI。
- **无单元测试/集成测试**：改动后靠 Windows 上手动运行验证。可先用 `cmd/imeqiao --diag` 打印各级光标定位的 HRESULT 与来源，定位 UIA 失败层级。

## 常用命令（Go 版，须在 `go_indicator/` 目录执行）

```bash
# 仅交叉编译验证“能否编译”（不运行；运行须在 Windows）
GOOS=windows GOARCH=amd64 go build ./...

# 标准检查工具
gofmt -w .                 # 格式化（仓库要求提交前执行）
GOOS=windows GOARCH=amd64 go vet ./...   # 检查 unsafe.Pointer 误用等

# 产出完整 exe（生成资源 .syso 并交叉编译，含窗口图标/manifest）
./build.sh                 # 等价：make
./build.sh <输出目录>      # 指定产物目录，默认当前目录 IME-Indicator.exe

# 用 Make 等效目标
make            # 同 build.sh
make vet        # 交叉编译目标下 go vet
make fmt        # gofmt -w
make clean      # 删除生成的 .syso 与 exe

# 在 Windows 上调试运行（直接 cargo 等价：go run）
go run ./cmd/imeqiao
```

构建期依赖：
- 运行时依赖仅 `golang.org/x/sys/windows`（syscall 调 Win32/COM/GDI）。
- 构建期工具 `github.com/akavel/rsrc`（生成图标/manifest 的 `.syso`，由 `build.sh`/`make` 自动 `go run ...@latest` 拉取，仅进入模块缓存，不改动本机 go 安装）。
- 若 `go.mod` 的 `go` 指令要求高于本机 `go` 版本，Go 的 **auto-toolchain**（`GOTOOLCHAIN=auto`）会自动下载匹配的工具链到模块缓存——这是正常行为，不会覆盖用户已安装的 go，仅在编译该模块时临时启用。

`config.toml` 不被 `.gitignore` 跟踪；程序首次运行时会以 `Default()` 在 exe 同目录自动生成带注释的 `config.toml`（`config.go::generateTemplate`）。

## Go 版架构

入口 `cmd/imeqiao/main.go` 负责线程编排与子命令分发；其余逻辑在 `internal/` 各包，对应从“检测状态”到“绘制提示”再到“对外桥接”的完整数据通路。

### 子命令
- （无参数）= 常驻进程：检测器 goroutine + 系统托盘 `GetMessageW` 循环 + IPC 守护。
- `--client` = WSL 桥接子进程：连 `127.0.0.1:<port>`，把 stdin→TCP、TCP→stdout，供 WSL/nvim 复用。
- `--diag` = 打印 5 级光标定位 HRESULT 与来源（`LastSource` / `LastUIAError`）。
- `get|zh|en` = 兼容旧 `ime_bridge.exe` 的 CLI。

### 线程与并发模型（`main.go`）
- `func init(){ runtime.LockOSThread() }`：主 goroutine 锁线程跑系统托盘消息循环（Win32 窗口/消息循环需线程亲和）。
- **detector goroutine**：`runtime.LockOSThread()` + COM 初始化（STA），且永不 Unlock；持 `IUIAutomation` 实例，在同一线程创建/更新两个 overlay 窗口。采用单层轮询：
  - `poll.interval_ms`（默认 30ms）统一做 IME 状态检测、overlay 显隐判定与坐标追踪。
- **IPC 守护 goroutines**：不锁线程，纯 Go；`net.Listener.Accept` + 每连接一个 conn goroutine，`daemon.Hub` 广播状态变更。

### 配置系统（`internal/config/config.go`）
- 零依赖迷你 TOML 解析器（无 `toml`/`serde` 等价物，刻意控制二进制体积）。
- 通过 `sync.Once` 懒加载并全局缓存（`config.Get()`）。配置进程启动后**不热重载**，改 `config.toml` 需重启（托盘“重启程序”即为此设计）。
- 颜色以 `#RRGGBBAA` 字符串书写，解析为 `0xAARRGGBB` 的 `u32`（`ParseColor`）。
- 新增 `[ipc]` 段（Enable/Port/Bind/Token）与 `ForegroundWhiteList`（SET 安全白名单）。

### 检测模块
- **`imedetect` — 中英模式判定**：取前台窗口 → `ImmGetDefaultIMEWnd` → `SendMessageTimeoutW` 发 `WM_IME_CONTROL`：先 `IMC_GETOPENSTATUS` 确认开启，再 `IMC_GETCONVERSIONMODE` 查 `IME_CMODE_NATIVE` 位判中文。`SetMode(zh)` 沿用 `OPEN=1/CONV=1025`（中文）、`OPEN=0/CONV=1024`（英文）。`GetForegroundProcessName()` 供 SET 白名单校验。
- **`caretdetect` — 文本光标定位**：`GetCaretPos()` 返回 `(x, y, height)`，采用**五级回退**（与 Rust 一致），并记 `LastSource` / `LastUIAError`（形如 `Car:Range:XXXX`）：
  1. `GetGUIThreadInfo`（记事本等）；
  2. UI Automation `TextPattern2.GetCaretRange`（VS Code）；
  3. UI Automation `TextPattern.GetSelection`（Chrome）；
  4. IME 组合窗口 `ImmGetCompositionWindow`；
  5. MSAA `AccessibleObjectFromWindow(OBJID_CARET)` + `accLocation`，再回退 `GUITHREADINFO`。
  - COM 用手写 vtable（`win32/com.go` 的 `ComObject`/`Call`/`Release`）；SafeArray 用 `defer SafeArrayDestroy` 释放（**修复了 Rust 版的持续泄漏**）。
- **`cursordetect` — 鼠标形状判定**：构造时把 `mouse_target_cursors`（默认 `[32513 (I-Beam), 32512 (Normal)]`）用 `LoadCursorW` 解析为共享句柄集合；运行时用 `GetCursorInfo().hCursor` 比对。

### 渲染（`internal/overlay`）
- `IndicatorOverlay` 用 **预渲染的 32bpp 预乘 alpha DIB** + `UpdateLayeredWindow` 绘制抗锯齿圆点（**不依赖 GDI+**，纯 Win32，确定性更强；颜色已预乘）。
- layered / topmost / transparent / tool / no-activate 的 popup 窗口；每个 overlay 在 `Update()` 内用 `PeekMessageW` 自行泵消息，与托盘 `GetMessageW` 循环相互独立。
- `WNDPROC` 用 `windows.NewCallback` 且仅在包级 `var` 创建一次；**回调参数统一用 `uintptr`**（避免 `NewCallback` 对参数类型的限制），内部再转 `uint32`。

### 系统托盘（`internal/tray`）
- `TrayManager` 创建隐藏窗口并 `Shell_NotifyIconW` 注册图标。图标加载优先级：exe 同目录 `icon.ico` → 否则枚举嵌入 exe 资源中的 `RT_GROUP_ICON` 加载第一个（`icon.png` 不再用作托盘图标）→ 仍失败则回落系统默认应用图标。注意：rsrc 为组图标分配的资源 ID 不固定（manifest 占 1、组图标当前为 2），故 `loadIcon` 用 `EnumResourceNamesW` 枚举而非写死 ID；`assets/icon.ico`+`app.manifest` 由 `build.sh` 经 `rsrc` 编为 `cmd/imeqiao/rsrc_windows_amd64.syso`。
- 右键菜单：编辑配置 / 重启程序 / 关于 / 退出，分别调用 `ShellExecuteW` 打开文件或重启自身、`MessageBoxW` 显示 `assets/about.txt`。

### 常驻 IPC 桥接（`internal/daemon` + `internal/client`）
- `Server` 默认监听 `127.0.0.1:<port>`（`bind=loopback`）；`bind` 为 `wsl`/`all` 时监听 `0.0.0.0` 且强制 `token` 非空并校验来源网段。
- 行协议（TCP 与 `--client` stdio 复用同一套）：`PING`→`PONG`；`HELLO`-`OK ime-qiao <ver>`；`AUTH <token>`→`OK/ERR`；`GET`→`MODE zh|en`；`SET zh|en`→`OK/ERR`（默认校验前台进程名 ∈ `ForegroundWhiteList`，否则 `ERR not-foreground`）；`SUB`→`OK` 并随状态变更推送 `EVENT MODE zh|en`；`QUIT`→`BYE`。
- `Hub` 订阅/广播，写满则丢弃最旧事件并计数（非阻塞）。
- **WSL2 互通**：WSL2 是独立 VM，`127.0.0.1` 不互通。守护进程在 **Windows 侧**监听 loopback；WSL 内 nvim 用 `jobstart` 启动同 exe 的 `--client`（该子进程运行在 Windows 侧，连真正的 loopback）经 stdio 桥接。`/etc/resolv.conf` 的 nameserver 不可靠，故不走 WSL 直连 TCP。
- **OSC 12 落地**：外部 Windows 进程无法给 Windows Terminal 注入 OSC 12，须由终端内进程（nvim）写自身 stdout。故自渲染终端（按 exe 名 `windowsterminal.exe`/`conhost.exe` 判定，`main.go::isSelfRendering`）**不自绘悬浮点**，仅经 `SUB` 推送 IME 状态，由 nvim 现有 `set_cursor_style`（OSC 12）上色。

## 改动注意点（Go 版）

- **`internal/win32` 是 Win32 绑定的唯一出入口**：`golang.org/x/sys/windows` v0.47 未导出 `POINT/HICON/SIZE/MSG/WNDCLASSEX/GUID` 等类型，也未导出 `SyscallN`，故在 `handles.go`/`types.go` 自定义类型别名与结构体，`dll.go` 集中声明 `LazyProc`。新增 Win32 API 时：在 `dll.go` 加 `ProcXxx`，在 `types.go`/`handles.go` 加对应常量/结构体。
- **`LazyProc.Call` 返回 3 值 `(r1, r2, err)`**：单值比较（`if proc.Call(...) != 0`）会编译失败。统一用 `win32.CallOK(proc, args...)`（判断 `r1 != 0`）与 `win32.CallR(proc, args...)`（取 `r1`）。
- **COM 与 `unsafe.Pointer`**：vtable 调用在 `win32/com.go` 手写；`go vet` 会报 `com.go:24`（从 `CoCreateInstance` 输出的 `uintptr` 读取 vtable）为 “possible misuse of unsafe.Pointer”——这是手写 COM 绑定的固有、安全的误报，不影响构建（exit 0）。其余 SafeArray 读取已改写为 vet 认可的指针算术形式。
- **运行时验证必须到 Windows**；涉及 UI Automation / 光标定位的行为差异需针对具体应用（VS Code、Chrome、记事本、Windows Terminal）实测。
- **新增配置项**需同步改四处：`Config` 结构体、`Default()`、`loadConfig()` 的解析分支（注意 section 名已小写为 `poll/tray/caret/mouse/ipc`）、`generateTemplate()` 的带注释示例。
- **图标/manifest 改动**：编辑 `assets/icon.svg`/`icon.ico`/`app.manifest` 后需重新跑 `build.sh`（重新生成 `.syso`）；用户自定义图标可把 `icon.png` 放到 exe 同目录。

## 改动注意点（Rust 版，仅作参考）

- 修改 Rust 版后必须到 **Windows** 用 `cargo run` 验证，无法在本环境编译；涉及 UI Automation / 光标定位的行为差异需针对具体应用实测。
- 新增配置项时，需同时改 `Config` 结构体、`Default`、`load_config()` 的解析分支、`config::xxx()` 访问器，并在 `generate_toml_template()` 中加入带注释的示例。
- 图标相关改动：编辑 `assets/icon.svg`/`icon.ico` 后需重新编译（`build.rs` 调用 `embed-resource` 编译 `assets/resource.rc`）；要支持用户自定义，将 `icon.png` 放在 exe 同目录即可。
