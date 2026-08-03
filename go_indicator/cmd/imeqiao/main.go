// IME Indicator 入口。
// 默认以常驻进程运行（检测器线程 + 系统托盘消息循环 + 常驻 IPC）。
// 子命令：get|zh|en（兼容旧 CLI）、--client（WSL 桥接）、--diag（caret 诊断）。
package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"imeqiao/internal/caretdetect"
	"imeqiao/internal/client"
	"imeqiao/internal/config"
	"imeqiao/internal/cursordetect"
	"imeqiao/internal/daemon"
	"imeqiao/internal/imedetect"
	"imeqiao/internal/overlay"
	"imeqiao/internal/tray"
	"imeqiao/internal/win32"
)

func init() {
	// 将主 goroutine 绑定到 OS 线程：托盘/菜单在主线程最稳。
	runtime.LockOSThread()
}

const usage = `IME Indicator —— Windows 输入法状态指示与 nvim 桥接

用法:
  IME-Indicator.exe              以常驻进程运行（检测器 + 托盘 + IPC 服务端）
  IME-Indicator.exe get          打印当前输入法状态：chinese | english
  IME-Indicator.exe zh | en      设置输入法为中文 / 英文
  IME-Indicator.exe --client     stdio <-> TCP 中继（WSL networkingMode 非 mirrored 时用）
  IME-Indicator.exe --diag       文本光标检测诊断
  IME-Indicator.exe --help       显示本帮助
`

func main() {
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "get", "zh", "en":
			legacyCLI(args[0])
			return
		case "--client":
			if err := client.Run(config.Get()); err != nil {
				fmt.Fprintln(os.Stderr, "client error:", err)
				os.Exit(1)
			}
			return
		case "--diag":
			runDiag()
			return
		case "--help", "-h":
			fmt.Print(usage)
			return
		default:
			// 不能静默 fallthrough 到常驻模式：调用方（nvim 兜底路径）拼错子命令时
			// 会误启一个 daemon，且拿不到任何返回值，故障表现为「无响应」而非报错。
			fmt.Fprintf(os.Stderr, "未知参数: %s\n\n%s", args[0], usage)
			os.Exit(2)
		}
	}
	runDaemon()
}

// legacyCLI 兼容旧 ime_bridge.exe 的子命令
func legacyCLI(cmd string) {
	switch cmd {
	case "get":
		if imedetect.IsChineseMode() {
			fmt.Println("chinese")
		} else {
			fmt.Println("english")
		}
	case "zh":
		if imedetect.SetMode(true) {
			fmt.Println("set chinese ok")
		} else {
			fmt.Println("set chinese failed")
		}
	case "en":
		if imedetect.SetMode(false) {
			fmt.Println("set english ok")
		} else {
			fmt.Println("set english failed")
		}
	}
}

func runDiag() {
	d := caretdetect.New()
	defer d.Cleanup()
	for i := 0; i < 5; i++ {
		pos, ok := d.GetCaretPos()
		fmt.Printf("iter %d source=%v ok=%v pos=%v err=%s\n", i, d.LastSource, ok, pos, d.LastUIAError)
		time.Sleep(300 * time.Millisecond)
	}
	fmt.Println("chinese:", imedetect.IsChineseMode())
}

func setDpiAwareness() {
	if r, _, _ := win32.ProcSetProcessDpiAwareness.Call(2); r != 0 {
		win32.ProcSetProcessDPIAware.Call()
	}
}

var gMutex windows.Handle

func ensureSingleInstance() bool {
	name, _ := windows.UTF16PtrFromString("Global\\IMEQiao")
	h, _, err := win32.ProcCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return false
	}
	gMutex = windows.Handle(h)
	if err == windows.ERROR_ALREADY_EXISTS {
		return false
	}
	return true
}

// releaseInstance 释放全局互斥体，允许新实例（如托盘"重启程序"启动的）成功创建。
func releaseInstance() {
	if gMutex != 0 {
		windows.ReleaseMutex(gMutex)
		windows.CloseHandle(gMutex)
		gMutex = 0
	}
}

// isSelfRendering 判断前台是否为自渲染宿主（拿不到 Win32 文本光标位置，需走 IPC 状态推送 +
// 鼠标兜底标记）。Neovide 用 GPU 自绘光标，不创建 Win32 caret，与终端同属此类。
func isSelfRendering() bool {
	switch imedetect.GetForegroundProcessName() {
	case "windowsterminal.exe", "conhost.exe", "neovide.exe":
		return true
	}
	return false
}

// printStartupStatus 打印常驻进程的启用状态（无控制台时静默丢弃）
func printStartupStatus(cfg *config.Config, server *daemon.Server) {
	fmt.Println("IME Indicator 已启动")
	fmt.Printf(" - 托盘: %v\n", cfg.TrayEnable)
	fmt.Printf(" - 文本光标: %v\n", cfg.CaretEnable)
	if cfg.MouseMode == "off" {
		fmt.Println(" - 鼠标标记: 关闭")
	} else {
		fmt.Printf(" - 鼠标标记: 模式=%s (大小:%d)\n", cfg.MouseMode, cfg.MouseSize)
	}
	if server != nil {
		fmt.Printf(" - IPC: %s (bind=%s)\n", server.Addr(), cfg.IPCBind)
	} else {
		fmt.Println(" - IPC: 未启用")
	}
	if len(cfg.ForegroundWhiteList) > 0 {
		fmt.Printf(" - SET 白名单: %v\n", cfg.ForegroundWhiteList)
	}
}

func runDaemon() {
	setDpiAwareness()
	if !ensureSingleInstance() {
		fmt.Println("IME Indicator 已在运行")
		os.Exit(0)
	}

	cfg := config.Get()
	hub := daemon.NewHub()
	server, err := daemon.Start(cfg, hub)
	if err != nil {
		fmt.Fprintln(os.Stderr, "IPC 启动失败:", err)
	}

	printStartupStatus(cfg, server)

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)

	// 检测器线程（锁定线程 + COM）
	go func() {
		runtime.LockOSThread()
		runDetector(cfg, server, done)
		wg.Done()
	}()

	if cfg.TrayEnable {
		t := tray.New(releaseInstance)
		defer t.Destroy()
		t.RunMessageLoop() // 阻塞，直到退出
	} else {
		// 无托盘：等待进程信号（SIGINT/SIGTERM）后退出，避免 select{} 死等导致无法清理
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
	}

	close(done)
	wg.Wait()
	if server != nil {
		server.Close()
	}
}

func runDetector(cfg *config.Config, server *daemon.Server, done chan struct{}) {
	caretDetector := caretdetect.New()
	defer caretDetector.Cleanup()

	cursorDetector := cursordetect.New(cfg.MouseTargetCursors)

	var caretOverlay, mouseOverlay *overlay.IndicatorOverlay
	if cfg.CaretEnable {
		caretOverlay = overlay.New("Caret", cfg.CaretSize, cfg.CaretColorCN, cfg.CaretColorEN, cfg.CaretOffsetX, cfg.CaretOffsetY)
		defer caretOverlay.Cleanup()
	}
	if cfg.MouseMode != "off" {
		mouseOverlay = overlay.New("Mouse", cfg.MouseSize, cfg.MouseColorCN, cfg.MouseColorEN, cfg.MouseOffsetX, cfg.MouseOffsetY)
		defer mouseOverlay.Cleanup()
	}

	stateInterval := time.Duration(cfg.PollStateIntervalMS) * time.Millisecond
	trackInterval := time.Duration(cfg.PollTrackIntervalMS) * time.Millisecond

	lastState := time.Now()
	var chineseMode bool
	caretActive := false
	mouseActive := false
	prevMode := false

	for {
		select {
		case <-done:
			return
		default:
		}

		now := time.Now()
		if now.Sub(lastState) >= stateInterval {
			chineseMode = imedetect.IsChineseMode()

			// caret
			caretUnavailable := false
			if cfg.CaretEnable && caretOverlay != nil {
				if isSelfRendering() {
					// 自渲染终端(Windows Terminal 等)拿不到文本光标位置
					caretUnavailable = true
					if caretActive {
						caretActive = false
						caretOverlay.Hide()
					}
				} else {
					_, ok := caretDetector.GetCaretPos()
					caretUnavailable = !ok
					should := ok && (chineseMode || cfg.CaretShowEN)
					if should != caretActive {
						caretActive = should
						if should {
							caretOverlay.Show()
						} else {
							caretOverlay.Hide()
						}
					}
				}
			}

			// mouse：由 MouseMode 决定以哪种方式在鼠标位置显示
			//  off      —— 不显示
			//  follow   —— 跟随鼠标：悬停在目标光标形状上时显示
			//  fallback —— 兜底：拿不到文本光标位置(含自渲染终端)时显示
			if mouseOverlay != nil {
				var should bool
				switch cfg.MouseMode {
				case "follow":
					should = cursorDetector.IsTargetCursor() && (chineseMode || cfg.MouseShowEN)
				case "fallback":
					should = cfg.CaretEnable && caretUnavailable && (chineseMode || cfg.CaretShowEN)
				default: // off 或其它
					should = false
				}
				if should != mouseActive {
					mouseActive = should
					if should {
						mouseOverlay.Show()
					} else {
						mouseOverlay.Hide()
					}
				}
			}

			if chineseMode != prevMode {
				prevMode = chineseMode
				if server != nil {
					server.BroadcastMode(chineseMode)
				}
			}
			lastState = now
		}

		// 坐标追踪
		if cfg.CaretEnable && caretActive && caretOverlay != nil {
			if pos, ok := caretDetector.GetCaretPos(); ok {
				caretOverlay.Update(pos[0], pos[1], chineseMode, pos[2])
			}
		}
		if mouseOverlay != nil && mouseActive {
			var pt win32.POINT
			win32.ProcGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
			mouseOverlay.Update(pt.X, pt.Y, chineseMode, 0)
		}

		time.Sleep(trackInterval)
	}
}
