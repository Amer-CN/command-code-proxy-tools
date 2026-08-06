@echo off
chcp 65001 >nul
cd /d "%~dp0"
title 构建 CommandCode 代理控制台

where go >nul 2>&1
if errorlevel 1 (
  echo [错误] 未找到 Go 工具链。
  echo 请先安装 Go 1.22 或更高版本: https://go.dev/dl/
  echo.
  pause
  exit /b 1
)

echo [1/3] 拉取界面依赖（webview_go / clipboard）...
go get github.com/webview/webview_go@latest github.com/atotto/clipboard@latest
if errorlevel 1 goto fail
go mod tidy
if errorlevel 1 goto fail

echo [2/3] 编译独立 exe（WebView2 渲染 · 无控制台窗口）...
set CGO_ENABLED=0
go build -trimpath -ldflags="-H windowsgui -s -w" -o CommandCodeProxyDeck.exe ./app
if errorlevel 1 goto fail

echo [3/3] 构建完成
echo.
echo   输出文件: %CD%\CommandCodeProxyDeck.exe
for %%F in (CommandCodeProxyDeck.exe) do echo   文件大小: %%~zF 字节
echo.
echo   双击 exe 即可打开控制台（界面使用系统自带的 WebView2 运行时）。
echo   「启动代理 / 停止代理 / 开机自启」等 bat 已适配新程序。
echo.
pause
exit /b 0

:fail
echo.
echo [失败] 构建出错，请检查上方日志。
pause
exit /b 1
