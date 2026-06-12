param(
    [string]$DatabasePath = $env:YUQING_DB_PATH,
    [string]$BackupDir = (Join-Path (Resolve-Path (Join-Path $PSScriptRoot "..")).Path "backups")
)

$ErrorActionPreference = "Stop"
if (-not $DatabasePath) {
    $DatabasePath = Join-Path (Resolve-Path (Join-Path $PSScriptRoot "..")).Path "data\yuqing.db"
}
if (-not (Test-Path $DatabasePath)) {
    throw "database not found: $DatabasePath"
}

New-Item -ItemType Directory -Force -Path $BackupDir | Out-Null
$stamp = Get-Date -Format "yyyyMMdd_HHmmss"
$target = Join-Path $BackupDir ("yuqing_$stamp.db")
Copy-Item -LiteralPath $DatabasePath -Destination $target -Force
Write-Host "backup created: $target"
