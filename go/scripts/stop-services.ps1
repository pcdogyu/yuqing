param(
    [string]$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path,
    [int]$WaitMilliseconds = 5000,
    [switch]$AutoElevate,
    [switch]$NoAutoElevate,
    [Parameter(Mandatory = $true, ValueFromRemainingArguments = $true)]
    [string[]]$Names
)

$ErrorActionPreference = "Stop"

try {
    $Root = [System.IO.Path]::GetFullPath($Root)
} catch {
    # Keep the original value for diagnostics if normalization fails.
}

$taskkill = Join-Path $env:SystemRoot "System32\taskkill.exe"
if (-not (Test-Path $taskkill)) {
    $taskkill = "taskkill.exe"
}

function Test-CurrentProcessElevated {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [Security.Principal.WindowsPrincipal]::new($identity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Quote-ProcessArgument([string]$value) {
    if ($null -eq $value) {
        return '""'
    }
    return '"' + $value.Replace('"', '\"') + '"'
}

function Invoke-ElevatedStopServices([string]$root, [int]$waitMilliseconds, [string[]]$serviceNames) {
    if ([string]::IsNullOrWhiteSpace($PSCommandPath)) {
        return 1
    }
    $args = @(
        "-NoProfile",
        "-ExecutionPolicy", "Bypass",
        "-File", (Quote-ProcessArgument $PSCommandPath),
        "-Root", (Quote-ProcessArgument $root),
        "-WaitMilliseconds", $waitMilliseconds,
        "-NoAutoElevate",
        "-Names", (Quote-ProcessArgument ($serviceNames -join " "))
    )
    try {
        Write-Host "Retrying service stop with administrator privileges. Accept the UAC prompt if it appears."
        $process = Start-Process -FilePath "powershell.exe" -ArgumentList $args -Verb RunAs -Wait -PassThru -ErrorAction Stop
        if ($null -eq $process) {
            return 1
        }
        return $process.ExitCode
    } catch {
        Write-Host "Administrator stop retry was not started: $($_.Exception.Message)"
        return 1
    }
}

function Get-SameProcess($target) {
    $current = Get-CimInstance Win32_Process -Filter ("ProcessId=" + $target.ProcessId) -ErrorAction SilentlyContinue
    if ($null -eq $current) {
        return $null
    }
    if ($current.Name -ine $target.Name) {
        return $null
    }
    if ([string]$current.CreationDate -ne [string]$target.CreationDate) {
        return $null
    }
    return $current
}

function Get-RemainingTargets($targets) {
    $remainingTargets = @()
    foreach ($target in $targets) {
        $sameProcess = Get-SameProcess $target.Process
        if ($null -ne $sameProcess) {
            $target.Process = $sameProcess
            $remainingTargets += $target
        }
    }
    return @($remainingTargets)
}

function New-StopTarget([string]$label, $process, [string]$expectedPath) {
    return [pscustomobject]@{
        Label = $label
        Process = $process
        ExpectedPath = $expectedPath
    }
}

$serviceNames = @($Names | ForEach-Object { $_ -split '\s+' } | ForEach-Object { $_.Trim() } | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
if ($serviceNames.Count -eq 0) {
    Write-Host "No service names were provided."
    exit 1
}

$allTargets = @()
foreach ($name in $serviceNames) {
    $processName = [System.IO.Path]::GetFileNameWithoutExtension($name)
    if ([string]::IsNullOrWhiteSpace($processName)) {
        continue
    }

    if ($processName -ieq "akshare-service") {
        $pythonTargets = @(Get-CimInstance Win32_Process -ErrorAction SilentlyContinue | Where-Object {
            $_.Name -match '^(python|py)\.exe$' -and $_.CommandLine -match 'akshare_auction_service\.py'
        })
        if ($pythonTargets.Count -gt 0) {
            $pythonPids = @($pythonTargets | ForEach-Object { $_.ProcessId } | Select-Object -Unique)
            Write-Host "AKShare python process is running. Stopping PIDs: $($pythonPids -join ', ')"
            foreach ($proc in $pythonTargets) {
                $allTargets += (New-StopTarget "AKShare python" $proc "")
            }
        } else {
            Write-Host "AKShare python process is not running."
        }
    }

    $processImageName = "$processName.exe"
    $serviceTargets = @(Get-CimInstance Win32_Process -Filter "Name = '$processImageName'" -ErrorAction SilentlyContinue)
    if ($serviceTargets.Count -gt 0) {
        $pids = @($serviceTargets | ForEach-Object { $_.ProcessId } | Select-Object -Unique)
        Write-Host "Service $processName is running. Stopping PIDs: $($pids -join ', ')"
        $expectedPath = Join-Path $Root "bin\$processImageName"
        foreach ($proc in $serviceTargets) {
            $allTargets += (New-StopTarget $processName $proc $expectedPath)
        }
    } else {
        Write-Host "Service $processName is not running."
    }
}

if ($allTargets.Count -eq 0) {
    exit 0
}

foreach ($target in $allTargets) {
    $proc = $target.Process
    if ($null -eq (Get-SameProcess $proc)) {
        continue
    }
    try {
        & $taskkill /F /T /PID $proc.ProcessId *> $null
    } catch {
        # Processes can disappear between enumeration and stop; final verification below decides.
    }
}

foreach ($target in $allTargets) {
    $proc = $target.Process
    if ($null -eq (Get-SameProcess $proc)) {
        continue
    }
    try {
        Stop-Process -Id $proc.ProcessId -Force -ErrorAction Stop
    } catch {
        # Keep going and report any PID that is still alive after the wait.
    }
}

$remainingTargets = Get-RemainingTargets $allTargets
if ($remainingTargets.Count -gt 0) {
    Write-Host "Stop requests were sent to $($allTargets.Count) process(es). Waiting for exit..."
}

$deadline = [DateTime]::UtcNow.AddMilliseconds($WaitMilliseconds)
while (($remainingTargets.Count -gt 0) -and ([DateTime]::UtcNow -lt $deadline)) {
    $stopped = $allTargets.Count - $remainingTargets.Count
    $remaining = @($remainingTargets | ForEach-Object { $_.Process.ProcessId } | Select-Object -Unique)
    Write-Host "[stop] progress $stopped/$($allTargets.Count) stopped; running PIDs: $($remaining -join ', ')"
    Start-Sleep -Seconds 1
    $remainingTargets = Get-RemainingTargets $allTargets
}

if (($remainingTargets.Count -gt 0) -and $AutoElevate -and (-not $NoAutoElevate) -and (-not (Test-CurrentProcessElevated))) {
    $elevatedExit = Invoke-ElevatedStopServices $Root $WaitMilliseconds $serviceNames
    if ($elevatedExit -eq 0) {
        Write-Host "Stopped services after administrator retry."
        exit 0
    }
    $remainingTargets = Get-RemainingTargets $allTargets
}

if ($remainingTargets.Count -gt 0) {
    $remaining = @($remainingTargets | ForEach-Object { $_.Process.ProcessId } | Select-Object -Unique)
    Write-Host "Failed to stop process PIDs: $($remaining -join ', ')"
    foreach ($target in $remainingTargets) {
        $proc = $target.Process
        $path = if ([string]::IsNullOrWhiteSpace($proc.ExecutablePath)) { "<unknown>" } else { $proc.ExecutablePath }
        $started = if ([string]::IsNullOrWhiteSpace([string]$proc.CreationDate)) { "<unknown>" } else { $proc.CreationDate }
        $commandLine = if ([string]::IsNullOrWhiteSpace($proc.CommandLine)) { "<unknown>" } else { $proc.CommandLine }
        if (-not [string]::IsNullOrWhiteSpace($target.ExpectedPath)) {
            Write-Host "Expected executable path for $($target.Label): $($target.ExpectedPath)"
        }
        Write-Host "Remaining process: Label=$($target.Label); PID=$($proc.ProcessId); Name=$($proc.Name); Path=$path; Started=$started; CommandLine=$commandLine"
    }
    exit 1
}

Write-Host "Stopped $($allTargets.Count) process(es)."
exit 0
