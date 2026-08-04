#!/usr/bin/env bash
# 交叉编译 Windows 可执行文件（在 WSL/Linux 上即可运行，仅编译不运行）。
# 产物：IME-Indicator.exe
set -euo pipefail
cd "$(dirname "$0")"

OUT_DIR="${1:-.}"
EXE="${OUT_DIR}/IME-Indicator.exe"

# 1) 生成资源 .syso（图标 + manifest）。必须落在 main 包目录，链接时才被纳入。
#    该步骤依赖 `go run` 拉取并编译 rsrc 工具，遇到 Go 工具链环境问题可能失败；
#    失败不影响编译，仅导致 exe 不含嵌入图标（可用 exe 同目录 icon.png 代替），故不中止脚本。
go run github.com/akavel/rsrc@latest \
  -ico assets/icon.ico \
  -manifest assets/app.manifest \
  -o cmd/imeqiao/rsrc_windows_amd64.syso || \
  echo "警告: rsrc 生成嵌入资源失败（Go 工具链环境问题），已跳过——exe 仍可正常编译，仅缺嵌入图标（可放 icon.png 到 exe 同目录代替）"

# 2) 交叉编译：零 cgo、无控制台窗口、去符号与调试信息、路径裁剪。
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build \
  -ldflags "-H windowsgui -s -w" \
  -trimpath \
  -o "${EXE}" ./cmd/imeqiao

echo "构建完成: ${EXE}"
