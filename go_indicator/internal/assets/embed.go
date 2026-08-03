// 内嵌静态资源，避免构建脚本拷贝 assets 目录。
package assets

import _ "embed"

// About 为"关于"对话框的默认文案；exe 同目录的 assets/about.txt 可覆盖之。
//
//go:embed about.txt
var About string
