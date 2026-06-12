param(
    [string]$SchedulerUrl = "http://127.0.0.1:8086",
    [string]$AuthUrl = "http://127.0.0.1:8081",
    [string]$ContentUrl = "http://127.0.0.1:8082",
    [string]$CrawlerUrl = "http://127.0.0.1:8083",
    [string]$AnalysisUrl = "http://127.0.0.1:8084",
    [string]$NlpUrl = "http://127.0.0.1:8085",
    [string]$GatewayUrl = "http://127.0.0.1",
    [string]$ServiceToken = $(if ($env:YUQING_SERVICE_TOKEN) { $env:YUQING_SERVICE_TOKEN } else { "stonedt-internal-token" })
)

$ErrorActionPreference = "Stop"

& (Join-Path $PSScriptRoot "health-check.ps1") `
    -GatewayUrl $GatewayUrl `
    -AuthUrl $AuthUrl `
    -ContentUrl $ContentUrl `
    -CrawlerUrl $CrawlerUrl `
    -AnalysisUrl $AnalysisUrl `
    -NlpUrl $NlpUrl `
    -SchedulerUrl $SchedulerUrl

$jobs = Invoke-RestMethod -Method Get -Uri "$SchedulerUrl/api/v1/scheduler/jobs" -TimeoutSec 5
if (-not ($jobs.data | Where-Object { $_.name -eq "analysis-refresh" })) {
    throw "scheduler jobs endpoint did not return analysis-refresh"
}
if (-not ($jobs.data | Where-Object { $_.name -eq "analysis-refresh" -and $_.java_quartz_name -and $_.next_run_at })) {
    throw "scheduler jobs endpoint did not return runtime metadata"
}

Invoke-RestMethod -Method Get -Uri "$ContentUrl/api/v1/search/hot-keywords?limit=3" -TimeoutSec 5 | Out-Null
Invoke-RestMethod -Method Get -Uri "$AnalysisUrl/api/v1/public-opinion/analysis?page_size=1" -TimeoutSec 5 | Out-Null
Invoke-RestMethod -Method Post -Uri "$NlpUrl/api/v1/nlp/summarize" -Body (@{ text = "smoke test" } | ConvertTo-Json) -ContentType "application/json" -TimeoutSec 5 | Out-Null
Invoke-RestMethod -Method Get -Uri "$NlpUrl/api/v1/nlp/capabilities" -TimeoutSec 5 | Out-Null
$operations = Invoke-RestMethod -Method Get -Uri "$ContentUrl/api/v1/system/operations" -TimeoutSec 8
if (-not $operations.data -or -not $operations.data.legacy_registry) {
    throw "operations API did not return production summary"
}
$alerts = Invoke-RestMethod -Method Get -Uri "$ContentUrl/api/v1/system/alerts" -TimeoutSec 8
if ($null -eq $alerts.data.ready) {
    throw "alerts API did not return readiness flag"
}

Invoke-RestMethod -Method Post -Uri "$ContentUrl/api/v1/system/audit-logs" `
    -Headers @{ "X-Service-Token" = $ServiceToken } `
    -Body (@{ user_id = 0; username = "smoke"; action = "smoke.audit"; resource = "/scripts/smoke-test"; detail_json = '{"status":"ok"}' } | ConvertTo-Json) `
    -ContentType "application/json" `
    -TimeoutSec 5 | Out-Null

$audit = Invoke-RestMethod -Method Get -Uri "$ContentUrl/api/v1/system/audit-logs?action=smoke.audit&limit=1" -TimeoutSec 5
if (-not $audit.data) {
    throw "audit log write probe did not return smoke.audit"
}

$legacyProbes = @(
    "/fullsearch/getSearchResult",
    "/timelysearch/result",
    "/platform/nlp/ocr",
    "/platform/xie/report",
    "/mobile/monitor",
    "/displayboard",
    "/volume",
    "/hot/hotpage",
    "/dist/monitor",
    "/img/code"
)
foreach ($path in $legacyProbes) {
    try {
        $legacy = Invoke-WebRequest -Method Get -Uri "$GatewayUrl$path" -TimeoutSec 5 -SkipHttpErrorCheck
        if ($legacy.StatusCode -ne 410) {
            throw "expected 410, got $($legacy.StatusCode)"
        }
    } catch {
        throw "legacy 410 probe failed for ${path}: $($_.Exception.Message)"
    }
}

$backup = & (Join-Path $PSScriptRoot "backup-sqlite.ps1") | ConvertFrom-Json
if ($backup.status -ne "ok" -or -not $backup.backup) {
    throw "backup verification failed during smoke test"
}
$restore = & (Join-Path $PSScriptRoot "restore-sqlite.ps1") -BackupPath $backup.backup | ConvertFrom-Json
if (-not $restore.ready) {
    throw "restore verification failed during smoke test"
}

Write-Host "smoke test passed"
