param(
    [string]$DatabasePath = $env:YUQING_DB_PATH,
    [string]$BaselinePath = "",
    [string]$OutputPath = "",
    [string]$GatewayUrl = "http://127.0.0.1",
    [string]$AuthUrl = "http://127.0.0.1:8081",
    [string]$WechatUrl = "http://127.0.0.1:8087",
    [string]$ContentUrl = "http://127.0.0.1:8082",
    [string]$CrawlerUrl = "http://127.0.0.1:8083",
    [string]$AnalysisUrl = "http://127.0.0.1:8084",
    [string]$NlpUrl = "http://127.0.0.1:8085",
    [string]$SchedulerUrl = "http://127.0.0.1:8086",
    [string]$CryptoMockUrl = $env:YUQING_CRYPTO_MOCK_URL,
    [string]$ServiceToken = $(if ($env:YUQING_SERVICE_TOKEN) { $env:YUQING_SERVICE_TOKEN } else { "stonedt-internal-token" })
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
if (-not $DatabasePath) {
    $DatabasePath = Join-Path $repoRoot "data\yuqing.db"
}

$steps = @()
$artifacts = [ordered]@{}

function Add-Step($name, $status, $message = "", $durationMs = 0) {
    $script:steps += [pscustomobject]@{
        name = $name
        status = $status
        message = $message
        duration_ms = $durationMs
    }
}

function Shorten-Text($value, $max = 1200) {
    $text = (($value | Out-String) -replace "`r", "").Trim()
    if ($text.Length -le $max) {
        return $text
    }
    return $text.Substring(0, $max) + "...<truncated>"
}

function Invoke-ReleaseStep($name, [scriptblock]$body) {
    $started = Get-Date
    try {
        $output = & $body 2>&1
        $duration = [int]((Get-Date) - $started).TotalMilliseconds
        Add-Step $name "ok" (Shorten-Text $output) $duration
        return $output
    } catch {
        $duration = [int]((Get-Date) - $started).TotalMilliseconds
        $message = $_.Exception.Message
        if ($_.ErrorDetails -and $_.ErrorDetails.Message) {
            $message = $_.ErrorDetails.Message
        }
        Add-Step $name "failed" (Shorten-Text $message) $duration
        return $null
    }
}

function Invoke-ChildPowerShell($scriptPath, [string[]]$arguments = @()) {
    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $scriptPath @arguments 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw (Shorten-Text $output)
    }
    return $output
}

function Get-UrlPort([string]$url) {
    $uri = [uri]$url
    if (-not $uri.IsDefaultPort) {
        return $uri.Port
    }
    if ($uri.Scheme -eq "https") {
        return 443
    }
    return 80
}

function Test-PortInUse([int]$port) {
    return @((Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue)).Count -gt 0
}

function Find-FreePort([int]$startPort) {
    for ($port = $startPort; $port -le ($startPort + 100); $port++) {
        if (-not (Test-PortInUse $port)) {
            return $port
        }
    }
    throw "no free port found from $startPort"
}

$gatewayPort = Get-UrlPort $GatewayUrl
if ($GatewayUrl -eq "http://127.0.0.1" -and (Test-PortInUse $gatewayPort)) {
    $gatewayPort = Find-FreePort 18080
    $GatewayUrl = "http://127.0.0.1:$gatewayPort"
}
$env:YUQING_GATEWAY_ADDR = ":$gatewayPort"

$wechatPort = Get-UrlPort $WechatUrl
if ($WechatUrl -eq "http://127.0.0.1:8087" -and (Test-PortInUse $wechatPort)) {
    $wechatPort = Find-FreePort 18087
    $WechatUrl = "http://127.0.0.1:$wechatPort"
}
$env:YUQING_WECHAT_ADDR = ":$wechatPort"
$env:YUQING_WECHAT_URL = $WechatUrl

$schedulerPort = Get-UrlPort $SchedulerUrl
if ($SchedulerUrl -eq "http://127.0.0.1:8086" -and (Test-PortInUse $schedulerPort)) {
    $schedulerPort = Find-FreePort 18086
    $SchedulerUrl = "http://127.0.0.1:$schedulerPort"
}
$env:YUQING_SCHEDULER_ADDR = ":$schedulerPort"
$env:YUQING_SCHEDULER_URL = $SchedulerUrl

Push-Location $repoRoot
try {
    Add-Step "gateway_endpoint" "ok" "gateway $GatewayUrl"
    Add-Step "wechat_endpoint" "ok" "wechat api $WechatUrl"
    Add-Step "scheduler_endpoint" "ok" "scheduler api $SchedulerUrl"

    Invoke-ReleaseStep "go_test" {
        & go test ./...
        if ($LASTEXITCODE -ne 0) {
            throw "go test ./... failed"
        }
    } | Out-Null

    Invoke-ReleaseStep "start_all" {
        Invoke-ChildPowerShell (Join-Path $PSScriptRoot "start-all.ps1")
    } | Out-Null

    Invoke-ReleaseStep "health_check" {
        $lastError = ""
        for ($attempt = 1; $attempt -le 12; $attempt++) {
            try {
                Invoke-ChildPowerShell (Join-Path $PSScriptRoot "health-check.ps1") @(
                    "-GatewayUrl", $GatewayUrl,
                    "-AuthUrl", $AuthUrl,
                    "-WechatUrl", $WechatUrl,
                    "-ContentUrl", $ContentUrl,
                    "-CrawlerUrl", $CrawlerUrl,
                    "-AnalysisUrl", $AnalysisUrl,
                    "-NlpUrl", $NlpUrl,
                    "-SchedulerUrl", $SchedulerUrl
                )
                return
            } catch {
                $lastError = $_.Exception.Message
                Start-Sleep -Seconds 2
            }
        }
        throw "health-check did not pass after retries: $lastError"
    } | Out-Null

    Invoke-ReleaseStep "smoke_test" {
        Invoke-ChildPowerShell (Join-Path $PSScriptRoot "smoke-test.ps1") @(
            "-GatewayUrl", $GatewayUrl,
            "-AuthUrl", $AuthUrl,
            "-WechatUrl", $WechatUrl,
            "-ContentUrl", $ContentUrl,
            "-CrawlerUrl", $CrawlerUrl,
            "-AnalysisUrl", $AnalysisUrl,
            "-NlpUrl", $NlpUrl,
            "-SchedulerUrl", $SchedulerUrl,
            "-ServiceToken", $ServiceToken,
            "-DatabasePath", $DatabasePath,
            "-CryptoMockUrl", $CryptoMockUrl
        )
    } | Out-Null

    $reconcileOutput = Invoke-ReleaseStep "reconcile" {
        $reconcileArgs = @(
            "-DatabasePath", $DatabasePath,
            "-ContentUrl", $ContentUrl,
            "-NlpUrl", $NlpUrl
        )
        if ($BaselinePath) {
            $reconcileArgs += @("-BaselinePath", $BaselinePath)
        }
        Invoke-ChildPowerShell (Join-Path $PSScriptRoot "reconcile-production.ps1") $reconcileArgs
    }
    if ($reconcileOutput) {
        $artifacts.reconcile = ($reconcileOutput | ConvertFrom-Json)
        if ($artifacts.reconcile.status -ne "ok") {
            Add-Step "reconcile_status" "failed" $artifacts.reconcile.status
        }
    }

    $backupOutput = Invoke-ReleaseStep "backup" {
        Invoke-ChildPowerShell (Join-Path $PSScriptRoot "backup-sqlite.ps1") @("-DatabasePath", $DatabasePath)
    }
    if ($backupOutput) {
        $artifacts.backup = ($backupOutput | ConvertFrom-Json)
        if ($artifacts.backup.status -ne "ok") {
            Add-Step "backup_status" "failed" $artifacts.backup.status
        }
    }

    if ($artifacts.backup -and $artifacts.backup.backup) {
        $restoreOutput = Invoke-ReleaseStep "restore" {
            Invoke-ChildPowerShell (Join-Path $PSScriptRoot "restore-sqlite.ps1") @(
                "-BackupPath", $artifacts.backup.backup,
                "-SourceDatabasePath", $DatabasePath
            )
        }
        if ($restoreOutput) {
            $artifacts.restore = ($restoreOutput | ConvertFrom-Json)
            if (-not $artifacts.restore.ready) {
                Add-Step "restore_ready" "failed" "restore ready=false"
            }
        }
    } else {
        Add-Step "restore" "failed" "backup artifact missing"
    }
} finally {
    Invoke-ReleaseStep "stop_all" {
        Invoke-ChildPowerShell (Join-Path $PSScriptRoot "stop-all.ps1")
    } | Out-Null
    Pop-Location
}

$failed = @($steps | Where-Object { $_.status -ne "ok" })
$report = [pscustomobject]@{
    ready = ($failed.Count -eq 0)
    status = $(if ($failed.Count -eq 0) { "ok" } else { "failed" })
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    database = $DatabasePath
    baseline = $(if ($BaselinePath) { $BaselinePath } else { $null })
    steps = $steps
    artifacts = $artifacts
}

$json = $report | ConvertTo-Json -Depth 12
if ($OutputPath) {
    $resolvedOutput = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($OutputPath)
    $outputDir = Split-Path -Parent $resolvedOutput
    if ($outputDir) {
        New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
    }
    Set-Content -LiteralPath $resolvedOutput -Value $json -Encoding UTF8
}
$json

if ($failed.Count -gt 0) {
    exit 1
}
