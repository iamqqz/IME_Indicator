# 输入指示器 (IME Indicator)

一个轻量级的 Windows 输入法中英状态实时提示工具。在光标和鼠标底部用彩色小点指示中英状态，极尽简洁克制但有效的提示。

![demo](./assets/demo.png)

![demo](./assets/demo.gif)

## 核心特性

- **光标跟随**：在文本光标下方显示彩色指示球（支持记事本、VS Code、Chrome 等主流软件）。
- **鼠标跟随**：在鼠标指针旁显示指示球，支持特定形状（如 I-Beam）触发。
- **系统托盘**：后台静默运行，右键菜单可编辑配置、重启、退出。
- **WSL / nvim 桥接**：内置 IPC 常驻服务，WSL 内 nvim 可通过 `IME-Indicator.exe --client` 获取 IME 状态，配合 OSC 12 实现终端光标颜色联动。
- **零依赖**：Go 实现编译为单一 `.exe`，内嵌图标与 manifest，拷贝即用。

## 项目结构

本仓库包含三个版本的实现：

- **[go_indicator/](./go_indicator/) (当前维护)**：
  - 使用 Go + Win32 API 开发，零 cgo。
  - 单文件运行：编译后为单个独立 `.exe` 文件（约 2.9MB，内嵌图标与 manifest）。
  - 系统托盘 + 常驻 IPC 桥接 + 日志诊断。

- **[rust_indicator/](./rust_indicator/) (上游实现)**：
  - 使用 Rust + Win32 API 开发。
  - 编译后约 300KB，内嵌图标。

- **[python_indicator/](./python_indicator/) (参考)**：
  - 原始的 Python + ctypes 实现，适合学习 Win32 API 调用参考。

## 直接运行

到 [Releases](https://github.com/iamqqz/IME_Indicator/releases) 下载已编译好的 `IME-Indicator.exe`，双击运行即可。

## 自行编译 (Go 版，推荐)

```bash
cd go_indicator
./build.sh            # 生成 IME-Indicator.exe (2.9MB)
```

或 `make`（等效）。交叉编译仅在 Linux/WSL 验证能否编译，运行时须在 Windows。

编译 Rust 版见上游文档。

## 开机自启动

右键 `install-autostart.ps1` → "使用 PowerShell 运行"（首次需 `Set-ExecutionPolicy -Scope CurrentUser RemoteSigned`）。

或手工：`Win + R` → `shell:startup` → 新建快捷方式指向 `IME-Indicator.exe`。

## 本 Fork 的主要改动

相对于上游 (HaujetZhao/IME_Indicator)：

- **Go 重写** (`go_indicator/`)：独立实现，与 Rust 版模块一一对应，零 cgo、纯 Win32。
- **常驻 IPC 桥接**：TCP loopback 服务端 + `--client` 子命令，支持 WSL 内 nvim 获取 IME 状态并通过 OSC 12 改终端光标颜色。
- **系统托盘图标修复**：托盘图标改用 `EnumResourceNamesW` 枚举嵌入的 `RT_GROUP_ICON`，避免 rsrc 资源 ID 分配变化导致图标不显示。
- **悬浮窗稳定性**：每帧用 `GetDC`/`ReleaseDC` + `SetWindowPos(SWP_SHOWWINDOW)` 保持分层窗口在 DWM 合成树中，修复切窗口后圆点消失的问题。
- **日志诊断**：基于 `log/slog` 的文件日志，默认写 exe 同目录 `ime.log`（带 2MB 轮转），用于排查悬浮窗/光标定位等不稳定问题。
- **轮询合并**：IME 状态检测与坐标追踪合并为单一 `interval_ms`（默认 30ms），去掉双层定时器，降低 CPU 占用。
- **开机自启动脚本**：`install-autostart.ps1` 一键创建 Startup 快捷方式。

---

上游作者：Antigravity & Haujet  
上游仓库：[HaujetZhao/IME_Indicator](https://github.com/HaujetZhao/IME_Indicator)
