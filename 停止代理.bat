@echo off
taskkill /F /IM command-code-proxy.exe >nul 2>&1
if %errorlevel%==0 (
  echo [OK] Proxy stopped.
) else (
  echo [INFO] Proxy was not running.
)
ping -n 3 127.0.0.1 >nul
