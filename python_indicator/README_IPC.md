# IME Indicator — 常驻 IPC 守护进程（Daemon）使用开发文档

本文档说明 `python_indicator` 中**常驻守护进程（Daemon）**的用途与接入方式。
守护进程在 Windows 侧后台运行，对外提供两件事：

1. 照常绘制输入法状态提示（光标旁小圆点、系统托盘图标）；
2. 在本地开一个 TCP 端口，让外部程序（如 WSL / Windows 里的 nvim）**读取输入法状态、订阅状态变化、甚至设置输入法状态**，而不必每次都起动一个 exe。

> 本文档只讲"常驻 IPC"这种接入方式。一次性 `get`/`zh`/`en`（exe 桥接）的用法不在本文范围。

---

## 1. 启动守护进程

在 **Windows** 上打开 PowerShell / cmd，进入 `python_indicator` 目录，运行：

```bat
python main.py
```

启动后程序会：

- 在 `127.0.0.1:<IPC_PORT>` 监听（默认端口 `51234`）；
- 显示系统托盘图标（右下角，右键菜单：编辑配置 / 重启 / 关于 / 退出）；
- 在普通应用（记事本等）里绘制光标旁的小圆点。

退出：托盘右键"退出"，或在该窗口按 `Ctrl+C`。

> 也可以从 WSL 的 bash 用 `python.exe main.py` 启动——`python.exe` 走 WSL 互操作，实际进程跑在 Windows 上，托盘也出现在你的 Windows 桌面。

---

## 2. 客户端如何连接

守护进程用**纯文本行协议**（每行以 `\n` 结尾）。有两种连法：

### 方式 A：直接 TCP 连接（推荐，若 `127.0.0.1` 可达）

如果客户端所在环境能直接访问 `127.0.0.1:<IPC_PORT>`（例如 nvim 跑在 Windows 上，或你的 WSL/网络环境 loopback 互通），直接建 TCP 连接即可。

### 方式 B：`--client` 桥接子进程（WSL 不能直连 loopback 时）

WSL2 与 Windows 默认**不共用** `127.0.0.1`；若你的环境直连不通，用桥接子进程：

```bat
python main.py --client
```

该子进程本身跑在 Windows 侧，连上 Windows 的本机端口，然后把**自己的 stdin 转发给守护进程、把守护进程的响应写回自己的 stdout**。
因此 WSL 里的 nvim 只要 `jobstart` 起这个子进程，就能通过它的 stdin/stdout 与守护进程对话，无需关心 loopback 是否互通。

---

## 3. 行协议（Line Protocol）

连接成功后，双方按行通信，每行以 `\n` 结尾。命令不区分大小写。

| 客户端发送 | 守护进程响应 | 说明 |
|------------|--------------|------|
| `PING` | `PONG` | 心跳 / 连通性检测 |
| `HELLO <ver>` | `OK ime-qiao 1` | 可选握手（兼容旧协议） |
| `AUTH <token>` | `OK` 或 `ERR unauthorized` | 仅当 `IPC_BIND` 非 `loopback` 时首条必须 |
| `GET` | `MODE zh` 或 `MODE en` | 查询当前是中文还是英文 |
| `SET zh` | `OK` | 设置为中文 |
| `SET en` | `OK` | 设置为英文 |
| `SUB` | `OK`，随后立即推一条 `EVENT MODE <当前状态>` | 订阅状态推送 |
| `QUIT` | `BYE` | 断开连接 |

错误响应：

- `ERR bad-args`：`SET` 参数不是 `zh`/`en`，或命令格式错。
- `ERR not-foreground`：`SET` 时焦点窗口不在白名单内（见第 5 节）。
- `ERR set-failed`：`SET` 发给系统但系统未确认（极少出现）。
- `ERR unauthorized`：需要鉴权但未通过（仅 `IPC_BIND` 非 `loopback` 时出现）。
- `ERR unknown`：无法识别的命令。

### 状态推送（EVENT）

客户端发送 `SUB` 后，守护进程会：

1. 立即回 `OK`；
2. 立即推一条当前状态：`EVENT MODE zh`（或 `en`）；
3. 此后**每次**输入法状态变化，都推送一条 `EVENT MODE zh|en`。

`EVENT` 行格式固定为：`EVENT MODE zh` 或 `EVENT MODE en`。

> 守护进程是独立常驻进程：多个客户端可同时 `SUB`；某个客户端断开不会影响守护进程或其它客户端。

---

## 4. 示例：nvim 接入（完整可复制）

下面两段是**直接可放进 `init.lua` 的完整示例**（路径/端口按需替换）。两者都做三件事：连上守护进程 → 发 `SUB` 订阅 → 收到 `EVENT MODE zh|en` 时向 nvim 自身 stdout 写 `OSC 12` 给光标上色。由于 nvim 本就在 ConPTY 内，由它写 `OSC 12` 是 Windows Terminal 光标变色**唯一可靠**的方式（详见第 6 节）。

共用的"行缓冲 + 上色"逻辑说明：

- `on_data`/`on_stdout` 给的是**数据块**而非整行，必须自己按 `\n` 切分、缓冲半行；
- `EVENT MODE zh|en` 解析出来后调用 `apply_ime_mode`；
- `vim.fn.chansend(1, ...)` 的 `1` 即 nvim 的 stdout（fd=1），要改的就是这个。

### 方式 A：直接 TCP（nvim 能连到 `127.0.0.1:51234`）

```lua
-- ============================================================
-- IME Indicator —— nvim 接入（方式 A：直接 TCP）
-- 把整段放进 init.lua；守护进程需在 Windows 侧已启动。
-- ============================================================
local IME_HOST, IME_PORT = '127.0.0.1', 51234
local CN_COLOR, EN_COLOR = '#FF7800', '#0078FF'   -- OSC 12 只要 RGB

-- 把输入法状态反映到当前终端光标色（Windows Terminal / 支持 OSC 12 的终端）
local function apply_ime_mode(mode)
  local color = (mode == 'zh') and CN_COLOR or EN_COLOR
  vim.fn.chansend(1, '\27]12;' .. color .. '\a')   -- fd=1 = nvim 自身 stdout
end

-- on_data 给的是碎块，需按行缓冲后逐行解析
local linebuf = ''
local function on_data(_, data)
  if type(data) ~= 'table' then return end
  for _, chunk in ipairs(data) do
    linebuf = linebuf .. chunk
    local nl
    while true do
      nl = linebuf:find('\n')
      if not nl then break end
      local line = linebuf:sub(1, nl - 1):match('^%s*(.-)%s*$')
      linebuf = linebuf:sub(nl + 1)
      if line ~= '' then
        local mode = line:match('^EVENT MODE (%w+)$')
        if mode then apply_ime_mode(mode) end
      end
    end
  end
end

local ime_ch = vim.fn.sockconnect('tcp', IME_HOST .. ':' .. IME_PORT,
  { rpc = false, on_data = on_data })

if ime_ch <= 0 then
  vim.notify('IME Indicator：连接守护进程失败（127.0.0.1:' .. IME_PORT .. '）',
    vim.log.levels.WARN)
else
  vim.fn.chansend(ime_ch, "HELLO nvim\n")
  vim.fn.chansend(ime_ch, "SUB\n")   -- 订阅；立即收到一条 EVENT MODE 快照
end

-- 主动设置输入法（受 FOREGROUND_WHITELIST 限制，见第 5 节）
function _G.ime_set(mode)
  if ime_ch > 0 then vim.fn.chansend(ime_ch, "SET " .. mode .. "\n") end
end
-- 用法： :lua _G.ime_set('zh')   或   :lua _G.ime_set('en')
```

### 方式 B：`--client` 桥接（WSL 里 nvim 直连 loopback 不通时）

```lua
-- ============================================================
-- IME Indicator —— nvim 接入（方式 B：--client 桥接子进程）
-- 该子进程跑在 Windows 侧，把守护进程的推送经 stdout 喂给 nvim。
-- ============================================================
local IME_PY     = 'python.exe'                          -- WSL 互操作：实际进程在 Windows
local IME_SCRIPT = 'C:\\ime_qiao\\python_indicator\\main.py'  -- 放 Windows 路径，避免 WSL 路径转换
local CN_COLOR, EN_COLOR = '#FF7800', '#0078FF'

local function apply_ime_mode(mode)
  local color = (mode == 'zh') and CN_COLOR or EN_COLOR
  vim.fn.chansend(1, '\27]12;' .. color .. '\a')
end

local linebuf = ''
local function feed(chunk_list)
  for _, chunk in ipairs(chunk_list or {}) do
    linebuf = linebuf .. chunk
    local nl
    while true do
      nl = linebuf:find('\n')
      if not nl then break end
      local line = linebuf:sub(1, nl - 1):match('^%s*(.-)%s*$')
      linebuf = linebuf:sub(nl + 1)
      if line ~= '' then
        local mode = line:match('^EVENT MODE (%w+)$')
        if mode then apply_ime_mode(mode) end
      end
    end
  end
end

local ime_job = vim.fn.jobstart({ IME_PY, IME_SCRIPT, '--client' }, {
  on_stdout = function(_, data) feed(data) end,
  on_exit   = function() vim.notify('IME Indicator 桥接断开', vim.log.levels.WARN) end,
})

if ime_job <= 0 then
  vim.notify('IME Indicator：--client 启动失败', vim.log.levels.WARN)
else
  vim.fn.chansend(ime_job, "HELLO nvim\n")
  vim.fn.chansend(ime_job, "SUB\n")
end

function _G.ime_set(mode)
  if ime_job > 0 then vim.fn.chansend(ime_job, "SET " .. mode .. "\n") end
end
```

> 把 `python_indicator` 整目录复制到 Windows 路径（如 `C:\ime_qiao\python_indicator`）再引用，可彻底避开 WSL 文件路径转换带来的麻烦。

---

## 5. `SET` 的安全白名单

`SET` 不会盲改任意窗口的输入法。守护进程会检查**当前焦点窗口的进程名**是否在 `FOREGROUND_WHITELIST` 中：

- 白名单为空 → 放行；
- 白名单非空且焦点进程不在其中 → 返回 `ERR not-foreground`，不改；
- 焦点进程在白名单中 → 放行。

默认白名单（`config.py`）：

```python
FOREGROUND_WHITELIST = ["windowsterminal.exe", "wsl.exe", "conhost.exe"]
```

目的：防止 nvim 在后台时把 `SET` 误改到别的窗口。

---

## 6. 拿不到光标位置的处理（含 Windows Terminal）

程序只在**真正拿不到文本光标位置**时做兜底，不再对特定终端做特殊处理：

- `caret_detector.get_caret_pos()` 走完五级回退（GetGUIThreadInfo → UI Automation → IME 组合窗口 → MSAA → GUITHREADINFO）仍返回 `None`，就视为"拿不到光标"。
- **Windows Terminal / conhost 也归入此类**：这类自渲染终端同样取不到光标位置，因此与所有普通程序一致，统一走兜底——**在鼠标当前位置绘制状态标记**，不再单独处理。

由 `config.py` 的 `MOUSE_MODE` 控制（三选一）：

```python
MOUSE_MODE = "fallback"   # off=不显示 | follow=跟随鼠标 | fallback=拿不到光标时在鼠标画标记
```

- `off`：不显示鼠标标记。
- `follow`：鼠标悬停在目标光标形状（I-Beam/箭头）上时显示（原 `MOUSE_ENABLE` 行为）。
- `fallback`：拿不到文本光标（含 Windows Terminal 等）时，在鼠标位置画标记（Plan B，默认）。

兜底标记的颜色按输入法状态变化，颜色即鼠标指示器的颜色（见第 7 节 `MOUSE_COLOR_CN` / `MOUSE_COLOR_EN`），改这两个值即可改变兜底标记的颜色。

### 关于 Windows Terminal 的光标着色（OSC 12）

> 此前曾尝试由守护进程**跨进程**往 WT 的 ConPTY 注入 `OSC 12`（`AttachConsole` + 写 `CONOUT$`）。经隔离测试证实：对 WT 的每个 `OpenConsole.exe`（ConPTY 宿主）调用 `AttachConsole` 均返回 `ERROR_INVALID_HANDLE`(6)，外部进程无法附加到 ConPTY 宿主，**该方案不可行，已从 Python 版移除**。
>
> 若仍希望 WT 内光标随输入法变色的可靠方式：让**运行在 WT 内的进程（如 nvim）自己向自身 stdout 写 `OSC 12`**——因为它本就在 ConPTY 内。守护进程已通过 IPC 双向推送状态（见第 4 节完整示例），nvim 订阅 `SUB` 后即可收到 `EVENT MODE zh|en`，再自行上色，整段逻辑只在 nvim 一处维护。

---

## 7. 配置项（`config.py`）

| 配置 | 默认 | 说明 |
|------|------|------|
| `IPC_ENABLE` | `True` | 是否开启常驻端口 |
| `IPC_PORT` | `51234` | 监听端口（原 `45123` 在本机被系统预留，无法绑定，已改） |
| `IPC_BIND` | `"loopback"` | `loopback`=仅 `127.0.0.1`；`wsl`/`all`=`0.0.0.0`（后两者必须设 `IPC_TOKEN`） |
| `IPC_TOKEN` | `""` | 当 `IPC_BIND` 非 `loopback` 时必填，连接首条需 `AUTH <token>` |
| `FOREGROUND_WHITELIST` | 见第 5 节 | `SET` 允许的焦点进程白名单 |
| `MOUSE_MODE` | `"fallback"` | 鼠标标记模式：`off`=不显示 / `follow`=跟随鼠标 / `fallback`=拿不到光标时画在鼠标（Plan B） |
| `MOUSE_COLOR_CN` / `MOUSE_COLOR_EN` | 橙 / 蓝（带透明度） | 鼠标指示器（含兜底标记）的中/英文颜色，改这里即可改变"鼠标颜色" |

> `IPC_BIND=wsl`/`all` 且未设 `IPC_TOKEN` 时，守护进程会拒绝启动（避免无密码暴露到局域网）。

---

## 8. 排错

- **启动即报端口绑定失败（`WinError 10013` / `10048`）**
  端口被占用或落进 Windows 保留段。换一个端口（改 `IPC_PORT`），或查占用：
  ```bat
  netstat -ano | findstr <端口>
  ```
- **nvim 连不上**
  先确认守护进程已启动；用 `PING` 验证：
  ```bat
  # 在能连到该端口的环境里
  (echo PING & timeout 1) | telnet 127.0.0.1 51234
  ```
  或直接 `python -c "import socket;s=socket.create_connection(('127.0.0.1',51234));s.sendall(b'PING\n');print(s.recv(100))"`。
- **WSL 里直连 `127.0.0.1` 不通**
  改用第 2 节方式 B 的 `--client` 桥接子进程。
- **`SET` 总是 `ERR not-foreground`**
  焦点窗口不在 `FOREGROUND_WHITELIST`，把对应进程名加进白名单。
- **Windows Terminal 里光标不变色（用 OSC 12 上色的场景）**
  守护进程是**独立常驻进程**，无法跨进程改写 WT 的 ConPTY（实测 `AttachConsole` 必失败，见第 6 节）。光标变色必须由**运行在 WT 内的进程（nvim）自己写 `OSC 12`** 完成——即第 4 节的 nvim 示例（订阅 `SUB` 后据 `EVENT MODE` 写 `\27]12;...\a`）。若没变色，先确认：① 第 4 节代码已加载、连接成功（留意 `vim.notify` 告警）；② 你的终端确实支持 OSC 12（Windows Terminal 支持）；③ 焦点在 WT 内的 nvim 里。
