param(
    [Parameter(Mandatory = $true)]
    [string]$ServiceName,
    [Parameter(Mandatory = $true)]
    [string]$Executable,
    [Parameter(Mandatory = $true)]
    [string]$WorkingDirectory,
    [Parameter(Mandatory = $true)]
    [string]$OutLog,
    [Parameter(Mandatory = $true)]
    [string]$ErrLog,
    [int]$TimeoutSeconds = 30,
    [string]$ArgumentFile = "",
    [string[]]$ArgumentList = @()
)

$ErrorActionPreference = "Stop"

if ($TimeoutSeconds -lt 1) {
    $TimeoutSeconds = 30
}

$Executable = [System.IO.Path]::GetFullPath($Executable)
$WorkingDirectory = [System.IO.Path]::GetFullPath($WorkingDirectory)
$OutLog = [System.IO.Path]::GetFullPath($OutLog)
$ErrLog = [System.IO.Path]::GetFullPath($ErrLog)

foreach ($path in @($OutLog, $ErrLog)) {
    $dir = Split-Path -Parent $path
    if (-not [string]::IsNullOrWhiteSpace($dir) -and -not (Test-Path -LiteralPath $dir)) {
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
    }
}

if (-not [string]::IsNullOrWhiteSpace($ArgumentFile)) {
    $ArgumentFile = [System.IO.Path]::GetFullPath($ArgumentFile)
    if (-not (Test-Path -LiteralPath $ArgumentFile)) {
        Write-Error "argument file not found: $ArgumentFile"
        exit 1
    }
    $ArgumentList = @(Get-Content -LiteralPath $ArgumentFile)
}
$argumentsJson = @($ArgumentList) | ConvertTo-Json -Compress -Depth 3

$job = Start-Job -ArgumentList $ServiceName, $Executable, $WorkingDirectory, $OutLog, $ErrLog, $argumentsJson -ScriptBlock {
    param(
        [string]$JobServiceName,
        [string]$JobExecutable,
        [string]$JobWorkingDirectory,
        [string]$JobOutLog,
        [string]$JobErrLog,
        [string]$JobArgumentsJson
    )

    $ErrorActionPreference = "Stop"
    $JobArgumentList = @()
    if (-not [string]::IsNullOrWhiteSpace($JobArgumentsJson)) {
        $parsedArguments = $JobArgumentsJson | ConvertFrom-Json
        if ($null -ne $parsedArguments) {
            $JobArgumentList = @($parsedArguments | ForEach-Object { [string]$_ })
        }
    }
    if (-not (Test-Path -LiteralPath $JobExecutable)) {
        throw "executable not found: $JobExecutable"
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
    if ($JobArgumentList.Count -gt 0) {
        $startArgs["ArgumentList"] = $JobArgumentList
    }
    $process = Start-Process @startArgs
    if ($null -eq $process) {
        throw "Start-Process returned null for $JobServiceName"
    }
    [pscustomobject]@{
        ServiceName = $JobServiceName
        ProcessId = $process.Id
    }
}

try {
    $completed = Wait-Job -Job $job -Timeout $TimeoutSeconds
    if ($null -eq $completed) {
        Stop-Job -Job $job -ErrorAction SilentlyContinue
        Write-Error "starting $ServiceName timed out after $TimeoutSeconds seconds"
        exit 124
    }

    if ($job.State -ne "Completed") {
        $errors = @($job.ChildJobs | ForEach-Object { $_.Error } | ForEach-Object { $_.ToString() })
        if ($errors.Count -gt 0) {
            Write-Error ($errors -join [Environment]::NewLine)
        } else {
            Write-Error "starting $ServiceName failed with job state $($job.State)"
        }
        exit 1
    }

    $result = Receive-Job -Job $job -ErrorAction Stop | Select-Object -Last 1
    if ($null -eq $result -or $null -eq $result.ProcessId) {
        Write-Error "starting $ServiceName did not return a process id"
        exit 1
    }
    Write-Host ("Started {0} as PID {1}" -f $result.ServiceName, $result.ProcessId)
    exit 0
} finally {
    Remove-Job -Job $job -Force -ErrorAction SilentlyContinue
}
