@echo off
chcp 65001 >nul
cd /d "%~dp0"
echo 正在清理 v1.x 旧文件（HTA 版已被 WebView2 独立程序取代）...
del /f /q "CommandCode代理-单文件版.hta" 2>nul
del /f /q "hta_template.txt" 2>nul
del /f /q "生成单文件版.py" 2>nul
rmdir /s /q "bin" 2>nul
echo.
echo [OK] 清理完成。接下来三步：
echo   1. git add -A ^&^& git commit -m "v2.0.0: WebView2 独立程序 + 科幻全息控制台" ^&^& git push
echo   2. 双击「构建EXE.bat」生成 CommandCodeProxyDeck.exe（需本机装有 Go）
echo   3. 双击 exe，点击左侧能量核心点火
echo.
echo 提示：旧版开机自启指向已删除的 bin 目录，请在新控制台里重新开关一次「开机自启」。
pause
