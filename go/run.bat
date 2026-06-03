@echo off
setlocal EnableExtensions

set "SKIP_PULL=0"
if /I "%~1"=="--skip-pull" set "SKIP_PULL=1"

set "GO_DIR=%~dp0"
for %%I in ("%GO_DIR%.") do set "GO_DIR=%%~fI"
for %%I in ("%GO_DIR%\..") do set "REPO_ROOT=%%~fI"
set "BIN_DIR=%GO_DIR%\bin"
set "SERVICE_PORTS=80 8081 8082 8083 8084 8085 8086"
set "SERVICE_NAMES=auth-service content-service crawler-service analysis-service nlp-service gateway-web scheduler-service"
set "YUQING_LOG_LEVEL=debug"
set "YUQING_RUN_VERSION=local"

cd /d "%REPO_ROOT%"
echo [1/6] Pull latest code from origin...
if "%SKIP_PULL%"=="1" (
    echo Skip pull requested. Continue with current worktree.
) else (
    for /f %%I in ('git rev-parse HEAD') do set "YUQING_HEAD_BEFORE=%%I"
    git diff --quiet -- go/data/yuqing.db go/data/yuqing.db-shm go/data/yuqing.db-wal >nul 2>nul
    if errorlevel 1 (
        echo Detected local database changes under go/data. Skipping git pull to preserve local data.
    ) else (
        git pull --ff-only
        if errorlevel 1 goto :fail
        for /f %%I in ('git rev-parse HEAD') do set "YUQING_HEAD_AFTER=%%I"
        if not "%YUQING_HEAD_BEFORE%"=="%YUQING_HEAD_AFTER%" (
            echo Repository updated. Restarting run.bat with the refreshed worktree...
            cmd /c ""%GO_DIR%\run.bat" --skip-pull"
            exit /b %ERRORLEVEL%
        )
    )
)

cd /d "%GO_DIR%"
echo [2/6] Resolve build metadata...
for /f %%I in ('git -C "%REPO_ROOT%" rev-parse --short HEAD') do set "YUQING_GIT_COMMIT=%%I"
for /f "usebackq delims=" %%I in (`powershell -NoProfile -Command "(Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')"`) do set "YUQING_BUILD_TIME=%%I"
if not defined YUQING_GIT_COMMIT set "YUQING_GIT_COMMIT=unknown"
if not defined YUQING_BUILD_TIME set "YUQING_BUILD_TIME=unknown"
set "LDFLAGS=-X github.com/stonedt-yuqing/go-jin10/internal/app.Version=%YUQING_RUN_VERSION% -X github.com/stonedt-yuqing/go-jin10/internal/app.GitCommit=%YUQING_GIT_COMMIT% -X github.com/stonedt-yuqing/go-jin10/internal/app.BuildTime=%YUQING_BUILD_TIME%"

echo [3/6] Run go test ./...
go test ./...
if errorlevel 1 goto :fail

echo [4/6] Stop processes occupying service ports...
for %%P in (%SERVICE_PORTS%) do (
    call :kill_port %%P
    if errorlevel 1 goto :fail
)
for %%S in (%SERVICE_NAMES%) do (
    call :kill_service %%S
    if errorlevel 1 goto :fail
)

echo [5/6] Build service binaries...
if not exist "%BIN_DIR%" mkdir "%BIN_DIR%"
for %%S in (
    auth-service
    content-service
    crawler-service
    analysis-service
    nlp-service
    gateway-web
    scheduler-service
) do (
    echo Building %%S...
    go build -ldflags "%LDFLAGS%" -o "%BIN_DIR%\%%S.exe" ".\cmd\%%S"
    if errorlevel 1 goto :fail
)

echo [6/6] Start services with debug logging...
start "auth-service" "%BIN_DIR%\auth-service.exe"
start "content-service" "%BIN_DIR%\content-service.exe"
start "crawler-service" "%BIN_DIR%\crawler-service.exe"
start "analysis-service" "%BIN_DIR%\analysis-service.exe"
start "nlp-service" "%BIN_DIR%\nlp-service.exe"
start "gateway-web" "%BIN_DIR%\gateway-web.exe"
start "scheduler-service" "%BIN_DIR%\scheduler-service.exe"

echo.
echo Services started.
echo Gateway: http://127.0.0.1
echo LogLevel: %YUQING_LOG_LEVEL%
echo Version: %YUQING_RUN_VERSION%
echo Commit: %YUQING_GIT_COMMIT%
echo BuildTime: %YUQING_BUILD_TIME%
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

:fail
echo.
echo run.bat failed with exit code %ERRORLEVEL%.
exit /b %ERRORLEVEL%
