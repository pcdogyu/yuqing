param(
    [Parameter(Mandatory = $true)]
    [string]$Name,
    [int]$WaitMilliseconds = 5000,
    [string]$ExpectedPath = ""
)

$processName = [System.IO.Path]::GetFileNameWithoutExtension($Name.Trim())
if ([string]::IsNullOrWhiteSpace($processName)) {
    Write-Host "Service name is empty."
    exit 1
}

$processImageName = "$processName.exe"
$expectedExecutablePath = ""
if (-not [string]::IsNullOrWhiteSpace($ExpectedPath)) {
    try {
        $expectedExecutablePath = [System.IO.Path]::GetFullPath($ExpectedPath.Trim())
    } catch {
        $expectedExecutablePath = $ExpectedPath.Trim()
    }
}
$targets = @(Get-CimInstance Win32_Process -Filter "Name = '$processImageName'" -ErrorAction SilentlyContinue)
if ($targets.Count -eq 0) {
    Write-Host "Service $processName is not running."
    exit 0
}

Write-Host "Service $processName is running. Stopping..."
$taskkill = Join-Path $env:SystemRoot "System32\taskkill.exe"
if (-not (Test-Path $taskkill)) {
    $taskkill = "taskkill.exe"
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
        $sameProcess = Get-SameProcess $target
        if ($null -ne $sameProcess) {
            $remainingTargets += $sameProcess
        }
    }
    return @($remainingTargets)
}

foreach ($proc in $targets) {
    if ($null -eq (Get-SameProcess $proc)) {
        continue
    }
    try {
        & $taskkill /F /T /PID $proc.ProcessId *> $null
    } catch {
        # A process can disappear between enumeration and kill; final verification below decides.
    }
    Start-Sleep -Milliseconds 200
    if ($null -ne (Get-SameProcess $proc)) {
        try {
            Stop-Process -Id $proc.ProcessId -Force -ErrorAction Stop
        } catch {
            # Keep going and report any PID that is still alive after the wait.
        }
    }
}

$deadline = [DateTime]::UtcNow.AddMilliseconds($WaitMilliseconds)
$remainingTargets = Get-RemainingTargets $targets
while (($remainingTargets.Count -gt 0) -and ([DateTime]::UtcNow -lt $deadline)) {
    Start-Sleep -Milliseconds 200
    $remainingTargets = Get-RemainingTargets $targets
}

if ($remainingTargets.Count -gt 0) {
    $remaining = @($remainingTargets | ForEach-Object { $_.ProcessId } | Select-Object -Unique)
    Write-Host "Failed to stop $processName.exe PIDs: $($remaining -join ', ')"
    if (-not [string]::IsNullOrWhiteSpace($expectedExecutablePath)) {
        Write-Host "Expected executable path: $expectedExecutablePath"
    }
    foreach ($proc in $remainingTargets) {
        $path = if ([string]::IsNullOrWhiteSpace($proc.ExecutablePath)) { "<unknown>" } else { $proc.ExecutablePath }
        $started = if ([string]::IsNullOrWhiteSpace([string]$proc.CreationDate)) { "<unknown>" } else { $proc.CreationDate }
        $commandLine = if ([string]::IsNullOrWhiteSpace($proc.CommandLine)) { "<unknown>" } else { $proc.CommandLine }
        Write-Host "Remaining process: PID=$($proc.ProcessId); Name=$($proc.Name); Path=$path; Started=$started; CommandLine=$commandLine"
    }
    exit 1
}

exit 0
