param(
    [string]$DatabasePath = $env:YUQING_DB_PATH,
    [string]$BaselinePath = ""
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
if (-not $DatabasePath) {
    $DatabasePath = Join-Path $repoRoot "data\yuqing.db"
}

$steps = @()
function Add-Step($name, $status, $message = "") {
    $script:steps += [pscustomobject]@{ name = $name; status = $status; message = $message }
}

Push-Location $repoRoot
try {
    try {
        & go test ./...
        if ($LASTEXITCODE -ne 0) { throw "go test failed" }
        Add-Step "go_test" "ok"

        & (Join-Path $PSScriptRoot "start-all.ps1") | Out-Null
        Add-Step "start_all" "ok"

        & (Join-Path $PSScriptRoot "health-check.ps1") | Out-Null
        Add-Step "health_check" "ok"

        & (Join-Path $PSScriptRoot "smoke-test.ps1") | Out-Null
        Add-Step "smoke_test" "ok"

        $reconcileArgs = @{ DatabasePath = $DatabasePath }
        if ($BaselinePath) { $reconcileArgs["BaselinePath"] = $BaselinePath }
        $reconcile = & (Join-Path $PSScriptRoot "reconcile-production.ps1") @reconcileArgs | ConvertFrom-Json
        Add-Step "reconcile" $reconcile.status

        $backup = & (Join-Path $PSScriptRoot "backup-sqlite.ps1") -DatabasePath $DatabasePath | ConvertFrom-Json
        Add-Step "backup" $backup.status

        $restore = & (Join-Path $PSScriptRoot "restore-sqlite.ps1") -BackupPath $backup.backup -SourceDatabasePath $DatabasePath | ConvertFrom-Json
        Add-Step "restore" $restore.status
    } catch {
        Add-Step "release_check" "failed" $_.Exception.Message
    } finally {
        try {
            & (Join-Path $PSScriptRoot "stop-all.ps1") | Out-Null
            Add-Step "stop_all" "ok"
        } catch {
            Add-Step "stop_all" "failed" $_.Exception.Message
        }
    }
} finally {
    Pop-Location
}

$failed = @($steps | Where-Object { $_.status -ne "ok" })
[pscustomobject]@{
    ready = ($failed.Count -eq 0)
    status = $(if ($failed.Count -eq 0) { "ok" } else { "failed" })
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    steps = $steps
} | ConvertTo-Json -Depth 8

if ($failed.Count -gt 0) {
    exit 1
}
