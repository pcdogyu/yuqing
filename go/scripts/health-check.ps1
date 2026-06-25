param(
    [string]$GatewayUrl = "http://127.0.0.1:8079",
    [string]$AuthUrl = "http://127.0.0.1:8081",
    [string]$WechatUrl = "http://127.0.0.1:8088",
    [string]$ContentUrl = "http://127.0.0.1:8082",
    [string]$CrawlerUrl = "http://127.0.0.1:8083",
    [string]$AnalysisUrl = "http://127.0.0.1:8084",
    [string]$NlpUrl = "http://127.0.0.1:8085",
    [string]$SchedulerUrl = "http://127.0.0.1:8086",
    [string]$ReleaseUrl = "http://127.0.0.1:8099"
)

$ErrorActionPreference = "Stop"
$targets = @(
    @{ Name = "gateway-web"; Url = "$GatewayUrl/healthz" },
    @{ Name = "auth-service"; Url = "$AuthUrl/healthz" },
    @{ Name = "wechat-service"; Url = "$WechatUrl/healthz" },
    @{ Name = "content-service"; Url = "$ContentUrl/healthz" },
    @{ Name = "crawler-service"; Url = "$CrawlerUrl/healthz" },
    @{ Name = "analysis-service"; Url = "$AnalysisUrl/healthz" },
    @{ Name = "nlp-service"; Url = "$NlpUrl/healthz" },
    @{ Name = "scheduler-service"; Url = "$SchedulerUrl/healthz" },
    @{ Name = "release-service"; Url = "$ReleaseUrl/healthz" }
)

$failed = 0
foreach ($target in $targets) {
    try {
        $response = Invoke-RestMethod -Method Get -Uri $target.Url -TimeoutSec 5
        if ($response -is [string]) {
            if ($response.Trim() -eq "ok") {
                Write-Host "ok $($target.Name) $($target.Url) ok"
                continue
            }
            throw "unexpected non-json health response"
        }
        $message = ""
        if ($response.PSObject.Properties.Name -contains "message") {
            $message = [string]$response.message
        }
        $status = ""
        if ($response.PSObject.Properties.Name -contains "status") {
            $status = [string]$response.status
        }
        $code = $null
        if ($response.PSObject.Properties.Name -contains "code") {
            $code = $response.code
        }
        if ($status -ne "ok" -and $message -ne "ok" -and $code -ne 200) {
            throw "unexpected health payload"
        }
        Write-Host "ok $($target.Name) $($target.Url) $message"
    } catch {
        $failed += 1
        Write-Host "failed $($target.Name) $($target.Url): $($_.Exception.Message)"
    }
}

if ($failed -gt 0) {
    exit 1
}
