# IME Indicator (Go) 构建
# 用法：
#   make            # 交叉编译 Windows 可执行文件到 ./IME-Indicator.exe
#   make run        # 仅验证交叉编译（不运行，运行须在 Windows）
#   make vet        # go vet（交叉编译目标）
#   make fmt        # gofmt -w
#   make clean      # 删除生成的 .syso 与 exe

GOOS := windows
GOARCH := amd64
EXE := IME-Indicator.exe
RSRC := cmd/imeqiao/rsrc_windows_amd64.syso

.PHONY: all run vet fmt clean

all: $(EXE)

$(RSRC):
	go run github.com/akavel/rsrc@latest -ico assets/icon.ico -manifest assets/app.manifest -o $(RSRC) || echo "警告: rsrc 生成嵌入资源失败（Go 工具链环境问题），已跳过——exe 仍可编译，仅缺嵌入图标"

$(EXE): $(RSRC)
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 go build -ldflags "-H windowsgui -s -w" -trimpath -o $(EXE) ./cmd/imeqiao

run:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build ./...

vet:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go vet ./...

fmt:
	gofmt -w .

clean:
	rm -f $(EXE) $(RSRC)
