param(
    [string]$DatabasePath = $env:YUQING_DB_PATH,
    [string]$ContentUrl = "http://127.0.0.1:8082",
    [string]$NlpUrl = "http://127.0.0.1:8085",
    [switch]$SkipHttp
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
if (-not $DatabasePath) {
    $DatabasePath = Join-Path $repoRoot "data\yuqing.db"
}

Push-Location $repoRoot
try {
    $dbReportJson = & go run ./cmd/ops-check -db $DatabasePath
    if ($LASTEXITCODE -ne 0) {
        throw "database reconciliation failed: $dbReportJson"
    }
} finally {
    Pop-Location
}

$httpChecks = @()
if (-not $SkipHttp) {
    foreach ($target in @(
        @{ Name = "content_health"; Url = "$ContentUrl/healthz"; Method = "Get" },
        @{ Name = "task_runs_api"; Url = "$ContentUrl/api/v1/system/task-runs?limit=1"; Method = "Get" },
        @{ Name = "audit_logs_api"; Url = "$ContentUrl/api/v1/system/audit-logs?limit=1"; Method = "Get" },
        @{ Name = "nlp_capabilities"; Url = "$NlpUrl/api/v1/nlp/capabilities"; Method = "Get" }
    )) {
        try {
            Invoke-RestMethod -Method $target.Method -Uri $target.Url -TimeoutSec 5 | Out-Null
            $httpChecks += [pscustomobject]@{ name = $target.Name; status = "ok"; url = $target.Url }
        } catch {
            $httpChecks += [pscustomobject]@{ name = $target.Name; status = "failed"; url = $target.Url; message = $_.Exception.Message }
        }
    }
}

$failedHttp = @($httpChecks | Where-Object { $_.status -ne "ok" }).Count
[pscustomobject]@{
    status = $(if ($failedHttp -eq 0) { "ok" } else { "failed" })
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    database = ($dbReportJson | ConvertFrom-Json)
    http_checks = $httpChecks
} | ConvertTo-Json -Depth 8

if ($failedHttp -gt 0) {
    exit 1
}
