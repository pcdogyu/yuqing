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

$targets = @(Get-Process -Name $processName -ErrorAction SilentlyContinue)
if ($targets.Count -eq 0) {
    Write-Host "Service $processName is not running."
    exit 0
}

Write-Host "Service $processName is running. Stopping..."
$taskkill = Join-Path $env:SystemRoot "System32\taskkill.exe"
if (-not (Test-Path $taskkill)) {
    $taskkill = "taskkill.exe"
}

foreach ($proc in $targets) {
    try {
        & $taskkill /F /T /PID $proc.Id *> $null
    } catch {
        # A process can disappear between enumeration and kill; final verification below decides.
    }
    Start-Sleep -Milliseconds 200
    if (Get-Process -Id $proc.Id -ErrorAction SilentlyContinue) {
        try {
            Stop-Process -Id $proc.Id -Force -ErrorAction Stop
        } catch {
            # Keep going and report any PID that is still alive after the wait.
        }
    }
}

Start-Sleep -Milliseconds $WaitMilliseconds
$remaining = @()
foreach ($proc in $targets) {
    if (Get-Process -Id $proc.Id -ErrorAction SilentlyContinue) {
        $remaining += $proc.Id
    }
}

if ($remaining.Count -gt 0) {
    $uniqueRemaining = @($remaining | Select-Object -Unique)
    Write-Host "Failed to stop $processName.exe PIDs: $($uniqueRemaining -join ', ')"
    exit 1
}

exit 0
