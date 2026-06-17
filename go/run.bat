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
if not defined YUQING_GO_TEST_FLAGS (
    set "GO_TEST_FLAGS=-p 3 -count=1 -timeout 5m"
) else (
    set "GO_TEST_FLAGS=%YUQING_GO_TEST_FLAGS%"
)
set "GO_TEST_LOG=%LOG_DIR%\go-test.log"
set "AKSHARE_AUCTION_PORT=19091"
set "AKSHARE_AUCTION_HOST=127.0.0.1"
set "YUQING_ASTOCK_AUCTION_URL_DEFAULTED=0"
set "YUQING_AKSHARE_AUCTION_STARTED=0"
if not defined YUQING_ASTOCK_AUCTION_URL (
    set "YUQING_ASTOCK_AUCTION_URL=http://127.0.0.1:%AKSHARE_AUCTION_PORT%"
    set "YUQING_ASTOCK_AUCTION_URL_DEFAULTED=1"
)
set "SERVICE_PORTS=80 8081 8082 8083 8084 8085 %AKSHARE_AUCTION_PORT%"
set "SERVICE_NAMES=auth-service content-service crawler-service analysis-service nlp-service gateway-web scheduler-service"
set "PORT_CHECKS=auth-service=8081 content-service=8082 crawler-service=8083 analysis-service=8084 nlp-service=8085 gateway-web=80"
set "YUQING_LOG_LEVEL=debug"
set "YUQING_RUN_VERSION=local"
set "TEMP_BOOTSTRAP=%TEMP%\yuqing-run-bootstrap-%RANDOM%-%RANDOM%.cmd"
set "YUQING_SCHEDULER_PORT=8086"

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
echo [1/6] Stop git fsmonitor daemon and prepare pull...
git fsmonitor--daemon stop >nul 2>nul
if "%SKIP_PULL%"=="1" (
    echo Skip pull requested. Continue with current worktree.
) else if "%AFTER_PULL%"=="1" (
    echo [1/6] Pull latest code from origin...
    for /f %%I in ('git rev-parse HEAD') do set "YUQING_HEAD_BEFORE=%%I"
    if not defined YUQING_HEAD_BEFORE (
        echo Failed to resolve current git HEAD before pull.
        goto :fail
    )
    git diff --quiet -- go/data/yuqing.db go/data/yuqing.db-shm go/data/yuqing.db-wal >nul 2>nul
    if errorlevel 1 (
        echo Detected local database changes under go/data. Skipping git pull to preserve local data.
    ) else (
        git pull --ff-only
        if errorlevel 1 goto :fail
        for /f %%I in ('git rev-parse HEAD') do set "YUQING_HEAD_AFTER=%%I"
        if not defined YUQING_HEAD_AFTER (
            echo Failed to resolve git HEAD after pull.
            goto :fail
        )
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
pushd "%REPO_ROOT%" >nul
for /f %%I in ('git rev-parse --short HEAD') do set "YUQING_GIT_COMMIT=%%I"
for /f %%I in ('git rev-parse --abbrev-ref HEAD') do set "YUQING_GIT_BRANCH=%%I"
popd >nul
for /f "usebackq delims=" %%I in (`powershell -NoProfile -Command "([TimeZoneInfo]::ConvertTimeBySystemTimeZoneId((Get-Date), 'China Standard Time')).ToString('yyyy-MM-ddTHH:mm:ss') + '+08:00'"`) do set "YUQING_BUILD_TIME=%%I"
if not defined YUQING_GIT_COMMIT set "YUQING_GIT_COMMIT=unknown"
if not defined YUQING_GIT_BRANCH set "YUQING_GIT_BRANCH=unknown"
if not defined YUQING_BUILD_TIME set "YUQING_BUILD_TIME=unknown"
set "LDFLAGS=-X github.com/pcdogyu/yuqing/go/internal/app.Version=%YUQING_RUN_VERSION% -X github.com/pcdogyu/yuqing/go/internal/app.GitCommit=%YUQING_GIT_COMMIT% -X github.com/pcdogyu/yuqing/go/internal/app.BuildTime=%YUQING_BUILD_TIME% -X github.com/pcdogyu/yuqing/go/internal/app.BranchName=%YUQING_GIT_BRANCH%"

if not exist "%LOG_DIR%" mkdir "%LOG_DIR%"

echo [3/6] Run go test ./...
powershell -NoProfile -NonInteractive -InputFormat None -Command "& { Set-Location -LiteralPath '%GO_DIR%'; if (Test-Path -LiteralPath '%GO_TEST_LOG%') { Remove-Item -LiteralPath '%GO_TEST_LOG%' -Force -ErrorAction SilentlyContinue }; Write-Host ('Go test flags: %GO_TEST_FLAGS%'); Write-Host ('Go test log: %GO_TEST_LOG%'); & go test ./... %GO_TEST_FLAGS% 2>&1 | Tee-Object -FilePath '%GO_TEST_LOG%'; exit $LASTEXITCODE }"
if errorlevel 1 (
    call :print_go_test_failures
    goto :fail
)

echo [4/6] Stop processes occupying service ports...
for %%P in (%SERVICE_PORTS%) do (
    call :kill_port %%P
    if errorlevel 1 goto :fail
)
for %%S in (%SERVICE_NAMES%) do (
    call :kill_service %%S
    if errorlevel 1 goto :fail
)
call :configure_scheduler_port
if errorlevel 1 goto :fail

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
    go build -ldflags "%LDFLAGS%" -o "%BIN_DIR%\%%S.exe" "./cmd/%%S"
    if errorlevel 1 goto :fail
)

echo [6/6] Start services with debug logging...
call :start_process_core auth-service
if errorlevel 1 goto :fail
call :start_process_core content-service
if errorlevel 1 goto :fail
call :start_process_core crawler-service
if errorlevel 1 goto :fail
call :start_process_core analysis-service
if errorlevel 1 goto :fail
call :start_process_core nlp-service
if errorlevel 1 goto :fail
call :start_process_core gateway-web
if errorlevel 1 goto :fail
call :start_akshare_auction_service
powershell -NoProfile -Command "$checks = @(@{Name='auth-service';Port=8081}, @{Name='content-service';Port=8082}, @{Name='crawler-service';Port=8083}, @{Name='analysis-service';Port=8084}, @{Name='nlp-service';Port=8085}, @{Name='gateway-web';Port=80}); $counts = @{}; foreach ($check in $checks) { $counts[$check.Name] = 0 }; while ($true) { Start-Sleep -Seconds 3; $allDone = $true; foreach ($check in $checks) { if ($counts[$check.Name] -ge 3) { Write-Host ('[{0}] check {1}/3: port {2} is listening.' -f $check.Name, $counts[$check.Name], $check.Port); continue }; $listening = Get-NetTCPConnection -LocalPort $check.Port -State Listen -ErrorAction SilentlyContinue; if ($listening) { $counts[$check.Name]++; Write-Host ('[{0}] check {1}/3: port {2} is listening.' -f $check.Name, $counts[$check.Name], $check.Port) } else { Write-Host ('[{0}] check {1}/3: port {2} is not listening.' -f $check.Name, $counts[$check.Name], $check.Port); exit 1 }; if ($counts[$check.Name] -lt 3) { $allDone = $false } }; if ($allDone) { break } }"
if errorlevel 1 goto :fail
for %%C in (%PORT_CHECKS%) do (
    for /f "tokens=1,2 delims==" %%A in ("%%C") do (
        echo PORT %%B %%A is up.
    )
)
call :start_process_service scheduler-service
if errorlevel 1 goto :fail

echo Final port checks:
call :print_port_status gateway-web 80
call :print_port_status auth-service 8081
call :print_port_status content-service 8082
call :print_port_status crawler-service 8083
call :print_port_status analysis-service 8084
call :print_port_status nlp-service 8085
if "%YUQING_AKSHARE_AUCTION_STARTED%"=="1" (
    call :print_port_status akshare-auction-service %AKSHARE_AUCTION_PORT%
) else (
    echo PORT %AKSHARE_AUCTION_PORT% akshare-auction-service is skipped.
)
call :print_port_status scheduler-service %YUQING_SCHEDULER_PORT%

echo.
echo Services started.
echo Gateway: http://127.0.0.1
echo Scheduler: %YUQING_SCHEDULER_URL%
if defined YUQING_ASTOCK_AUCTION_URL (
    echo AKShareAuction: %YUQING_ASTOCK_AUCTION_URL%
) else (
    echo AKShareAuction: disabled ^(Python/AKShare service not available^)
)
echo LogLevel: %YUQING_LOG_LEVEL%
echo Version: %YUQING_RUN_VERSION%
echo Commit: %YUQING_GIT_COMMIT%
echo BuildTime: %YUQING_BUILD_TIME%
echo Branch: %YUQING_GIT_BRANCH%
exit /b 0

:find_python
set "PYTHON_EXE="
set "PYTHON_LAUNCH_ARGS="
for /f "delims=" %%I in ('where python 2^>nul') do (
    if not defined PYTHON_EXE set "PYTHON_EXE=%%I"
)
if defined PYTHON_EXE exit /b 0
for /f "delims=" %%I in ('where py 2^>nul') do (
    if not defined PYTHON_EXE set "PYTHON_EXE=%%I"
)
if defined PYTHON_EXE (
    set "PYTHON_LAUNCH_ARGS=-3"
    exit /b 0
)
echo WARNING: Python was not found. AKShare auction service will be skipped.
echo Install Python 3 and rerun run.bat, or set YUQING_AKSHARE_PYTHON to python.exe.
exit /b 1

:ensure_akshare_deps
if defined YUQING_AKSHARE_PYTHON (
    if not exist "%YUQING_AKSHARE_PYTHON%" (
        echo WARNING: YUQING_AKSHARE_PYTHON does not exist: %YUQING_AKSHARE_PYTHON%
        echo AKShare auction service will be skipped.
        exit /b 1
    )
    set "PYTHON_EXE=%YUQING_AKSHARE_PYTHON%"
    set "PYTHON_LAUNCH_ARGS="
) else (
    call :find_python
    if errorlevel 1 exit /b 1
)
"%PYTHON_EXE%" %PYTHON_LAUNCH_ARGS% -c "import akshare" >nul 2>nul
if not errorlevel 1 exit /b 0
echo Installing AKShare Python dependencies...
"%PYTHON_EXE%" %PYTHON_LAUNCH_ARGS% -m pip install -r "%GO_DIR%\requirements-akshare.txt"
if errorlevel 1 (
    echo WARNING: Failed to install AKShare dependencies. AKShare auction service will be skipped.
    echo Run manually: python -m pip install -r "%GO_DIR%\requirements-akshare.txt"
    exit /b 1
)
exit /b 0

:start_akshare_auction_service
call :ensure_akshare_deps
if errorlevel 1 (
    if "%YUQING_ASTOCK_AUCTION_URL_DEFAULTED%"=="1" set "YUQING_ASTOCK_AUCTION_URL="
    exit /b 0
)
set "TARGET_SERVICE=akshare-auction-service"
set "OUT_LOG=%LOG_DIR%\%TARGET_SERVICE%.out.log"
set "ERR_LOG=%LOG_DIR%\%TARGET_SERVICE%.err.log"
if exist "%OUT_LOG%" del /Q "%OUT_LOG%" >nul 2>nul
if exist "%ERR_LOG%" del /Q "%ERR_LOG%" >nul 2>nul
echo Starting %TARGET_SERVICE%...
powershell -NoProfile -Command "$argsList = @(); if ('%PYTHON_LAUNCH_ARGS%' -ne '') { $argsList += '%PYTHON_LAUNCH_ARGS%' }; $argsList += @('%GO_DIR%\services\akshare_auction_service.py','--host','%AKSHARE_AUCTION_HOST%','--port','%AKSHARE_AUCTION_PORT%'); $p = Start-Process -FilePath '%PYTHON_EXE%' -ArgumentList $argsList -WorkingDirectory '%GO_DIR%' -RedirectStandardOutput '%OUT_LOG%' -RedirectStandardError '%ERR_LOG%' -PassThru -WindowStyle Hidden; if ($null -eq $p) { exit 1 }"
if errorlevel 1 (
    echo WARNING: Failed to start %TARGET_SERVICE%.
    if "%YUQING_ASTOCK_AUCTION_URL_DEFAULTED%"=="1" set "YUQING_ASTOCK_AUCTION_URL="
    exit /b 0
)
call :wait_for_port %TARGET_SERVICE% %AKSHARE_AUCTION_PORT%
if errorlevel 1 (
    echo WARNING: %TARGET_SERVICE% did not open port %AKSHARE_AUCTION_PORT%.
    if "%YUQING_ASTOCK_AUCTION_URL_DEFAULTED%"=="1" set "YUQING_ASTOCK_AUCTION_URL="
    exit /b 0
)
set "YUQING_AKSHARE_AUCTION_STARTED=1"
exit /b 0

:wait_for_port
set "WAIT_SERVICE=%~1"
set "WAIT_PORT=%~2"
set "WAIT_COUNT=0"
:wait_for_port_loop
powershell -NoProfile -Command "Start-Sleep -Seconds 2" >nul
set /a WAIT_COUNT+=1
netstat -ano -p tcp | findstr /R /C:":%WAIT_PORT% .*LISTENING" >nul
if not errorlevel 1 (
    echo [%WAIT_SERVICE%] check %WAIT_COUNT%/3: port %WAIT_PORT% is listening.
) else (
    echo [%WAIT_SERVICE%] check %WAIT_COUNT%/3: port %WAIT_PORT% is not listening.
    if %WAIT_COUNT% GEQ 3 exit /b 1
    goto :wait_for_port_loop
)
if %WAIT_COUNT% LSS 3 goto :wait_for_port_loop
echo PORT %WAIT_PORT% %WAIT_SERVICE% is up.
exit /b 0

:start_process_service
set "TARGET_SERVICE=%~1"
call :start_process_core %TARGET_SERVICE%
if errorlevel 1 exit /b 1
call :wait_for_process %TARGET_SERVICE%
if errorlevel 1 exit /b 1
exit /b 0

:start_process_core
set "TARGET_SERVICE=%~1"
set "OUT_LOG=%LOG_DIR%\%TARGET_SERVICE%.out.log"
set "ERR_LOG=%LOG_DIR%\%TARGET_SERVICE%.err.log"
if exist "%OUT_LOG%" del /Q "%OUT_LOG%" >nul 2>nul
if exist "%ERR_LOG%" del /Q "%ERR_LOG%" >nul 2>nul
echo Starting %TARGET_SERVICE%...
powershell -NoProfile -Command "$p = Start-Process -FilePath '%BIN_DIR%\%TARGET_SERVICE%.exe' -WorkingDirectory '%GO_DIR%' -PassThru; if ($null -eq $p) { exit 1 }"
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

:print_go_test_failures
if exist "%GO_TEST_LOG%" (
    echo ---- go test failure summary ----
    powershell -NoProfile -Command "Select-String -Path '%GO_TEST_LOG%' -Pattern '^FAIL|--- FAIL|panic:|build failed|\[build failed\]|WaitDelay|Test I/O incomplete' -Context 4,8 | ForEach-Object { $_.ToString() }"
    echo ---- end go test failure summary ----
) else (
    echo Go test log not found: %GO_TEST_LOG%
)
exit /b 0

:configure_scheduler_port
for /f "usebackq tokens=1,2,3 delims=|" %%A in (`powershell -NoProfile -Command "$addr=$env:YUQING_SCHEDULER_ADDR; if ([string]::IsNullOrWhiteSpace($addr)) { $addr=':8086' }; if ($addr -match ':(\d+)$') { $port=[int]$Matches[1] } else { $port=8086 }; function Test-Port([int]$p) { return @((Get-NetTCPConnection -LocalPort $p -State Listen -ErrorAction SilentlyContinue)).Count -gt 0 }; $changed=$false; if (Test-Port $port) { $found=$false; for ($p=18086; $p -le 18186; $p++) { if (-not (Test-Port $p)) { $port=$p; $addr=':'+$p; $changed=$true; $found=$true; break } }; if (-not $found) { throw 'no free scheduler port found from 18086' } }; $url=$env:YUQING_SCHEDULER_URL; if ([string]::IsNullOrWhiteSpace($url) -or $changed -or $url -match ':8086/?$') { $url='http://127.0.0.1:'+$port }; Write-Output ($addr+'|'+$url+'|'+$port)"`) do (
    set "YUQING_SCHEDULER_ADDR=%%A"
    set "YUQING_SCHEDULER_URL=%%B"
    set "YUQING_SCHEDULER_PORT=%%C"
)
if not defined YUQING_SCHEDULER_ADDR (
    echo Failed to configure scheduler port.
    exit /b 1
)
echo Scheduler management endpoint: %YUQING_SCHEDULER_URL%
exit /b 0

:fail
echo.
echo run.bat failed with exit code %ERRORLEVEL%.
exit /b %ERRORLEVEL%
