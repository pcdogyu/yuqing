@echo off
setlocal EnableExtensions

set "SERVICE_PORTS=80 8081 8082 8083 8084 8085 8087 8088"
set "SERVICE_NAMES=auth-service wechat-service content-service crawler-service analysis-service nlp-service gateway-web scheduler-service akshare-service"
set "HAS_REMAINING_PORTS=0"

echo [1/3] Stop service processes...
for %%S in (%SERVICE_NAMES%) do (
    call :kill_service %%S
    if errorlevel 1 goto :fail
)

echo [2/3] Stop remaining listeners by port...
for %%P in (%SERVICE_PORTS%) do (
    call :kill_port %%P
    if errorlevel 1 goto :fail
)

echo [3/3] Check service ports...
for %%P in (%SERVICE_PORTS%) do (
    call :check_port %%P
)

if "%HAS_REMAINING_PORTS%"=="1" (
    echo.
    echo stop.bat completed with remaining listening ports.
    exit /b 1
)

echo.
echo All configured services are stopped and ports are clear.
exit /b 0

:kill_service
set "TARGET_SERVICE=%~1"
tasklist /FI "IMAGENAME eq %TARGET_SERVICE%.exe" | find /I "%TARGET_SERVICE%.exe" >nul
if errorlevel 1 (
    echo Service %TARGET_SERVICE% is not running.
    exit /b 0
)
echo Service %TARGET_SERVICE% is running. Stopping...
taskkill /F /IM "%TARGET_SERVICE%.exe" >nul 2>nul
if errorlevel 1 (
    echo Failed to stop %TARGET_SERVICE%.exe.
    exit /b 1
)
exit /b 0

:kill_port
set "TARGET_PORT=%~1"
set "FOUND_PORT_PID="
for /f "tokens=5" %%I in ('netstat -ano -p tcp ^| findstr /R /C:":%TARGET_PORT% .*LISTENING"') do (
    set "FOUND_PORT_PID=%%I"
    echo Port %TARGET_PORT% occupied by PID %%I. Stopping...
    taskkill /F /PID %%I >nul 2>nul
    if errorlevel 1 (
        echo Failed to stop PID %%I for port %TARGET_PORT%.
        exit /b 1
    )
)
if not defined FOUND_PORT_PID (
    echo Port %TARGET_PORT% is free.
)
exit /b 0

:check_port
set "CHECK_PORT=%~1"
netstat -ano -p tcp | findstr /R /C:":%CHECK_PORT% .*LISTENING" >nul
if errorlevel 1 (
    echo Port %CHECK_PORT% is clear.
    exit /b 0
)
echo Port %CHECK_PORT% is still listening.
set "HAS_REMAINING_PORTS=1"
exit /b 0

:fail
echo.
echo stop.bat failed with exit code %ERRORLEVEL%.
exit /b %ERRORLEVEL%
