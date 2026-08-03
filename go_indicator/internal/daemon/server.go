// 常驻桥接服务：TCP loopback 监听 + 行协议（PING/HELLO/AUTH/GET/SET/SUB/QUIT + EVENT 推送）。
// 守护进程在 Windows 侧监听 127.0.0.1:<port>；WSL 内 nvim 通过同 exe 的 --client 子进程
// （运行在 Windows 侧）连接 loopback，再经 stdio 与 nvim 桥接，从而跨 WSL2 工作。
package daemon

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"

	"imeqiao/internal/config"
	"imeqiao/internal/imedetect"
)

// Server 常驻 IPC 服务
type Server struct {
	cfg  *config.Config
	hub  *Hub
	ln   net.Listener
	auth bool // 是否要求 AUTH
}

// Start 启动监听（若 IPC 启用）。返回 *Server 以便后续关闭。
func Start(cfg *config.Config, hub *Hub) (*Server, error) {
	if !cfg.IPCEnable {
		return nil, nil
	}
	addr := fmt.Sprintf("127.0.0.1:%d", cfg.IPCPort)
	if cfg.IPCBind != "loopback" {
		addr = fmt.Sprintf("0.0.0.0:%d", cfg.IPCPort)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, hub: hub, ln: ln, auth: cfg.IPCBind != "loopback"}
	go s.serve()
	return s, nil
}

// Addr 返回监听地址（用于诊断）
func (s *Server) Addr() string {
	if s == nil || s.ln == nil {
		return ""
	}
	return s.ln.Addr().String()
}

// Close 停止监听
func (s *Server) Close() {
	if s != nil && s.ln != nil {
		s.ln.Close()
	}
}

func (s *Server) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	authed := !s.auth
	subCh := make(chan string, 8)
	subscribed := false
	defer func() {
		if subscribed {
			s.hub.Unsubscribe(subCh)
		}
	}()

	var wmu sync.Mutex
	send := func(line string) {
		wmu.Lock()
		defer wmu.Unlock()
		writer.WriteString(line)
		writer.WriteByte('\n')
		writer.Flush()
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		cmd := strings.ToUpper(parts[0])

		if !authed {
			if cmd == "AUTH" && len(parts) >= 2 && parts[1] == s.cfg.IPCToken {
				authed = true
				send("OK")
			} else {
				send("ERR unauthorized")
				return
			}
			continue
		}

		switch cmd {
		case "PING":
			send("PONG")
		case "HELLO":
			send("OK ime-qiao 1")
		case "GET":
			if imedetect.IsChineseMode() {
				send("MODE zh")
			} else {
				send("MODE en")
			}
		case "SET":
			if len(parts) < 2 {
				send("ERR bad-args")
				break
			}
			mode := strings.ToLower(parts[1])
			if mode != "zh" && mode != "en" {
				send("ERR bad-args")
				break
			}
			if !s.allowSet() {
				send("ERR not-foreground")
				break
			}
			ok := imedetect.SetMode(mode == "zh")
			if ok {
				send("OK")
			} else {
				send("ERR set-failed")
			}
		case "SUB":
			if !subscribed {
				subCh = s.hub.Subscribe()
				subscribed = true
				// 当前状态立即推送
				if imedetect.IsChineseMode() {
					send("OK")
					send("EVENT MODE zh")
				} else {
					send("OK")
					send("EVENT MODE en")
				}
				// 事件推送协程
				go func() {
					for msg := range subCh {
						send(msg)
					}
				}()
			} else {
				send("OK")
			}
		case "QUIT":
			send("BYE")
			return
		default:
			send("ERR unknown")
		}
	}
}

// allowSet 检查前台进程是否在白名单（为空则放行）
func (s *Server) allowSet() bool {
	wl := s.cfg.ForegroundWhiteList
	if len(wl) == 0 {
		return true
	}
	fg := imedetect.GetForegroundProcessName()
	for _, w := range wl {
		if fg == w {
			return true
		}
	}
	return false
}

// BroadcastMode 由检测器在状态变化时调用
func (s *Server) BroadcastMode(chinese bool) {
	if s == nil || s.hub == nil {
		return
	}
	mode := "EVENT MODE en"
	if chinese {
		mode = "EVENT MODE zh"
	}
	s.hub.Broadcast(mode)
}
