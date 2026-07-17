param(
    [string]$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path,
    [string]$BinDir = (Join-Path $Root "bin"),
    [string]$WorkingDirectory = $Root,
    [string]$LogDir = (Join-Path $Root "runtime-logs"),
    [int]$TimeoutSeconds = 30,
    [string]$AkshareArgumentFile = "",
    [Parameter(Mandatory = $true, ValueFromRemainingArguments = $true)]
    [string[]]$Names
)

$ErrorActionPreference = "Stop"

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

function Get-ServicePort([string]$ServiceName) {
    switch -Regex ($ServiceName) {
        "^auth-service$" { return 8081 }
        "^wechat-service$" { return $(if ($env:YUQING_WECHAT_ADDR -match ":(\d+)$") { [int]$Matches[1] } else { 8088 }) }
        "^content-service$" { return 8082 }
        "^crawler-service$" { return 8083 }
        "^analysis-service$" { return 8084 }
        "^nlp-service$" { return 8085 }
        "^gateway-web$" { return $(if ($env:YUQING_GATEWAY_ADDR -match ":(\d+)$") { [int]$Matches[1] } else { 8079 }) }
        "^scheduler-service$" { return $(if ($env:YUQING_SCHEDULER_ADDR -match ":(\d+)$") { [int]$Matches[1] } else { 8086 }) }
        "^release-service$" { return (Get-UrlPort $env:YUQING_RELEASE_URL 8099) }
        "^akshare-service$" { return 8087 }
        default { return 0 }
    }
}

function Test-ServiceListening([string]$ServiceName) {
    $port = Get-ServicePort $ServiceName
    if ($port -le 0) {
        return $false
    }
    return @((Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue)).Count -gt 0
}

function Get-ArgumentList([string]$ServiceName) {
    if ($ServiceName -ieq "akshare-service" -and -not [string]::IsNullOrWhiteSpace($AkshareArgumentFile)) {
        if (-not (Test-Path -LiteralPath $AkshareArgumentFile)) {
            throw "AKShare argument file not found: $AkshareArgumentFile"
        }
        return @([string[]](Get-Content -LiteralPath $AkshareArgumentFile))
    }
    return @()
}

if ($TimeoutSeconds -lt 1) {
    $TimeoutSeconds = 30
}

$Root = [System.IO.Path]::GetFullPath($Root)
$BinDir = [System.IO.Path]::GetFullPath($BinDir)
$WorkingDirectory = [System.IO.Path]::GetFullPath($WorkingDirectory)
$LogDir = [System.IO.Path]::GetFullPath($LogDir)
if (-not (Test-Path -LiteralPath $LogDir)) {
    New-Item -ItemType Directory -Path $LogDir -Force | Out-Null
}

$serviceNames = @($Names | ForEach-Object { $_ -split '\s+' } | ForEach-Object { $_.Trim() } | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
if ($serviceNames.Count -eq 0) {
    Write-Host "No services were provided for startup."
    exit 1
}

$startTargets = @()
foreach ($serviceName in $serviceNames) {
    if (Test-ServiceListening $serviceName) {
        Write-Host "$serviceName already listening; skip start."
        continue
    }
    $executable = Join-Path $BinDir "$serviceName.exe"
    if (-not (Test-Path -LiteralPath $executable)) {
        Write-Host "Executable not found for ${serviceName}: $executable"
        exit 1
    }
    $startTargets += [pscustomobject]@{
        ServiceName = $serviceName
        Executable = $executable
        OutLog = Join-Path $LogDir "$serviceName.out.log"
        ErrLog = Join-Path $LogDir "$serviceName.err.log"
        ArgumentList = @(Get-ArgumentList $serviceName)
    }
}

if ($startTargets.Count -eq 0) {
    Write-Host "All requested services are already listening."
    exit 0
}

Write-Host "Starting $($startTargets.Count) service process(es)..."
$jobs = @()
foreach ($target in $startTargets) {
    Remove-Item -LiteralPath $target.OutLog, $target.ErrLog -Force -ErrorAction SilentlyContinue
    Write-Host "Starting $($target.ServiceName)..."
    $argumentsJson = @($target.ArgumentList) | ConvertTo-Json -Compress -Depth 3
    $jobs += [pscustomobject]@{
        ServiceName = $target.ServiceName
        Job = Start-Job -ArgumentList $target.ServiceName, $target.Executable, $WorkingDirectory, $target.OutLog, $target.ErrLog, $argumentsJson -ScriptBlock {
            param(
                [string]$JobServiceName,
                [string]$JobExecutable,
                [string]$JobWorkingDirectory,
                [string]$JobOutLog,
                [string]$JobErrLog,
                [string]$JobArgumentsJson
            )

            $ErrorActionPreference = "Stop"
            function ConvertTo-ProcessArgumentString([string[]]$Arguments) {
                return @($Arguments | ForEach-Object {
                    $arg = [string]$_
                    if ($arg -match '[\s"]') {
                        '"' + $arg.Replace('"', '\"') + '"'
                    } else {
                        $arg
                    }
                }) -join " "
            }

            $jobArgumentList = @()
            if (-not [string]::IsNullOrWhiteSpace($JobArgumentsJson)) {
                $parsedArguments = $JobArgumentsJson | ConvertFrom-Json
                if ($null -ne $parsedArguments) {
                    $jobArgumentList = @($parsedArguments | ForEach-Object { [string]$_ })
                }
            }

            $startArgs = @{
                FilePath = $JobExecutable
                WorkingDirectory = $JobWorkingDirectory
                RedirectStandardOutput = $JobOutLog
                RedirectStandardError = $JobErrLog
                PassThru = $true
                WindowStyle = "Hidden"
                ErrorAction = "Stop"
            }
            if ($jobArgumentList.Count -gt 0) {
                $startArgs["ArgumentList"] = ConvertTo-ProcessArgumentString $jobArgumentList
            }
            $process = Start-Process @startArgs
            [pscustomobject]@{
                ServiceName = $JobServiceName
                ProcessId = $process.Id
            }
        }
    }
}

$deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
$pending = @($jobs)
while (($pending.Count -gt 0) -and ([DateTime]::UtcNow -lt $deadline)) {
    Start-Sleep -Milliseconds 100
    $pending = @($jobs | Where-Object { $_.Job.State -notin @("Completed", "Failed", "Stopped") })
}

$failed = $false
foreach ($entry in $jobs) {
    if ($entry.Job.State -notin @("Completed")) {
        $failed = $true
        Write-Host "Failed to start $($entry.ServiceName): job state $($entry.Job.State)"
        Stop-Job -Job $entry.Job -ErrorAction SilentlyContinue
        continue
    }

    $result = Receive-Job -Job $entry.Job -ErrorAction SilentlyContinue | Select-Object -Last 1
    if ($null -eq $result -or $null -eq $result.ProcessId) {
        $failed = $true
        Write-Host "Failed to start $($entry.ServiceName): process id was not returned."
        $entry.Job.ChildJobs | ForEach-Object {
            $_.Error | ForEach-Object { Write-Host $_.ToString() }
        }
        continue
    }

    Write-Host ("Started {0} as PID {1}" -f $result.ServiceName, $result.ProcessId)
}

$jobs | ForEach-Object { Remove-Job -Job $_.Job -Force -ErrorAction SilentlyContinue }

if ($failed) {
    exit 1
}

Write-Host "All start requests were sent."
exit 0
