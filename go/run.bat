@echo off
setlocal EnableExtensions EnableDelayedExpansion

set "STATUS_ONLY=0"
if /I "%~1"=="status" set "STATUS_ONLY=1"
if /I "%~1"=="--status" set "STATUS_ONLY=1"
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
if not defined YUQING_RUN_GO_TEST set "YUQING_RUN_GO_TEST=0"
if not defined YUQING_GO_TEST_FLAGS (
    set "GO_TEST_FLAGS=-p 3 -count=1 -timeout 5m"
) else (
    set "GO_TEST_FLAGS=%YUQING_GO_TEST_FLAGS%"
)
if defined YUQING_GO_BUILD_FLAGS (
    set "GO_BUILD_FLAGS=%YUQING_GO_BUILD_FLAGS%"
) else (
    set "GO_BUILD_FLAGS="
)
set "AKSHARE_AUCTION_PORT=8087"
set "AKSHARE_AUCTION_HOST=127.0.0.1"
set "GATEWAY_WEB_PORT=8079"
set "WECHAT_SERVICE_PORT=8088"
set "RELEASE_SERVICE_PORT=8099"
set "YUQING_ASTOCK_AUCTION_URL_DEFAULTED=0"
set "YUQING_ASTOCK_HOLDING_URL_DEFAULTED=0"
set "YUQING_AKSHARE_AUCTION_STARTED=0"
if not defined YUQING_RELEASE_ADDR (
    set "YUQING_RELEASE_ADDR=:%RELEASE_SERVICE_PORT%"
)
if not defined YUQING_RELEASE_URL (
    set "YUQING_RELEASE_URL=http://127.0.0.1:%RELEASE_SERVICE_PORT%"
)
if not defined YUQING_RELEASE_DIR (
    set "YUQING_RELEASE_DIR=%REPO_ROOT%\release"
)
if not defined YUQING_SCHEDULER_ADDR (
    set "YUQING_SCHEDULER_ADDR=:8086"
)
if not defined YUQING_SCHEDULER_URL (
    set "YUQING_SCHEDULER_URL=http://127.0.0.1:8086"
)
if not defined YUQING_WECHAT_ADDR (
    set "YUQING_WECHAT_ADDR=:%WECHAT_SERVICE_PORT%"
)
if not defined YUQING_WECHAT_URL (
    set "YUQING_WECHAT_URL=http://127.0.0.1:%WECHAT_SERVICE_PORT%"
)
if not defined YUQING_GATEWAY_ADDR (
    set "YUQING_GATEWAY_ADDR=:%GATEWAY_WEB_PORT%"
)
if not defined YUQING_GATEWAY_HTTP_ADDRS (
    if defined YUQING_GATEWAY_TLS_CERT_FILE (
        set "YUQING_GATEWAY_HTTP_ADDRS=:%GATEWAY_WEB_PORT%"
    ) else if defined YUQING_GATEWAY_TLS_KEY_FILE (
        set "YUQING_GATEWAY_HTTP_ADDRS=:%GATEWAY_WEB_PORT%"
    ) else (
        set "YUQING_GATEWAY_HTTP_ADDRS=:%GATEWAY_WEB_PORT%,:80"
    )
)
if not defined YUQING_GATEWAY_URL (
    set "YUQING_GATEWAY_URL=http://127.0.0.1:%GATEWAY_WEB_PORT%"
)
if not defined YUQING_ASTOCK_AUCTION_URL (
    set "YUQING_ASTOCK_AUCTION_URL=http://127.0.0.1:%AKSHARE_AUCTION_PORT%"
    set "YUQING_ASTOCK_AUCTION_URL_DEFAULTED=1"
)
if not defined YUQING_ASTOCK_HOLDING_URL (
    set "YUQING_ASTOCK_HOLDING_URL=http://127.0.0.1:%AKSHARE_AUCTION_PORT%"
    set "YUQING_ASTOCK_HOLDING_URL_DEFAULTED=1"
)
set "SERVICE_NAMES=auth-service wechat-service content-service crawler-service analysis-service nlp-service gateway-web scheduler-service release-service akshare-service"
set "YUQING_LOG_LEVEL=debug"
set "YUQING_RUN_VERSION=local"
set "TEMP_BOOTSTRAP=%TEMP%\yuqing-run-bootstrap-%RANDOM%-%RANDOM%.cmd"
set "GIT_ASK_YESNO_HELPER=%TEMP%\yuqing-git-ask-yesno-%RANDOM%-%RANDOM%.cmd"

if "%STATUS_ONLY%"=="1" (
    call :print_service_status
    exit /b !ERRORLEVEL!
)

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
        >"%GIT_ASK_YESNO_HELPER%" (
            echo @echo off
            echo rem Auto-answer "No" to git retry prompts such as unlink/rmdir failures during pull.
            echo exit /b 1
        )
        if errorlevel 1 (
            echo Failed to create git yes/no helper: %GIT_ASK_YESNO_HELPER%
            goto :fail
        )
        set "GIT_TERMINAL_PROMPT=0"
        set "GIT_ASK_YESNO=%GIT_ASK_YESNO_HELPER%"
        git pull --ff-only origin golang
        set "YUQING_PULL_EXIT=!ERRORLEVEL!"
        set "GIT_ASK_YESNO="
        set "GIT_TERMINAL_PROMPT="
        del /Q "%GIT_ASK_YESNO_HELPER%" >nul 2>nul
        if not "!YUQING_PULL_EXIT!"=="0" (
            echo git pull failed with exit code !YUQING_PULL_EXIT!.
            exit /b !YUQING_PULL_EXIT!
        )
        for /f %%I in ('git rev-parse HEAD') do set "YUQING_HEAD_AFTER=%%I"
        if not defined YUQING_HEAD_AFTER (
            echo Failed to resolve git HEAD after pull.
            goto :fail
        )
        if not "!YUQING_HEAD_BEFORE!"=="!YUQING_HEAD_AFTER!" (
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

echo [3/6] Skip go test during startup...
if "%YUQING_RUN_GO_TEST%"=="1" (
    echo YUQING_RUN_GO_TEST=1, running go test ./... before startup.
    echo Go test flags: %GO_TEST_FLAGS%
    go test ./... %GO_TEST_FLAGS%
    if errorlevel 1 goto :fail
    echo go test completed.
) else (
    echo Startup tests are disabled by default. Set YUQING_RUN_GO_TEST=1 to run them manually before startup.
)

echo [4/6] Stop existing service processes...
for %%S in (%SERVICE_NAMES%) do (
    call :kill_service %%S
    if errorlevel 1 goto :fail
)
call :ensure_release_port
if errorlevel 1 goto :fail

echo [5/6] Build service binaries concurrently...
if not exist "%BIN_DIR%" mkdir "%BIN_DIR%"
if defined GO_BUILD_FLAGS echo Go build flags: %GO_BUILD_FLAGS%
go build %GO_BUILD_FLAGS% -ldflags "%LDFLAGS%" -o "%BIN_DIR%\\" "./cmd/auth-service" "./cmd/wechat-service" "./cmd/content-service" "./cmd/crawler-service" "./cmd/analysis-service" "./cmd/nlp-service" "./cmd/gateway-web" "./cmd/akshare-service" "./cmd/scheduler-service" "./cmd/release-service"
if errorlevel 1 goto :fail

echo [6/6] Start services with debug logging...
for %%S in (
    auth-service
    wechat-service
    content-service
    crawler-service
    analysis-service
    nlp-service
    gateway-web
    scheduler-service
    release-service
) do (
    call :start_process_core %%S
    if errorlevel 1 goto :fail
)
call :start_akshare_auction_service
if errorlevel 1 goto :fail

echo.
echo Services started.
echo Gateway: http://127.0.0.1:%GATEWAY_WEB_PORT%
echo Gateway80: http://127.0.0.1/
echo Wechat: %YUQING_WECHAT_URL%
echo Scheduler: %YUQING_SCHEDULER_URL%
echo Release: %YUQING_RELEASE_URL%
echo ReleasePort: %RELEASE_SERVICE_PORT%
if defined YUQING_ASTOCK_AUCTION_URL (
    echo AKShareAuction: %YUQING_ASTOCK_AUCTION_URL%
) else (
    echo AKShareAuction: disabled ^(Python/AKShare service not available^)
)
if defined YUQING_STOCK_RESEARCH_URL (
    echo StockResearch: %YUQING_STOCK_RESEARCH_URL%
) else (
    echo StockResearch: public sources only ^(AKShare service not available^)
)
if defined YUQING_ASTOCK_HOLDING_URL (
    echo AStockHolding: %YUQING_ASTOCK_HOLDING_URL%
) else (
    echo AStockHolding: disabled ^(Python/AKShare service not available^)
)
echo LogLevel: %YUQING_LOG_LEVEL%
echo Version: %YUQING_RUN_VERSION%
echo Commit: %YUQING_GIT_COMMIT%
echo BuildTime: %YUQING_BUILD_TIME%
echo Branch: %YUQING_GIT_BRANCH%
echo.
echo Service status:
call :print_service_status
if errorlevel 1 echo WARNING: Failed to print service status.
exit /b 0

:print_service_status
powershell -NoProfile -ExecutionPolicy Bypass -File "%GO_DIR%\scripts\service-status.ps1" -LogDir "%LOG_DIR%"
exit /b %ERRORLEVEL%

:ensure_release_port
echo Checking release-service port...
set "DETECTED_RELEASE_PORT="
set "RELEASE_PORT_WAS_BUSY=0"
for /f "usebackq tokens=1,2" %%A in (`powershell -NoProfile -ExecutionPolicy Bypass -Command "$ErrorActionPreference='Stop'; function Get-UrlPort([string]$url, [int]$fallback) { if ([string]::IsNullOrWhiteSpace($url)) { return $fallback }; try { $uri = [uri]$url; if (-not $uri.IsDefaultPort) { return $uri.Port }; if ($uri.Scheme -eq 'https') { return 443 }; return 80 } catch { return $fallback } }; function Test-PortInUse([int]$port) { return @((Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue)).Count -gt 0 }; function Find-FreePort([int]$startPort) { for ($port = $startPort; $port -le ($startPort + 100); $port++) { if (-not (Test-PortInUse $port)) { return $port } }; throw \"no free port found from $startPort\" }; $defaultPort = [int]$env:RELEASE_SERVICE_PORT; $url = $env:YUQING_RELEASE_URL; $port = Get-UrlPort $url $defaultPort; $busy = 0; if ($url -eq ('http://127.0.0.1:' + $defaultPort) -and (Test-PortInUse $port)) { $port = Find-FreePort 18099; $busy = 1 }; Write-Output (\"$port $busy\")"`) do (
    set "DETECTED_RELEASE_PORT=%%A"
    set "RELEASE_PORT_WAS_BUSY=%%B"
)
if not defined DETECTED_RELEASE_PORT (
    echo Failed to resolve release-service port.
    exit /b 1
)
set "RELEASE_SERVICE_PORT=%DETECTED_RELEASE_PORT%"
set "YUQING_RELEASE_ADDR=:%RELEASE_SERVICE_PORT%"
if "%RELEASE_PORT_WAS_BUSY%"=="1" (
    set "YUQING_RELEASE_URL=http://127.0.0.1:%RELEASE_SERVICE_PORT%"
    echo Release-service default port 8099 is busy. Using !YUQING_RELEASE_URL!.
) else (
    echo Release-service port: %RELEASE_SERVICE_PORT%
)
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
    if "%YUQING_ASTOCK_HOLDING_URL_DEFAULTED%"=="1" set "YUQING_ASTOCK_HOLDING_URL="
    exit /b 0
)
set "TARGET_SERVICE=akshare-service"
set "OUT_LOG=%LOG_DIR%\%TARGET_SERVICE%.out.log"
set "ERR_LOG=%LOG_DIR%\%TARGET_SERVICE%.err.log"
if exist "%OUT_LOG%" del /Q "%OUT_LOG%" >nul 2>nul
if exist "%ERR_LOG%" del /Q "%ERR_LOG%" >nul 2>nul
echo Starting %TARGET_SERVICE%...
powershell -NoProfile -Command "$q=[char]34; $argsList = '--host '+$q+'%AKSHARE_AUCTION_HOST%'+$q+' --port '+$q+'%AKSHARE_AUCTION_PORT%'+$q+' --python '+$q+'%PYTHON_EXE%'+$q; if ('%PYTHON_LAUNCH_ARGS%' -ne '') { $argsList += ' --python-arg '+$q+'%PYTHON_LAUNCH_ARGS%'+$q }; $p = Start-Process -FilePath '%BIN_DIR%\%TARGET_SERVICE%.exe' -ArgumentList $argsList -WorkingDirectory '%GO_DIR%' -RedirectStandardOutput '%OUT_LOG%' -RedirectStandardError '%ERR_LOG%' -PassThru -WindowStyle Hidden; if ($null -eq $p) { exit 1 }"
if errorlevel 1 (
    echo WARNING: Failed to start %TARGET_SERVICE%.
    if "%YUQING_ASTOCK_AUCTION_URL_DEFAULTED%"=="1" set "YUQING_ASTOCK_AUCTION_URL="
    if "%YUQING_ASTOCK_HOLDING_URL_DEFAULTED%"=="1" set "YUQING_ASTOCK_HOLDING_URL="
    exit /b 0
)
set "YUQING_AKSHARE_AUCTION_STARTED=1"
if not defined YUQING_STOCK_RESEARCH_URL (
    set "YUQING_STOCK_RESEARCH_URL=http://127.0.0.1:%AKSHARE_AUCTION_PORT%"
)
exit /b 0

:start_process_core
set "TARGET_SERVICE=%~1"
set "OUT_LOG=%LOG_DIR%\%TARGET_SERVICE%.out.log"
set "ERR_LOG=%LOG_DIR%\%TARGET_SERVICE%.err.log"
if exist "%OUT_LOG%" del /Q "%OUT_LOG%" >nul 2>nul
if exist "%ERR_LOG%" del /Q "%ERR_LOG%" >nul 2>nul
echo Starting %TARGET_SERVICE%...
powershell -NoProfile -Command "$p = Start-Process -FilePath '%BIN_DIR%\%TARGET_SERVICE%.exe' -WorkingDirectory '%GO_DIR%' -RedirectStandardOutput '%OUT_LOG%' -RedirectStandardError '%ERR_LOG%' -PassThru -WindowStyle Hidden; if ($null -eq $p) { exit 1 }"
if errorlevel 1 (
    echo Failed to start %TARGET_SERVICE%.
    exit /b 1
)
exit /b 0

:kill_service
set "TARGET_SERVICE=%~1"
if /I "%TARGET_SERVICE%"=="akshare-service" (
    call :kill_akshare_python
    if errorlevel 1 exit /b 1
)
tasklist /FI "IMAGENAME eq %TARGET_SERVICE%.exe" | find /I "%TARGET_SERVICE%.exe" >nul
if errorlevel 1 (
    echo Service %TARGET_SERVICE% is not running.
    exit /b 0
)
powershell -NoProfile -ExecutionPolicy Bypass -File "%GO_DIR%\scripts\stop-service.ps1" -Name "%TARGET_SERVICE%"
if errorlevel 1 (
    echo Failed to stop %TARGET_SERVICE%.exe.
    exit /b 1
)
exit /b 0

:kill_akshare_python
powershell -NoProfile -Command "$targets = Get-CimInstance Win32_Process | Where-Object { $_.Name -match '^(python|py)\.exe$' -and $_.CommandLine -match 'akshare_auction_service\.py' }; foreach ($proc in $targets) { try { Stop-Process -Id $proc.ProcessId -Force -ErrorAction Stop; Write-Host ('Stopped AKShare python process ' + $proc.ProcessId) } catch { Write-Host ('Failed to stop AKShare python process ' + $proc.ProcessId); exit 1 } }"
if errorlevel 1 (
    echo Failed to stop AKShare python process.
    exit /b 1
)
exit /b 0

:fail
set "YUQING_FAIL_EXIT=%ERRORLEVEL%"
if "!YUQING_FAIL_EXIT!"=="0" set "YUQING_FAIL_EXIT=1"
echo.
echo run.bat failed with exit code !YUQING_FAIL_EXIT!.
exit /b !YUQING_FAIL_EXIT!
