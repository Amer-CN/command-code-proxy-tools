@echo off
setlocal
set "STARTUP=%APPDATA%\Microsoft\Windows\Start Menu\Programs\Startup"
set "VBS=%STARTUP%\command-code-proxy-autostart.vbs"

if not exist "%STARTUP%" (
  echo [FAIL] Startup folder not found: %STARTUP%
  ping -n 4 127.0.0.1 >nul
  exit /b 1
)

echo Set sh = CreateObject("WScript.Shell") > "%VBS%"
echo sh.Run "%~dp0bin\command-code-proxy.exe -port 55990", 0, False >> "%VBS%"

if exist "%VBS%" (
  echo [OK] Autostart installed. The proxy will start silently at every logon.
  echo      To remove it later, run the uninstall bat in this folder.
) else (
  echo [FAIL] Could not create autostart file.
)
ping -n 4 127.0.0.1 >nul
