param(
    [string]$SchedulerUrl = "http://127.0.0.1:8086",
    [string]$AuthUrl = "http://127.0.0.1:8081",
    [string]$WechatUrl = "http://127.0.0.1:8088",
    [string]$ContentUrl = "http://127.0.0.1:8082",
    [string]$CrawlerUrl = "http://127.0.0.1:8083",
    [string]$AnalysisUrl = "http://127.0.0.1:8084",
    [string]$NlpUrl = "http://127.0.0.1:8085",
    [string]$GatewayUrl = "http://127.0.0.1:8080",
    [string]$DatabasePath = $env:YUQING_DB_PATH,
    [string]$CryptoMockUrl = $env:YUQING_CRYPTO_MOCK_URL,
    [string]$ServiceToken = $(if ($env:YUQING_SERVICE_TOKEN) { $env:YUQING_SERVICE_TOKEN } else { "stonedt-internal-token" })
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
if (-not $DatabasePath) {
    $DatabasePath = Join-Path $repoRoot "data\yuqing.db"
}

function Invoke-WebRequestAllowError([string]$Method, [string]$Uri, [int]$TimeoutSec = 5) {
    try {
        return Invoke-WebRequest -Method $Method -Uri $Uri -TimeoutSec $TimeoutSec
    } catch {
        if ($_.Exception.Response) {
            return $_.Exception.Response
        }
        throw
    }
}

& (Join-Path $PSScriptRoot "health-check.ps1") `
    -GatewayUrl $GatewayUrl `
    -AuthUrl $AuthUrl `
    -WechatUrl $WechatUrl `
    -ContentUrl $ContentUrl `
    -CrawlerUrl $CrawlerUrl `
    -AnalysisUrl $AnalysisUrl `
    -NlpUrl $NlpUrl `
    -SchedulerUrl $SchedulerUrl

$jobs = $null
for ($attempt = 1; $attempt -le 12; $attempt++) {
    try {
        $jobs = Invoke-RestMethod -Method Get -Uri "$SchedulerUrl/api/v1/scheduler/jobs" -TimeoutSec 5
        if ($jobs.data | Where-Object { $_.name -eq "analysis-refresh" -and $_.java_quartz_name -and $_.next_run_at }) {
            break
        }
    } catch {
        if ($attempt -eq 12) {
            throw
        }
    }
    Start-Sleep -Seconds 2
}
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
if (-not $operations.data.scheduler_jobs -or -not $operations.data.task_summary -or -not $operations.data.audit_summary) {
    throw "operations API did not return scheduler/task/audit summaries"
}
if (-not $operations.data.legacy_route_probes -or -not $operations.data.external_integrations -or -not $operations.data.backup) {
    throw "operations API did not return legacy/external/backup summaries"
}
$alerts = Invoke-RestMethod -Method Get -Uri "$ContentUrl/api/v1/system/alerts" -TimeoutSec 8
if ($null -eq $alerts.data.ready) {
    throw "alerts API did not return readiness flag"
}
if ($null -eq $alerts.data.alerts) {
    throw "alerts API did not return alerts list"
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
        $legacy = Invoke-WebRequestAllowError "Get" "$GatewayUrl$path" 5
        $statusCode = [int]$legacy.StatusCode
        if ($statusCode -ne 410) {
            throw "expected 410, got $statusCode"
        }
    } catch {
        throw "legacy 410 probe failed for ${path}: $($_.Exception.Message)"
    }
}

if ($CryptoMockUrl) {
    $mockBase = $CryptoMockUrl.TrimEnd("/")
    foreach ($probe in @(
        @{ Path = "/mock/non-200"; Expected = 502; Name = "external_non_200" },
        @{ Path = "/mock/bad-json"; Expected = 200; Name = "external_invalid_json" },
        @{ Path = "/mock/empty"; Expected = 200; Name = "external_empty_data" },
        @{ Path = "/mock/duplicate"; Expected = 200; Name = "external_duplicate_data" }
    )) {
        $mock = Invoke-WebRequestAllowError "Get" "$mockBase$($probe.Path)" 5
        $statusCode = [int]$mock.StatusCode
        if ($statusCode -ne $probe.Expected) {
            throw "crypto mock $($probe.Name) expected $($probe.Expected), got $statusCode"
        }
        if ($probe.Name -eq "external_invalid_json") {
            try {
                $mock.Content | ConvertFrom-Json | Out-Null
                throw "crypto mock external_invalid_json returned valid json"
            } catch {
                if ($_.Exception.Message -eq "crypto mock external_invalid_json returned valid json") {
                    throw
                }
            }
        }
    }
}

$backup = & (Join-Path $PSScriptRoot "backup-sqlite.ps1") -DatabasePath $DatabasePath | ConvertFrom-Json
if ($backup.status -ne "ok" -or -not $backup.backup) {
    throw "backup verification failed during smoke test"
}
$restore = & (Join-Path $PSScriptRoot "restore-sqlite.ps1") -BackupPath $backup.backup -SourceDatabasePath $DatabasePath | ConvertFrom-Json
if (-not $restore.ready) {
    throw "restore verification failed during smoke test"
}

Write-Host "smoke test passed"
