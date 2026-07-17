param(
    [string]$LogDir = "",
    [int]$TimeoutSeconds = 30,
    [switch]$SkipAkshare
)

$ErrorActionPreference = "Stop"

if ($TimeoutSeconds -lt 1) {
    $TimeoutSeconds = 30
}

if ([string]::IsNullOrWhiteSpace($LogDir)) {
    $LogDir = Join-Path (Split-Path -Parent $PSScriptRoot) "runtime-logs"
}

function Get-UrlPort([string]$Url, [int]$DefaultPort) {
    if ([string]::IsNullOrWhiteSpace($Url)) {
        return $DefaultPort
    }
    try {
        $uri = [uri]$Url
        if (-not $uri.IsDefaultPort) {
            return $uri.Port
        }
        if ($uri.Scheme -eq "https") {
            return 443
        }
        return 80
    } catch {
        return $DefaultPort
    }
}

$releaseUrl = if ([string]::IsNullOrWhiteSpace($env:YUQING_RELEASE_URL)) {
    "http://127.0.0.1:8099"
} else {
    $env:YUQING_RELEASE_URL
}
$releasePort = Get-UrlPort $releaseUrl 8099

$services = @(
    [pscustomobject]@{ Name = "auth-service"; Port = 8081 },
    [pscustomobject]@{ Name = "wechat-service"; Port = 8088 },
    [pscustomobject]@{ Name = "content-service"; Port = 8082 },
    [pscustomobject]@{ Name = "crawler-service"; Port = 8083 },
    [pscustomobject]@{ Name = "analysis-service"; Port = 8084 },
    [pscustomobject]@{ Name = "nlp-service"; Port = 8085 },
    [pscustomobject]@{ Name = "gateway-web"; Port = 8079 },
    [pscustomobject]@{ Name = "gateway-web"; Port = 80 },
    [pscustomobject]@{ Name = "scheduler-service"; Port = 8086 },
    [pscustomobject]@{ Name = "release-service"; Port = $releasePort },
    [pscustomobject]@{ Name = "akshare-service"; Port = 8087 }
)
if ($SkipAkshare) {
    $services = @($services | Where-Object { $_.Name -ne "akshare-service" })
}

$total = $services.Count
function Get-MissingServices {
    $listeners = @(Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue)
    $missing = @()
    foreach ($service in $services) {
        $matches = @($listeners | Where-Object { $_.LocalPort -eq $service.Port })
        if ($matches.Count -eq 0) {
            $missing += "$($service.Name):$($service.Port)"
        }
    }
    return @($missing)
}

for ($attempt = 1; $attempt -le $TimeoutSeconds; $attempt++) {
    $missing = @(Get-MissingServices)

    $listening = $total - $missing.Count
    if ($missing.Count -eq 0) {
        Write-Host "[services] progress $listening/$total listening."
        exit 0
    }

    Write-Host "[services] progress $listening/$total listening; waiting: $($missing -join ', ')"
    if ($attempt -lt $TimeoutSeconds) {
        Start-Sleep -Seconds 1
    }
}

$missing = @(Get-MissingServices)
if ($missing.Count -eq 0) {
    Write-Host "[services] progress $total/$total listening."
    exit 0
}

exit 1
