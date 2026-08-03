// 零依赖迷你 TOML 配置解析，刻意控制二进制体积（不引入 toml/serde 等价物）。
// 字段与 rust_indicator/config.rs 对应；新增 [ipc] 段用于常驻桥接。
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Config 全局配置
type Config struct {
	PollStateIntervalMS uint64
	PollTrackIntervalMS uint64

	TrayEnable bool

	CaretEnable  bool
	CaretColorCN uint32
	CaretColorEN uint32
	CaretSize    int32
	CaretOffsetX int32
	CaretOffsetY int32
	CaretShowEN  bool

	MouseMode          string // 鼠标标记模式: off | follow | fallback
	MouseColorCN       uint32
	MouseColorEN       uint32
	MouseSize          int32
	MouseOffsetX       int32
	MouseOffsetY       int32
	MouseShowEN        bool
	MouseTargetCursors []uint32

	// 常驻桥接 IPC
	IPCEnable bool
	IPCPort   int
	IPCBind   string // loopback | wsl | all
	IPCToken  string

	// SET 安全：仅当焦点窗口进程名在此白名单（小写）时才允许设置输入法
	ForegroundWhiteList []string
}

// Default 默认值
func Default() *Config {
	return &Config{
		PollStateIntervalMS: 100,
		PollTrackIntervalMS: 10,
		TrayEnable:          true,

		CaretEnable:  true,
		CaretColorCN: ParseColor("#FF7800A0"),
		CaretColorEN: ParseColor("#0078FF30"),
		CaretSize:    8,
		CaretOffsetX: 0,
		CaretOffsetY: 0,
		CaretShowEN:  true,

		MouseMode:          "fallback",
		MouseColorCN:       ParseColor("#FF7800C8"),
		MouseColorEN:       ParseColor("#0078FF30"),
		MouseSize:          8,
		MouseOffsetX:       0,
		MouseOffsetY:       24,
		MouseShowEN:        true,
		MouseTargetCursors: []uint32{32513, 32512},

		IPCEnable: true,
		IPCPort:   51234,
		IPCBind:   "loopback",
		IPCToken:  "",

		ForegroundWhiteList: []string{"windowsterminal.exe", "wsl.exe", "conhost.exe", "neovide.exe"},
	}
}

// ============================================================
// 颜色解析 #RRGGBBAA -> 0xAARRGGBB
// ============================================================

func ParseColor(s string) uint32 {
	clean := strings.TrimSpace(s)
	clean = strings.Trim(clean, "\"")
	clean = strings.TrimPrefix(clean, "#")
	if len(clean) >= 6 {
		r, _ := strconv.ParseUint(clean[0:2], 16, 32)
		g, _ := strconv.ParseUint(clean[2:4], 16, 32)
		b, _ := strconv.ParseUint(clean[4:6], 16, 32)
		var a uint64 = 0xA0
		if len(clean) >= 8 {
			if v, err := strconv.ParseUint(clean[6:8], 16, 32); err == nil {
				a = v
			}
		}
		return uint32((a << 24) | (r << 16) | (g << 8) | b)
	}
	return 0xA0FF7800
}

// ============================================================
// 解析
// ============================================================

func loadConfig() *Config {
	cfg := Default()
	path := GetConfigPath()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		_ = os.WriteFile(path, []byte(generateTemplate()), 0644)
		return cfg
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}

	sections := map[string]map[string]string{}
	var cur string
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			cur = strings.ToLower(line[1 : len(line)-1])
			continue
		}
		if idx := strings.Index(line, "="); idx >= 0 {
			key := strings.ToLower(strings.TrimSpace(line[:idx]))
			// 去掉行内注释（" #" 形式）
			val := strings.TrimSpace(line[idx+1:])
			if i := strings.Index(val, " #"); i >= 0 {
				val = strings.TrimSpace(val[:i])
			}
			val = strings.Trim(val, "\"")
			if sections[cur] == nil {
				sections[cur] = map[string]string{}
			}
			sections[cur][key] = val
		}
	}

	get := func(sec, key string) (string, bool) {
		m, ok := sections[sec]
		if !ok {
			return "", false
		}
		v, ok := m[key]
		return v, ok
	}
	getBool := func(sec, key string) (bool, bool) {
		v, ok := get(sec, key)
		if !ok {
			return false, false
		}
		switch v {
		case "true":
			return true, true
		case "false":
			return false, true
		}
		return false, false
	}
	getU64 := func(sec, key string) (uint64, bool) {
		v, ok := get(sec, key)
		if !ok {
			return 0, false
		}
		n, err := strconv.ParseUint(v, 10, 64)
		return n, err == nil
	}
	getI32 := func(sec, key string) (int32, bool) {
		v, ok := get(sec, key)
		if !ok {
			return 0, false
		}
		n, err := strconv.ParseInt(v, 10, 32)
		return int32(n), err == nil
	}

	if v, ok := getU64("poll", "state_interval_ms"); ok {
		cfg.PollStateIntervalMS = v
	}
	if v, ok := getU64("poll", "track_interval_ms"); ok {
		cfg.PollTrackIntervalMS = v
	}
	if v, ok := getBool("tray", "enable"); ok {
		cfg.TrayEnable = v
	}
	if v, ok := getBool("caret", "enable"); ok {
		cfg.CaretEnable = v
	}
	if v, ok := get("caret", "color_cn"); ok {
		cfg.CaretColorCN = ParseColor(v)
	}
	if v, ok := get("caret", "color_en"); ok {
		cfg.CaretColorEN = ParseColor(v)
	}
	if v, ok := getI32("caret", "size"); ok {
		cfg.CaretSize = v
	}
	if v, ok := getI32("caret", "offset_x"); ok {
		cfg.CaretOffsetX = v
	}
	if v, ok := getI32("caret", "offset_y"); ok {
		cfg.CaretOffsetY = v
	}
	if v, ok := getBool("caret", "show_en"); ok {
		cfg.CaretShowEN = v
	}
	if v, ok := get("mouse", "mode"); ok {
		cfg.MouseMode = v
	}
	if v, ok := get("mouse", "color_cn"); ok {
		cfg.MouseColorCN = ParseColor(v)
	}
	if v, ok := get("mouse", "color_en"); ok {
		cfg.MouseColorEN = ParseColor(v)
	}
	if v, ok := getI32("mouse", "size"); ok {
		cfg.MouseSize = v
	}
	if v, ok := getI32("mouse", "offset_x"); ok {
		cfg.MouseOffsetX = v
	}
	if v, ok := getI32("mouse", "offset_y"); ok {
		cfg.MouseOffsetY = v
	}
	if v, ok := getBool("mouse", "show_en"); ok {
		cfg.MouseShowEN = v
	}
	if v, ok := get("mouse", "target_cursors"); ok {
		// 替换语义：用户配置覆盖默认值，而非追加（避免 [32513] 变成 [32513,32512,32513]）
		cfg.MouseTargetCursors = nil
		cs := strings.Trim(v, "[]")
		for _, p := range strings.Split(cs, ",") {
			p = strings.TrimSpace(p)
			if n, err := strconv.ParseUint(p, 10, 32); err == nil {
				cfg.MouseTargetCursors = append(cfg.MouseTargetCursors, uint32(n))
			}
		}
	}
	if v, ok := getBool("ipc", "enable"); ok {
		cfg.IPCEnable = v
	}
	if v, ok := get("ipc", "port"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.IPCPort = n
		}
	}
	if v, ok := get("ipc", "bind"); ok {
		cfg.IPCBind = v
	}
	if v, ok := get("ipc", "token"); ok {
		cfg.IPCToken = v
	}
	if v, ok := get("ipc", "foreground_whitelist"); ok {
		wl := strings.Trim(v, "[]")
		var list []string
		for _, p := range strings.Split(wl, ",") {
			p = strings.ToLower(strings.TrimSpace(p))
			p = strings.Trim(p, "\"")
			if p != "" {
				list = append(list, p)
			}
		}
		if len(list) > 0 {
			cfg.ForegroundWhiteList = list
		}
	}

	return cfg
}

// ============================================================
// 全局缓存
// ============================================================

var (
	once     sync.Once
	instance *Config
)

// Get 懒加载并缓存（进程内不热重载，改动需重启）
func Get() *Config {
	once.Do(func() {
		instance = loadConfig()
	})
	return instance
}

// GetConfigPath 返回 exe 同目录的 config.toml
func GetConfigPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "config.toml"
	}
	return filepath.Join(filepath.Dir(exe), "config.toml")
}

// ============================================================
// 配置模板
// ============================================================

func generateTemplate() string {
	return `# 输入指示器 (IME Indicator) 配置文件

[poll]
state_interval_ms = 100   # 状态检测间隔 (ms)
track_interval_ms = 10    # 位置追踪间隔 (ms)

[tray]
enable = true               # 是否显示托盘图标 (false 时完全后台运行，只能通过任务管理器结束)

[caret]
enable = true               # 是否启用文本光标提示
color_cn = "#FF7800A0"    # 中文状态颜色 (#RRGGBBAA)
color_en = "#0078FF30"    # 英文状态颜色
size = 8                    # 提示球大小
offset_x = 0
offset_y = 0
show_en = true              # 英文状态下是否显示

[mouse]
mode = "fallback"           # 鼠标标记模式: off | follow | fallback
color_cn = "#FF7800C8"    # 中文状态颜色
color_en = "#0078FF30"    # 英文状态颜色
size = 8                    # 提示球大小
offset_x = 0
offset_y = 24
show_en = true              # 英文状态下是否显示
target_cursors = [32513, 32512]  # I-Beam, Normal

[ipc]
enable = true               # 常驻桥接 IPC
port = 51234                # 监听端口 (Windows 侧 loopback)
bind = "loopback"           # loopback | wsl | all
token = ""                  # bind 非 loopback 时必填
foreground_whitelist = ["windowsterminal.exe", "wsl.exe", "conhost.exe", "neovide.exe"]  # SET 仅允许这些前台进程
`
}
