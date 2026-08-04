// 文件日志子系统（基于标准库 log/slog，零新增依赖）。
//
// 发布版用 -H windowsgui 编译，stdout/stderr 被丢弃，因此所有诊断必须落盘。
// 日志默认写入 %LOCALAPPDATA%\IME-Indicator\ime.log；可通过 [log] 配置段覆盖。
// 多条 goroutine 并发写同一文件，故底层 writer 用互斥锁串行化。
package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
)

var (
	mu     sync.Mutex
	file   *os.File
	logger *slog.Logger
)

// Options 日志初始化参数
type Options struct {
	Enabled bool   // 是否启用（false 时丢弃）
	Level   string // debug | info | warn | error，默认 info
	Path    string // 自定义日志路径，空则用默认位置
}

const (
	defaultSubdir = "IME-Indicator"
	defaultName   = "ime.log"
	maxSize       = 2 << 20 // 2MB：超过则重命名为 .old 再新建
)

// syncWriter 串行化写入，避免并发写同一文件时行交错
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

func init() {
	// 默认丢弃，避免未 Init 时调用产生 nil panic
	logger = slog.New(slog.NewTextHandler(io.Discard, nil))
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Init 打开日志文件并初始化全局 logger。失败则回落到 stderr（GUI 下 stderr 丢弃，但至少不崩）。
func Init(opts Options) {
	if !opts.Enabled {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		return
	}

	path := opts.Path
	if path == "" {
		local := os.Getenv("LOCALAPPDATA")
		if local == "" {
			local = os.TempDir()
		}
		path = filepath.Join(local, defaultSubdir, defaultName)
	}

	if fi, err := os.Stat(path); err == nil && fi.Size() > maxSize {
		_ = os.Rename(path, path+".old")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: parseLevel(opts.Level)}))
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: parseLevel(opts.Level)}))
		return
	}

	mu.Lock()
	if file != nil {
		_ = file.Close()
	}
	file = f
	mu.Unlock()

	handler := slog.NewTextHandler(&syncWriter{w: f}, &slog.HandlerOptions{Level: parseLevel(opts.Level)})
	logger = slog.New(handler)
	Info("日志已初始化", "path", path, "level", opts.Level)
}

// Close 关闭日志文件（进程退出前调用）
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		_ = file.Sync()
		_ = file.Close()
		file = nil
	}
}

// Recover 用于 defer，捕获 panic 并落盘（带上堆栈），避免进程静默消失。
func Recover(label string) {
	if r := recover(); r != nil {
		Error("捕获 panic", "label", label, "panic", r, "stack", string(debug.Stack()))
	}
}

func Info(msg string, args ...any)  { logger.Info(msg, args...) }
func Warn(msg string, args ...any)  { logger.Warn(msg, args...) }
func Error(msg string, args ...any) { logger.Error(msg, args...) }
func Debug(msg string, args ...any) { logger.Debug(msg, args...) }
