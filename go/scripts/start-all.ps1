param(
    [string]$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
)

$ErrorActionPreference = "Stop"
Set-Location $Root

$dbConfigPath = if ($env:YUQING_DB_CONFIG_PATH) { $env:YUQING_DB_CONFIG_PATH } else { Join-Path $Root "data\database-config.json" }
$env:YUQING_DB_CONFIG_PATH = $dbConfigPath
$dbRuntimeConfig = $null
if (Test-Path $dbConfigPath) {
    try {
        $dbRuntimeConfig = Get-Content -Raw -Path $dbConfigPath | ConvertFrom-Json
    } catch {
        Write-Warning "failed to read database config '$dbConfigPath': $($_.Exception.Message)"
    }
}
if ($dbRuntimeConfig) {
    if (-not $env:YUQING_DB_DRIVER -and $dbRuntimeConfig.driver) { $env:YUQING_DB_DRIVER = $dbRuntimeConfig.driver }
    if (-not $env:YUQING_DB_PATH -and $dbRuntimeConfig.sqlite_path) { $env:YUQING_DB_PATH = $dbRuntimeConfig.sqlite_path }
    if (-not $env:YUQING_DATABASE_URL -and $dbRuntimeConfig.postgres_dsn) { $env:YUQING_DATABASE_URL = $dbRuntimeConfig.postgres_dsn }
    if (-not $env:YUQING_POSTGRES_HOST -and $dbRuntimeConfig.postgres_host) { $env:YUQING_POSTGRES_HOST = $dbRuntimeConfig.postgres_host }
    if (-not $env:YUQING_POSTGRES_PORT -and $dbRuntimeConfig.postgres_port) { $env:YUQING_POSTGRES_PORT = $dbRuntimeConfig.postgres_port }
    if (-not $env:YUQING_POSTGRES_DB -and $dbRuntimeConfig.postgres_database) { $env:YUQING_POSTGRES_DB = $dbRuntimeConfig.postgres_database }
    if (-not $env:YUQING_POSTGRES_USER -and $dbRuntimeConfig.postgres_user) { $env:YUQING_POSTGRES_USER = $dbRuntimeConfig.postgres_user }
    if (-not $env:YUQING_POSTGRES_PASSWORD -and $dbRuntimeConfig.postgres_password) { $env:YUQING_POSTGRES_PASSWORD = $dbRuntimeConfig.postgres_password }
    if (-not $env:YUQING_POSTGRES_SSLMODE -and $dbRuntimeConfig.postgres_sslmode) { $env:YUQING_POSTGRES_SSLMODE = $dbRuntimeConfig.postgres_sslmode }
}
$env:YUQING_DB_PATH = if ($env:YUQING_DB_PATH) { $env:YUQING_DB_PATH } else { Join-Path $Root "data\yuqing.db" }
$repoRoot = (Resolve-Path (Join-Path $Root "..")).Path
$env:YUQING_RELEASE_ADDR = if ($env:YUQING_RELEASE_ADDR) { $env:YUQING_RELEASE_ADDR } else { ":8099" }
$env:YUQING_RELEASE_URL = if ($env:YUQING_RELEASE_URL) { $env:YUQING_RELEASE_URL } else { "http://127.0.0.1:8099" }
$env:YUQING_RELEASE_DIR = if ($env:YUQING_RELEASE_DIR) { $env:YUQING_RELEASE_DIR } else { Join-Path $repoRoot "release" }

$services = @(
    @{ Name = "auth-service"; Path = ".\cmd\auth-service" },
    @{ Name = "wechat-service"; Path = ".\cmd\wechat-service" },
    @{ Name = "content-service"; Path = ".\cmd\content-service" },
    @{ Name = "crawler-service"; Path = ".\cmd\crawler-service" },
    @{ Name = "analysis-service"; Path = ".\cmd\analysis-service" },
    @{ Name = "nlp-service"; Path = ".\cmd\nlp-service" },
    @{ Name = "scheduler-service"; Path = ".\cmd\scheduler-service" },
    @{ Name = "release-service"; Path = ".\cmd\release-service" },
    @{ Name = "gateway-web"; Path = ".\cmd\gateway-web" }
)

$pidDir = Join-Path $Root "runtime-pids"
New-Item -ItemType Directory -Force -Path $pidDir | Out-Null

foreach ($service in $services) {
    $pidFile = Join-Path $pidDir ($service.Name + ".pid")
    if (Test-Path $pidFile) {
        $oldPid = Get-Content $pidFile -ErrorAction SilentlyContinue
        if ($oldPid -and (Get-Process -Id $oldPid -ErrorAction SilentlyContinue)) {
            Write-Host "$($service.Name) already running as PID $oldPid"
            continue
        }
    }
    $process = Start-Process -FilePath "go" -ArgumentList @("run", $service.Path) -WorkingDirectory $Root -PassThru -WindowStyle Hidden
    Set-Content -Path $pidFile -Value $process.Id
    Write-Host "started $($service.Name) as PID $($process.Id)"
}
