param(
    [string]$SchedulerUrl = "http://127.0.0.1:8086",
    [string]$AuthUrl = "http://127.0.0.1:8081",
    [string]$ContentUrl = "http://127.0.0.1:8082",
    [string]$CrawlerUrl = "http://127.0.0.1:8083",
    [string]$AnalysisUrl = "http://127.0.0.1:8084",
    [string]$NlpUrl = "http://127.0.0.1:8085",
    [string]$GatewayUrl = "http://127.0.0.1"
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

Invoke-RestMethod -Method Get -Uri "$ContentUrl/api/v1/search/hot-keywords?limit=3" -TimeoutSec 5 | Out-Null
Invoke-RestMethod -Method Get -Uri "$AnalysisUrl/api/v1/public-opinion/analysis?page_size=1" -TimeoutSec 5 | Out-Null
Invoke-RestMethod -Method Post -Uri "$NlpUrl/api/v1/nlp/summarize" -Body (@{ text = "smoke test" } | ConvertTo-Json) -ContentType "application/json" -TimeoutSec 5 | Out-Null

try {
    Invoke-WebRequest -Method Get -Uri "$GatewayUrl/fullsearch/getSearchResult" -TimeoutSec 5 | Out-Null
} catch {
    Write-Host "legacy 410 probe skipped: $($_.Exception.Message)"
}

Write-Host "smoke test passed"
