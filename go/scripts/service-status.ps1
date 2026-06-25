param(
    [string]$LogDir = ""
)

$ErrorActionPreference = "Stop"

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
    [pscustomobject]@{ Name = "auth-service"; Port = 8081; Url = "http://127.0.0.1:8081" },
    [pscustomobject]@{ Name = "wechat-service"; Port = 8088; Url = "http://127.0.0.1:8088" },
    [pscustomobject]@{ Name = "content-service"; Port = 8082; Url = "http://127.0.0.1:8082" },
    [pscustomobject]@{ Name = "crawler-service"; Port = 8083; Url = "http://127.0.0.1:8083" },
    [pscustomobject]@{ Name = "analysis-service"; Port = 8084; Url = "http://127.0.0.1:8084" },
    [pscustomobject]@{ Name = "nlp-service"; Port = 8085; Url = "http://127.0.0.1:8085" },
    [pscustomobject]@{ Name = "gateway-web"; Port = 8079; Url = "http://127.0.0.1:8079" },
    [pscustomobject]@{ Name = "gateway-web"; Port = 80; Url = "http://127.0.0.1/" },
    [pscustomobject]@{ Name = "scheduler-service"; Port = 8086; Url = "http://127.0.0.1:8086" },
    [pscustomobject]@{ Name = "release-service"; Port = $releasePort; Url = $releaseUrl },
    [pscustomobject]@{ Name = "akshare-service"; Port = 8087; Url = "http://127.0.0.1:8087" }
)

$listeners = @(Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue)
$rows = foreach ($service in $services) {
    $port = $service.Port
    $url = $service.Url
    $matches = @($listeners | Where-Object { $_.LocalPort -eq $port })
    $pids = @($matches | ForEach-Object { $_.OwningProcess } | Where-Object { $_ } | Select-Object -Unique)
    $processes = @()
    foreach ($processId in $pids) {
        $proc = Get-Process -Id $processId -ErrorAction SilentlyContinue
        if ($null -ne $proc) {
            $processes += $proc
        }
    }
    if ($processes.Count -eq 0) {
        $processes = @(Get-Process -Name $service.Name -ErrorAction SilentlyContinue)
        $pids = @($processes | ForEach-Object { $_.Id } | Select-Object -Unique)
    }

    if ($service.Name -eq "release-service" -and $processes.Count -gt 0) {
        $ownedMatches = @($listeners | Where-Object { $pids -contains $_.OwningProcess })
        if ($ownedMatches.Count -gt 0) {
            $ownedPorts = @($ownedMatches | ForEach-Object { $_.LocalPort } | Select-Object -Unique | Sort-Object)
            $port = [int]$ownedPorts[0]
            $matches = @($ownedMatches | Where-Object { $_.LocalPort -eq $port })
            if ([string]::IsNullOrWhiteSpace($env:YUQING_RELEASE_URL) -or $url -eq "http://127.0.0.1:8099") {
                $url = "http://127.0.0.1:$port"
            }
        }
    }

    $addresses = @($matches | ForEach-Object { $_.LocalAddress } | Select-Object -Unique)
    $logPath = Join-Path $LogDir "$($service.Name).err.log"
    $errBytes = "--"
    $errTime = "--"
    if (Test-Path -LiteralPath $logPath) {
        $log = Get-Item -LiteralPath $logPath
        $errBytes = [string]$log.Length
        $errTime = $log.LastWriteTime.ToString("yyyy-MM-dd HH:mm:ss")
    }

    [pscustomobject]@{
        Service = $service.Name
        Port = $port
        Listening = if ($matches.Count -gt 0) { "YES" } else { "NO" }
        Address = if ($addresses.Count -gt 0) { $addresses -join "," } else { "--" }
        PID = if ($pids.Count -gt 0) { $pids -join "," } else { "--" }
        Process = if ($processes.Count -gt 0) { (@($processes | ForEach-Object { $_.ProcessName } | Select-Object -Unique) -join ",") } else { "--" }
        ErrBytes = $errBytes
        ErrUpdated = $errTime
        Url = $url
    }
}

$rows | Sort-Object Port | Format-Table -AutoSize
