param(
    [string]$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
)

$ErrorActionPreference = "Stop"
Set-Location $Root

$env:YUQING_DB_PATH = if ($env:YUQING_DB_PATH) { $env:YUQING_DB_PATH } else { Join-Path $Root "data\yuqing.db" }

$services = @(
    @{ Name = "auth-service"; Path = ".\cmd\auth-service" },
    @{ Name = "content-service"; Path = ".\cmd\content-service" },
    @{ Name = "crawler-service"; Path = ".\cmd\crawler-service" },
    @{ Name = "analysis-service"; Path = ".\cmd\analysis-service" },
    @{ Name = "nlp-service"; Path = ".\cmd\nlp-service" },
    @{ Name = "scheduler-service"; Path = ".\cmd\scheduler-service" },
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
