@echo off
setlocal EnableExtensions

set "GO_DIR=%~dp0"
for %%I in ("%GO_DIR%.") do set "GO_DIR=%%~fI"
for %%I in ("%GO_DIR%\..") do set "REPO_ROOT=%%~fI"
set "BIN_DIR=%GO_DIR%\bin"
set "YUQING_LOG_LEVEL=debug"
set "YUQING_RUN_VERSION=local"

cd /d "%REPO_ROOT%"
echo [1/5] Pull latest code from origin...
git diff --quiet -- go/data/yuqing.db go/data/yuqing.db-shm go/data/yuqing.db-wal >nul 2>nul
if errorlevel 1 (
    echo Detected local database changes under go/data. Skipping git pull to preserve local data.
) else (
    git pull --ff-only
    if errorlevel 1 goto :fail
)

cd /d "%GO_DIR%"
echo [2/5] Resolve build metadata...
for /f %%I in ('git -C "%REPO_ROOT%" rev-parse --short HEAD') do set "YUQING_GIT_COMMIT=%%I"
for /f "usebackq delims=" %%I in (`powershell -NoProfile -Command "(Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')"`) do set "YUQING_BUILD_TIME=%%I"
if not defined YUQING_GIT_COMMIT set "YUQING_GIT_COMMIT=unknown"
if not defined YUQING_BUILD_TIME set "YUQING_BUILD_TIME=unknown"
set "LDFLAGS=-X github.com/stonedt-yuqing/go-jin10/internal/app.Version=%YUQING_RUN_VERSION% -X github.com/stonedt-yuqing/go-jin10/internal/app.GitCommit=%YUQING_GIT_COMMIT% -X github.com/stonedt-yuqing/go-jin10/internal/app.BuildTime=%YUQING_BUILD_TIME%"

echo [3/5] Run go test ./...
go test ./...
if errorlevel 1 goto :fail

echo [4/5] Build service binaries...
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

echo [5/5] Start services with debug logging...
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

:fail
echo.
echo run.bat failed with exit code %ERRORLEVEL%.
exit /b %ERRORLEVEL%
