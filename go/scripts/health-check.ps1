param(
    [string]$GatewayUrl = "http://127.0.0.1",
    [string]$AuthUrl = "http://127.0.0.1:8081",
    [string]$ContentUrl = "http://127.0.0.1:8082",
    [string]$CrawlerUrl = "http://127.0.0.1:8083",
    [string]$AnalysisUrl = "http://127.0.0.1:8084",
    [string]$NlpUrl = "http://127.0.0.1:8085",
    [string]$SchedulerUrl = "http://127.0.0.1:8086"
)

$ErrorActionPreference = "Stop"
$targets = @(
    @{ Name = "gateway-web"; Url = "$GatewayUrl/healthz" },
    @{ Name = "auth-service"; Url = "$AuthUrl/healthz" },
    @{ Name = "content-service"; Url = "$ContentUrl/healthz" },
    @{ Name = "crawler-service"; Url = "$CrawlerUrl/healthz" },
    @{ Name = "analysis-service"; Url = "$AnalysisUrl/healthz" },
    @{ Name = "nlp-service"; Url = "$NlpUrl/healthz" },
    @{ Name = "scheduler-service"; Url = "$SchedulerUrl/healthz" }
)

$failed = 0
foreach ($target in $targets) {
    try {
        $response = Invoke-RestMethod -Method Get -Uri $target.Url -TimeoutSec 5
        Write-Host "ok $($target.Name) $($target.Url) $($response.message)"
    } catch {
        $failed += 1
        Write-Host "failed $($target.Name) $($target.Url): $($_.Exception.Message)"
    }
}

if ($failed -gt 0) {
    exit 1
}
