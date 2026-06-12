param(
    [string]$BackupPath = "",
    [string]$SourceDatabasePath = $env:YUQING_DB_PATH,
    [string]$BackupDir = (Join-Path (Resolve-Path (Join-Path $PSScriptRoot "..")).Path "backups")
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
if (-not $SourceDatabasePath) {
    $SourceDatabasePath = Join-Path $repoRoot "data\yuqing.db"
}

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
        $sourceCheckJson = $null
        if (Test-Path $SourceDatabasePath) {
            $sourceCheckJson = & go run ./cmd/ops-check -db $SourceDatabasePath
            if ($LASTEXITCODE -ne 0) {
                throw "source database verification failed: $sourceCheckJson"
            }
        }
    } finally {
        Pop-Location
    }
    $check = $checkJson | ConvertFrom-Json
    $sourceCheck = $null
    $tableCountsMatch = $null
    $tableCountDiff = @()
    if ($sourceCheckJson) {
        $sourceCheck = $sourceCheckJson | ConvertFrom-Json
        $sourceCounts = @{}
        foreach ($item in @($sourceCheck.checks | Where-Object { $null -ne $_.count })) {
            $sourceCounts[$item.name] = [int64]$item.count
        }
        foreach ($item in @($check.checks | Where-Object { $null -ne $_.count })) {
            if ($sourceCounts.ContainsKey($item.name) -and [int64]$item.count -ne [int64]$sourceCounts[$item.name]) {
                $tableCountDiff += [pscustomobject]@{
                    name = $item.name
                    source = [int64]$sourceCounts[$item.name]
                    restored = [int64]$item.count
                }
            }
        }
        $tableCountsMatch = ($tableCountDiff.Count -eq 0)
    }
    [pscustomobject]@{
        status = "ok"
        ready = ($check.status -eq "ok" -and ($null -eq $tableCountsMatch -or $tableCountsMatch))
        backup = (Resolve-Path $BackupPath).Path
        source = $(if (Test-Path $SourceDatabasePath) { (Resolve-Path $SourceDatabasePath).Path } else { $null })
        restored_copy = $restorePath
        verified_at = (Get-Date).ToUniversalTime().ToString("o")
        table_counts_match = $tableCountsMatch
        table_count_diff = $tableCountDiff
        verification = $check
        source_verification = $sourceCheck
    } | ConvertTo-Json -Depth 8
} finally {
    if (Test-Path $tempDir) {
        Remove-Item -LiteralPath $tempDir -Recurse -Force
    }
}
