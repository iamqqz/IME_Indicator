"""
常驻 IPC 桥接：TCP loopback 行协议 + Hub 广播。

协议（与 Go 版一致，WSL 的 --client 子进程复用同一套）：
  PING                  -> PONG
  HELLO <ver>           -> OK ime-qiao <ver>
  AUTH <token>          -> OK / ERR unauthorized   （bind 非 loopback 时首条必须）
  GET                   -> MODE zh|en
  SET zh|en             -> OK / ERR <reason>        （默认校验前台进程名 ∈ 白名单）
  SUB                   -> OK，随后状态变化推送 EVENT MODE zh|en
  QUIT                  -> BYE 并关闭

安全：bind=loopback（默认）仅本机；bind=wsl/all 时强制 token 非空并校验来源。
"""
import socket
import threading
import queue

from ime_detector import is_chinese_mode, set_mode, get_foreground_process_name
import config


class Hub:
    """订阅者集合，向所有订阅者广播（满则丢弃，不阻塞）。"""

    def __init__(self):
        self._subs = set()
        self._lock = threading.Lock()

    def subscribe(self):
        ch = queue.Queue(maxsize=8)
        with self._lock:
            self._subs.add(ch)
        return ch

    def unsubscribe(self, ch):
        with self._lock:
            self._subs.discard(ch)

    def broadcast(self, msg):
        with self._lock:
            subs = list(self._subs)
        for ch in subs:
            try:
                ch.put_nowait(msg)
            except Exception:
                # 队列满或已关闭：丢弃最旧事件
                pass


def allow_set(cfg):
    """SET 仅允许作用于白名单内的前台进程；白名单为空则放行。"""
    wl = cfg.FOREGROUND_WHITELIST
    if not wl:
        return True
    fg = get_foreground_process_name()
    return fg in wl


class Server:
    def __init__(self, cfg, hub):
        self.cfg = cfg
        self.hub = hub
        self.sock = None
        self.auth_required = cfg.IPC_BIND != "loopback"

    def start(self):
        if not self.cfg.IPC_ENABLE:
            return None
        # 监听 0.0.0.0（wsl/all）时必须设密码，否则任何机器都能连，直接不启动
        if self.cfg.IPC_BIND != "loopback" and not self.cfg.IPC_TOKEN:
            print("IPC 未启动：bind 为非 loopback 时必须设置 IPC_TOKEN")
            return None
        addr = "127.0.0.1" if self.cfg.IPC_BIND == "loopback" else "0.0.0.0"
        self.sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self.sock.bind((addr, self.cfg.IPC_PORT))
        self.sock.listen(8)
        t = threading.Thread(target=self._serve, daemon=True)
        t.start()
        return self

    def _serve(self):
        while True:
            try:
                conn, _ = self.sock.accept()
            except OSError:
                return
            threading.Thread(target=self._handle, args=(conn,), daemon=True).start()

    def _handle(self, conn):
        authed = not self.auth_required
        sub_ch = None
        wlock = threading.Lock()

        def send(line):
            try:
                with wlock:
                    conn.sendall((line + "\n").encode("utf-8"))
            except OSError:
                pass

        def pump():
            try:
                for msg in iter(sub_ch.get, None):
                    send(msg)
            except Exception:
                pass

        try:
            conn.settimeout(None)
            f = conn.makefile("rwb", buffering=0)
            while True:
                raw = f.readline()
                if not raw:
                    break
                text = raw.decode("utf-8", "replace").strip()
                if not text:
                    continue
                parts = text.split()
                cmd = parts[0].upper()
                if not authed:
                    if cmd == "AUTH" and len(parts) >= 2 and parts[1] == self.cfg.IPC_TOKEN:
                        authed = True
                        send("OK")
                    else:
                        send("ERR unauthorized")
                        break
                    continue
                if cmd == "PING":
                    send("PONG")
                elif cmd == "HELLO":
                    send("OK ime-qiao 1")
                elif cmd == "GET":
                    send("MODE " + ("zh" if is_chinese_mode() else "en"))
                elif cmd == "SET":
                    if len(parts) < 2 or parts[1] not in ("zh", "en"):
                        send("ERR bad-args")
                    elif not allow_set(self.cfg):
                        send("ERR not-foreground")
                    elif set_mode(parts[1] == "zh"):
                        send("OK")
                    else:
                        send("ERR set-failed")
                elif cmd == "SUB":
                    if sub_ch is None:
                        sub_ch = self.hub.subscribe()
                        send("OK")
                        send("EVENT MODE " + ("zh" if is_chinese_mode() else "en"))
                        threading.Thread(target=pump, daemon=True).start()
                    else:
                        send("OK")
                elif cmd == "QUIT":
                    send("BYE")
                    break
                else:
                    send("ERR unknown")
        finally:
            if sub_ch is not None:
                self.hub.unsubscribe(sub_ch)
            try:
                conn.close()
            except OSError:
                pass

    def broadcast_mode(self, chinese):
        """由检测器在状态变化时调用。"""
        self.hub.broadcast("EVENT MODE " + ("zh" if chinese else "en"))

    def close(self):
        if self.sock:
            try:
                self.sock.close()
            except OSError:
                pass
