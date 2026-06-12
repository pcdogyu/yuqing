param(
    [string]$BackupPath = "",
    [string]$BackupDir = (Join-Path (Resolve-Path (Join-Path $PSScriptRoot "..")).Path "backups")
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path

if (-not $BackupPath) {
    if (-not (Test-Path $BackupDir)) {
        throw "backup directory not found: $BackupDir"
    }
    $latest = Get-ChildItem -LiteralPath $BackupDir -Filter *.db | Sort-Object LastWriteTime -Descending | Select-Object -First 1
    if (-not $latest) {
        throw "no sqlite backup found in $BackupDir"
    }
    $BackupPath = $latest.FullName
}
if (-not (Test-Path $BackupPath)) {
    throw "backup not found: $BackupPath"
}

$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("yuqing_restore_" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempDir | Out-Null
$restorePath = Join-Path $tempDir "yuqing-restored.db"

try {
    Copy-Item -LiteralPath $BackupPath -Destination $restorePath -Force
    Push-Location $repoRoot
    try {
        $checkJson = & go run ./cmd/ops-check -db $restorePath
        if ($LASTEXITCODE -ne 0) {
            throw "restored database verification failed: $checkJson"
        }
    } finally {
        Pop-Location
    }
    $check = $checkJson | ConvertFrom-Json
    [pscustomobject]@{
        status = "ok"
        ready = ($check.status -eq "ok")
        backup = (Resolve-Path $BackupPath).Path
        restored_copy = $restorePath
        verified_at = (Get-Date).ToUniversalTime().ToString("o")
        verification = $check
    } | ConvertTo-Json -Depth 8
} finally {
    if (Test-Path $tempDir) {
        Remove-Item -LiteralPath $tempDir -Recurse -Force
    }
}
