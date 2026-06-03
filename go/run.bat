@echo off
setlocal EnableExtensions

set "SKIP_PULL=0"
if /I "%~1"=="--skip-pull" set "SKIP_PULL=1"
set "AFTER_PULL=0"
if /I "%~1"=="--after-pull" set "AFTER_PULL=1"
set "SCRIPT_PATH=%~f0"
if "%AFTER_PULL%"=="1" if not "%~2"=="" set "SCRIPT_PATH=%~f2"

for %%I in ("%SCRIPT_PATH%") do set "SCRIPT_DIR=%%~dpI"
set "GO_DIR=%SCRIPT_DIR%"
for %%I in ("%GO_DIR%.") do set "GO_DIR=%%~fI"
for %%I in ("%GO_DIR%\..") do set "REPO_ROOT=%%~fI"
set "BIN_DIR=%GO_DIR%\bin"
set "LOG_DIR=%GO_DIR%\runtime-logs"
set "SERVICE_PORTS=80 8081 8082 8083 8084 8085"
set "SERVICE_NAMES=auth-service content-service crawler-service analysis-service nlp-service gateway-web scheduler-service"
set "YUQING_LOG_LEVEL=debug"
set "YUQING_RUN_VERSION=local"
set "TEMP_BOOTSTRAP=%TEMP%\yuqing-run-bootstrap-%RANDOM%-%RANDOM%.cmd"

if "%SKIP_PULL%"=="0" if "%AFTER_PULL%"=="0" (
    copy /Y "%~f0" "%TEMP_BOOTSTRAP%" >nul
    if errorlevel 1 (
        echo Failed to create bootstrap copy: %TEMP_BOOTSTRAP%
        goto :fail
    )
    cmd /c ""%TEMP_BOOTSTRAP%" --after-pull "%~f0""
    set "BOOTSTRAP_EXIT=%ERRORLEVEL%"
    del /Q "%TEMP_BOOTSTRAP%" >nul 2>nul
    exit /b %BOOTSTRAP_EXIT%
)

cd /d "%REPO_ROOT%"
echo [1/6] Pull latest code from origin...
if "%SKIP_PULL%"=="1" (
    echo Skip pull requested. Continue with current worktree.
) else if "%AFTER_PULL%"=="1" (
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
        )
    )
    cmd /c ""%GO_DIR%\run.bat" --skip-pull"
    exit /b %ERRORLEVEL%
) else (
    echo Internal error: unexpected startup mode.
    goto :fail
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
if not exist "%LOG_DIR%" mkdir "%LOG_DIR%"
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
    go build -ldflags "%LDFLAGS%" -o "%BIN_DIR%\%%S.exe" "./cmd/%%S"
    if errorlevel 1 goto :fail
)

echo [6/6] Start services with debug logging...
call :start_service auth-service 8081
if errorlevel 1 goto :fail
call :start_service content-service 8082
if errorlevel 1 goto :fail
call :start_service crawler-service 8083
if errorlevel 1 goto :fail
call :start_service analysis-service 8084
if errorlevel 1 goto :fail
call :start_service nlp-service 8085
if errorlevel 1 goto :fail
call :start_service gateway-web 80
if errorlevel 1 goto :fail
call :start_process_service scheduler-service
if errorlevel 1 goto :fail

echo Final port checks:
call :print_port_status gateway-web 80
call :print_port_status auth-service 8081
call :print_port_status content-service 8082
call :print_port_status crawler-service 8083
call :print_port_status analysis-service 8084
call :print_port_status nlp-service 8085

echo.
echo Services started.
echo Gateway: http://127.0.0.1
echo LogLevel: %YUQING_LOG_LEVEL%
echo Version: %YUQING_RUN_VERSION%
echo Commit: %YUQING_GIT_COMMIT%
echo BuildTime: %YUQING_BUILD_TIME%
exit /b 0

:start_service
set "TARGET_SERVICE=%~1"
set "TARGET_PORT=%~2"
call :start_process_core %TARGET_SERVICE%
if errorlevel 1 exit /b 1
call :wait_for_port_stable %TARGET_SERVICE% %TARGET_PORT%
exit /b %ERRORLEVEL%

:start_process_service
set "TARGET_SERVICE=%~1"
call :start_process_core %TARGET_SERVICE%
if errorlevel 1 exit /b 1
call :wait_for_process %TARGET_SERVICE%
if errorlevel 1 exit /b 1
call :print_service_logs %TARGET_SERVICE%
exit /b 0

:start_process_core
set "TARGET_SERVICE=%~1"
set "OUT_LOG=%LOG_DIR%\%TARGET_SERVICE%.out.log"
set "ERR_LOG=%LOG_DIR%\%TARGET_SERVICE%.err.log"
if exist "%OUT_LOG%" del /Q "%OUT_LOG%" >nul 2>nul
if exist "%ERR_LOG%" del /Q "%ERR_LOG%" >nul 2>nul
echo Starting %TARGET_SERVICE%...
powershell -NoProfile -Command "$p = Start-Process -FilePath '%BIN_DIR%\%TARGET_SERVICE%.exe' -WorkingDirectory '%GO_DIR%' -RedirectStandardOutput '%OUT_LOG%' -RedirectStandardError '%ERR_LOG%' -PassThru; if ($null -eq $p) { exit 1 }"
if errorlevel 1 (
    echo Failed to start %TARGET_SERVICE%.
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

:wait_for_port_stable
set "WAIT_SERVICE=%~1"
set "WAIT_PORT=%~2"
set "WAIT_COUNT=0"
:wait_for_port_stable_loop
set /a WAIT_COUNT+=1
powershell -NoProfile -Command "Start-Sleep -Seconds 3" >nul
netstat -ano -p tcp | findstr /R /C:":%WAIT_PORT% .*LISTENING" >nul
if not errorlevel 1 (
    echo [%WAIT_SERVICE%] check %WAIT_COUNT%/3: port %WAIT_PORT% is listening.
) else (
    echo [%WAIT_SERVICE%] check %WAIT_COUNT%/3: port %WAIT_PORT% is not listening.
    exit /b 1
)
if %WAIT_COUNT% LSS 3 goto :wait_for_port_stable_loop
echo PORT %WAIT_PORT% %WAIT_SERVICE% is up.
call :print_service_logs %WAIT_SERVICE%
exit /b 0

:wait_for_process
set "WAIT_SERVICE=%~1"
set "WAIT_COUNT=0"
:wait_for_process_loop
powershell -NoProfile -Command "Start-Sleep -Seconds 3" >nul
set /a WAIT_COUNT+=1
tasklist /FI "IMAGENAME eq %WAIT_SERVICE%.exe" | find /I "%WAIT_SERVICE%.exe" >nul
if not errorlevel 1 (
    echo [%WAIT_SERVICE%] check %WAIT_COUNT%/3: process is running.
) else (
    echo [%WAIT_SERVICE%] check %WAIT_COUNT%/3: process is not running.
    exit /b 1
)
if %WAIT_COUNT% LSS 3 goto :wait_for_process_loop
echo PROCESS %WAIT_SERVICE% is up.
exit /b 0

:print_port_status
set "STATUS_SERVICE=%~1"
set "STATUS_PORT=%~2"
netstat -ano -p tcp | findstr /R /C:":%STATUS_PORT% .*LISTENING" >nul
if not errorlevel 1 (
    echo PORT %STATUS_PORT% %STATUS_SERVICE% is up.
    exit /b 0
)
echo PORT %STATUS_PORT% %STATUS_SERVICE% is down.
exit /b 1

:print_service_logs
set "LOG_SERVICE=%~1"
set "OUT_LOG=%LOG_DIR%\%LOG_SERVICE%.out.log"
set "ERR_LOG=%LOG_DIR%\%LOG_SERVICE%.err.log"
echo ---- %LOG_SERVICE% startup log ----
if exist "%OUT_LOG%" (
    type "%OUT_LOG%"
)
if exist "%ERR_LOG%" (
    for %%I in ("%ERR_LOG%") do if %%~zI GTR 0 type "%ERR_LOG%"
)
echo ---- end %LOG_SERVICE% log ----
exit /b 0

:fail
echo.
echo run.bat failed with exit code %ERRORLEVEL%.
exit /b %ERRORLEVEL%
