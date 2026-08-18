# cm (cmd-mgr) Windows 快速安装脚本
# 用法: irm https://raw.githubusercontent.com/HenC49/cmd-mgr/main/scripts/install.ps1 | iex
# 可选环境变量:
#   CM_VERSION=v0.1.0      指定版本（带 v 前缀），默认 latest
#   CM_INSTALL_DIR=<path>  指定安装目录，默认 %LOCALAPPDATA%\Programs\cm
$ErrorActionPreference = "Stop"
$Repo = "HenC49/cmd-mgr"

switch ($env:PROCESSOR_ARCHITECTURE) {
  "AMD64" { $arch = "amd64" }
  "ARM64" { $arch = "arm64" }
  default { throw "不支持的架构: $($env:PROCESSOR_ARCHITECTURE)" }
}

$asset = "cm-windows-$arch.zip"
if ($env:CM_VERSION) {
  $url = "https://github.com/$Repo/releases/download/$($env:CM_VERSION)/$asset"
} else {
  $url = "https://github.com/$Repo/releases/latest/download/$asset"
}

$dest = if ($env:CM_INSTALL_DIR) { $env:CM_INSTALL_DIR } else { "$env:LOCALAPPDATA\Programs\cm" }
New-Item -ItemType Directory -Force -Path $dest | Out-Null

$tmp = Join-Path $env:TEMP $asset
Write-Host "==> 下载 $url"
Invoke-WebRequest -Uri $url -OutFile $tmp

Expand-Archive -Path $tmp -DestinationPath $dest -Force
Remove-Item $tmp

# 加入用户 PATH（幂等）
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$dest*") {
  [Environment]::SetEnvironmentVariable("Path", "$userPath;$dest", "User")
  Write-Host "==> 已把 $dest 加入用户 PATH（新开终端生效）"
}

Write-Host "==> 安装完成: $dest\cm.exe"
& "$dest\cm.exe" --version
