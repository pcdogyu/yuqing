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

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Push-Location $repoRoot
try {
    $checkpointJson = & go run ./cmd/ops-check -db $DatabasePath -checkpoint
    if ($LASTEXITCODE -ne 0) {
        throw "source checkpoint failed: $checkpointJson"
    }
    Copy-Item -LiteralPath $DatabasePath -Destination $target -Force
    $sourceInfo = Get-Item -LiteralPath $DatabasePath
    $backupInfo = Get-Item -LiteralPath $target

    $checkJson = & go run ./cmd/ops-check -db $target
    if ($LASTEXITCODE -ne 0) {
        throw "backup verification failed: $checkJson"
    }
} finally {
    Pop-Location
}

[pscustomobject]@{
    status = "ok"
    source = $sourceInfo.FullName
    backup = $backupInfo.FullName
    source_bytes = $sourceInfo.Length
    backup_bytes = $backupInfo.Length
    verified_at = (Get-Date).ToUniversalTime().ToString("o")
    checkpoint = ($checkpointJson | ConvertFrom-Json)
    verification = ($checkJson | ConvertFrom-Json)
} | ConvertTo-Json -Depth 8
