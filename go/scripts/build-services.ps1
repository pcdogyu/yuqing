param(
    [string]$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path,
    [string]$BinDir = (Join-Path $Root "bin"),
    [string]$Ldflags = "",
    [string]$GoBuildFlags = "",
    [string]$GoCacheDir = "",
    [Parameter(Mandatory = $true, ValueFromRemainingArguments = $true)]
    [string[]]$Services
)

$ErrorActionPreference = "Stop"

function Split-ArgumentString([string]$Text) {
    if ([string]::IsNullOrWhiteSpace($Text)) {
        return @()
    }

    $parseErrors = $null
    $tokens = [System.Management.Automation.PSParser]::Tokenize($Text, [ref]$parseErrors)
    if ($parseErrors -and $parseErrors.Count -gt 0) {
        throw "Failed to parse Go build flags: $($parseErrors[0].Message)"
    }

    $args = @()
    foreach ($token in $tokens) {
        if ($token.Type -in @("Command", "CommandArgument", "CommandParameter", "String", "Parameter", "Number")) {
            $args += $token.Content
        }
    }
    return @($args)
}

$Root = [System.IO.Path]::GetFullPath($Root)
$BinDir = [System.IO.Path]::GetFullPath($BinDir)
if (-not (Test-Path $BinDir)) {
    New-Item -ItemType Directory -Path $BinDir | Out-Null
}
if (-not [string]::IsNullOrWhiteSpace($GoCacheDir)) {
    $GoCacheDir = [System.IO.Path]::GetFullPath($GoCacheDir)
    if (-not (Test-Path $GoCacheDir)) {
        New-Item -ItemType Directory -Path $GoCacheDir | Out-Null
    }
    Write-Host "[build] GOCACHE $GoCacheDir"
}

$serviceNames = @($Services | ForEach-Object { $_ -split '\s+' } | ForEach-Object { $_.Trim() } | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
if ($serviceNames.Count -eq 0) {
    Write-Host "No services were provided for build."
    exit 1
}

$buildFlags = @(Split-ArgumentString $GoBuildFlags)
$total = $serviceNames.Count
$jobs = @()
$queued = 0

foreach ($service in $serviceNames) {
    $queued++
    Write-Host "[build] queued $queued/$total $service"
    $job = Start-Job -Name "build-$service" -ArgumentList $Root, $BinDir, $service, $Ldflags, $buildFlags, $GoCacheDir -ScriptBlock {
        param(
            [string]$JobRoot,
            [string]$JobBinDir,
            [string]$JobService,
            [string]$JobLdflags,
            [string[]]$JobBuildFlags,
            [string]$JobGoCacheDir
        )

        Set-Location $JobRoot
        if (-not [string]::IsNullOrWhiteSpace($JobGoCacheDir)) {
            $env:GOCACHE = $JobGoCacheDir
        }
        $outputPath = Join-Path $JobBinDir "$JobService.exe"
        $args = @("build")
        if ($JobBuildFlags -and $JobBuildFlags.Count -gt 0) {
            $args += $JobBuildFlags
        }
        if (-not [string]::IsNullOrWhiteSpace($JobLdflags)) {
            $args += @("-ldflags", $JobLdflags)
        }
        $args += @("-o", $outputPath, "./cmd/$JobService")

        $started = Get-Date
        $output = @(& go @args 2>&1 | ForEach-Object { $_.ToString() })
        $exitCode = if ($null -eq $LASTEXITCODE) { 0 } else { $LASTEXITCODE }
        $duration = [Math]::Round(((Get-Date) - $started).TotalSeconds, 1)
        [pscustomobject]@{
            Service = $JobService
            ExitCode = $exitCode
            DurationSeconds = $duration
            Output = $output
        }
    }
    $jobs += [pscustomobject]@{
        Service = $service
        Job = $job
        Reported = $false
    }
}

$completed = 0
$failed = $false
$lastProgressAt = [DateTime]::MinValue

while ($completed -lt $total) {
    foreach ($entry in $jobs) {
        if ($entry.Reported) {
            continue
        }
        if ($entry.Job.State -notin @("Completed", "Failed", "Stopped")) {
            continue
        }

        $entry.Reported = $true
        $completed++
        $result = $null
        $received = @(Receive-Job -Job $entry.Job -ErrorAction SilentlyContinue)
        if ($received.Count -gt 0) {
            $result = $received | Where-Object { $_.PSObject.Properties["Service"] -and $_.Service -eq $entry.Service } | Select-Object -Last 1
        }

        if ($entry.Job.State -ne "Completed" -or $null -eq $result -or [int]$result.ExitCode -ne 0) {
            $failed = $true
            $duration = if ($null -eq $result) { "unknown" } else { "$($result.DurationSeconds)s" }
            Write-Host "[build] FAILED $completed/$total $($entry.Service) ($duration)"
            if ($null -ne $result -and $result.Output) {
                $result.Output | ForEach-Object { Write-Host $_ }
            } else {
                $entry.Job.ChildJobs | ForEach-Object {
                    if ($_.Error.Count -gt 0) {
                        $_.Error | ForEach-Object { Write-Host $_.ToString() }
                    }
                }
            }
        } else {
            Write-Host "[build] completed $completed/$total $($entry.Service) ($($result.DurationSeconds)s)"
            if ($result.Output) {
                $result.Output | ForEach-Object { Write-Host $_ }
            }
        }
    }

    if ($completed -lt $total) {
        $now = [DateTime]::UtcNow
        if (($now - $lastProgressAt).TotalSeconds -ge 2) {
            $running = @($jobs | Where-Object { -not $_.Reported } | ForEach-Object { $_.Service })
            Write-Host "[build] progress $completed/$total completed; running: $($running -join ', ')"
            $lastProgressAt = $now
        }
        Start-Sleep -Milliseconds 300
    }
}

$jobs | ForEach-Object { Remove-Job -Job $_.Job -Force -ErrorAction SilentlyContinue }

if ($failed) {
    exit 1
}

Write-Host "[build] all service binaries built."
exit 0
