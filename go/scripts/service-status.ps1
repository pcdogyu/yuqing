param(
    [string]$LogDir = ""
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($LogDir)) {
    $LogDir = Join-Path (Split-Path -Parent $PSScriptRoot) "runtime-logs"
}

$services = @(
    [pscustomobject]@{ Name = "auth-service"; Port = 8081; Url = "http://127.0.0.1:8081" },
    [pscustomobject]@{ Name = "wechat-service"; Port = 8088; Url = "http://127.0.0.1:8088" },
    [pscustomobject]@{ Name = "content-service"; Port = 8082; Url = "http://127.0.0.1:8082" },
    [pscustomobject]@{ Name = "crawler-service"; Port = 8083; Url = "http://127.0.0.1:8083" },
    [pscustomobject]@{ Name = "analysis-service"; Port = 8084; Url = "http://127.0.0.1:8084" },
    [pscustomobject]@{ Name = "nlp-service"; Port = 8085; Url = "http://127.0.0.1:8085" },
    [pscustomobject]@{ Name = "gateway-web"; Port = 8079; Url = "http://127.0.0.1:8079" },
    [pscustomobject]@{ Name = "scheduler-service"; Port = 8086; Url = "http://127.0.0.1:8086" },
    [pscustomobject]@{ Name = "akshare-service"; Port = 8087; Url = "http://127.0.0.1:8087" }
)

$listeners = @(Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue)
$rows = foreach ($service in $services) {
    $matches = @($listeners | Where-Object { $_.LocalPort -eq $service.Port })
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
        Port = $service.Port
        Listening = if ($matches.Count -gt 0) { "YES" } else { "NO" }
        Address = if ($addresses.Count -gt 0) { $addresses -join "," } else { "--" }
        PID = if ($pids.Count -gt 0) { $pids -join "," } else { "--" }
        Process = if ($processes.Count -gt 0) { (@($processes | ForEach-Object { $_.ProcessName } | Select-Object -Unique) -join ",") } else { "--" }
        ErrBytes = $errBytes
        ErrUpdated = $errTime
        Url = $service.Url
    }
}

$rows | Format-Table -AutoSize
