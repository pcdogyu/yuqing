param(
    [Parameter(Mandatory = $true)]
    [string]$Name,
    [int]$WaitMilliseconds = 700
)

$processName = [System.IO.Path]::GetFileNameWithoutExtension($Name.Trim())
if ([string]::IsNullOrWhiteSpace($processName)) {
    Write-Host "Service name is empty."
    exit 1
}

$processImageName = "$processName.exe"
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

Start-Sleep -Milliseconds $WaitMilliseconds
$remaining = @()
foreach ($proc in $targets) {
    if ($null -ne (Get-SameProcess $proc)) {
        $remaining += $proc.ProcessId
    }
}

if ($remaining.Count -gt 0) {
    $uniqueRemaining = @($remaining | Select-Object -Unique)
    Write-Host "Failed to stop $processName.exe PIDs: $($uniqueRemaining -join ', ')"
    exit 1
}

exit 0
