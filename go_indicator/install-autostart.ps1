# IME Indicator 开机自启动安装脚本
# 用法：右键 → "使用 PowerShell 运行"，或管理员 PowerShell 中执行：
#   Set-ExecutionPolicy -Scope CurrentUser RemoteSigned   （仅首次需放行脚本执行）
#   .\install-autostart.ps1
#
# 卸载：删除 %APPDATA%\Microsoft\Windows\Start Menu\Programs\Startup\IME-Indicator.lnk 即可

$ErrorActionPreference = "Stop"

$exePath   = "E:\config_and_data\nvim_config_lazy\ime_helper\IME-Indicator.exe"
$shortcut  = "$env:APPDATA\Microsoft\Windows\Start Menu\Programs\Startup\IME-Indicator.lnk"

if (-not (Test-Path $exePath)) {
    Write-Error "exe 不存在: $exePath"
    exit 1
}

$WshShell = New-Object -ComObject WScript.Shell
$lnk = $WshShell.CreateShortcut($shortcut)
$lnk.TargetPath = $exePath
$lnk.WorkingDirectory = Split-Path $exePath -Parent
$lnk.Description = "IME Indicator 输入法中英状态指示器"
$lnk.Save()

Write-Host "已创建自启动快捷方式: $shortcut"
Write-Host "→ 指向: $exePath"
