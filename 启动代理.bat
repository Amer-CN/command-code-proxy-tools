@echo off
cd /d "%~dp0"

rem --- check if already running ---
curl -s -f -o nul http://127.0.0.1:55990/health >nul 2>&1
if %errorlevel%==0 (
  echo [INFO] Proxy already running at http://127.0.0.1:55990
  ping -n 3 127.0.0.1 >nul
  exit /b 0
)

start "CommandCode Proxy" /min "bin\command-code-proxy.exe" -port 55990 >nul 2>&1

echo Starting... waiting for the server (up to ~15s)
set /a tries=0
:waitloop
ping -n 2 127.0.0.1 >nul
curl -s -f -o nul http://127.0.0.1:55990/health >nul 2>&1
if %errorlevel%==0 goto :up
set /a tries+=1
if %tries% lss 14 goto :waitloop

echo [FAIL] Not responding - port 55990 may be occupied by another program.
ping -n 4 127.0.0.1 >nul
exit /b 1

:up
echo [OK] Started. Base URL for your agent: http://127.0.0.1:55990/v1
ping -n 4 127.0.0.1 >nul
exit /b 0
