param(
    [string]$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
)

$ErrorActionPreference = "Stop"
$pidDir = Join-Path $Root "runtime-pids"

if (-not (Test-Path $pidDir)) {
    Write-Host "runtime-pids directory does not exist"
    exit 0
}

Get-ChildItem -Path $pidDir -Filter "*.pid" | ForEach-Object {
    $pidValue = Get-Content $_.FullName -ErrorAction SilentlyContinue
    if ($pidValue) {
        $process = Get-Process -Id $pidValue -ErrorAction SilentlyContinue
        if ($process) {
            Stop-Process -Id $process.Id -Force
            Write-Host "stopped $($_.BaseName) PID $($process.Id)"
        }
    }
    Remove-Item -LiteralPath $_.FullName -Force
}

foreach ($name in @(
    "auth-service",
    "wechat-service",
    "content-service",
    "crawler-service",
    "analysis-service",
    "nlp-service",
    "scheduler-service",
    "gateway-web"
)) {
    Get-Process -Name $name -ErrorAction SilentlyContinue | ForEach-Object {
        Stop-Process -Id $_.Id -Force
        Write-Host "stopped $name child PID $($_.Id)"
    }
}
