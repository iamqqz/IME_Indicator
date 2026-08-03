#!/usr/bin/env bash
# 交叉编译 Windows 可执行文件（在 WSL/Linux 上即可运行，仅编译不运行）。
# 产物：IME-Indicator.exe
set -euo pipefail
cd "$(dirname "$0")"

OUT_DIR="${1:-.}"
EXE="${OUT_DIR}/IME-Indicator.exe"

# 1) 生成资源 .syso（图标 + manifest）。必须落在 main 包目录，链接时才被纳入。
go run github.com/akavel/rsrc@latest \
  -ico assets/icon.ico \
  -manifest assets/app.manifest \
  -o cmd/imeqiao/rsrc_windows_amd64.syso

# 2) 交叉编译：零 cgo、无控制台窗口、去符号与调试信息、路径裁剪。
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build \
  -ldflags "-H windowsgui -s -w" \
  -trimpath \
  -o "${EXE}" ./cmd/imeqiao

echo "构建完成: ${EXE}"
