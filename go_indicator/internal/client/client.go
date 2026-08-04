// --client 子命令：在 Windows 侧运行，连接本机 loopback 的守护进程，
// 将 stdin 收到的命令转发给守护进程、将守护进程响应/事件写回 stdout。
// WSL 内 nvim 用 jobstart 启动本 exe 的 --client，从而跨 WSL2 与守护进程通信。
package client

import (
	"fmt"
	"io"
	"net"
	"os"

	"imeqiao/internal/config"
	"imeqiao/internal/logging"
)

// Run 运行客户端桥接，直到连接关闭。
func Run(cfg *config.Config) error {
	addr := fmt.Sprintf("127.0.0.1:%d", cfg.IPCPort)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	if cfg.IPCBind != "loopback" && cfg.IPCToken != "" {
		if _, err := conn.Write([]byte("AUTH " + cfg.IPCToken + "\n")); err != nil {
			return err
		}
	}

	go func() {
		if _, err := io.Copy(conn, os.Stdin); err != nil {
			logging.Warn("client: stdin->conn 复制失败", "error", err)
		}
		conn.Close()
	}()
	_, err = io.Copy(os.Stdout, conn)
	return err
}
