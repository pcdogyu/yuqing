param(
    [string]$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path,
    [string]$ReleaseDir = "",
    [string]$ServerShare = "\\10.15.0.7\yuqing-release",
    [string]$ServerReleaseDir = "C:\yuqing\release",
    [string]$ServerUser = "10.15.0.7\hyuser",
    [string]$ReleaseBaseUrl = "http://10.15.0.7:8099",
    [string]$ApkSigner = "",
    [switch]$SkipBuild,
    [switch]$SkipCopy,
    [switch]$SkipNetUse,
    [switch]$KeepConnection
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($ReleaseDir)) {
    $ReleaseDir = Join-Path $RepoRoot "release"
}

$androidDir = Join-Path $RepoRoot "android"
if (-not (Test-Path -LiteralPath $androidDir)) {
    throw "Android directory not found: $androidDir"
}

if (-not (Test-Path -LiteralPath $ReleaseDir)) {
    New-Item -ItemType Directory -Force -Path $ReleaseDir | Out-Null
}

if (-not $SkipBuild) {
    Push-Location $androidDir
    try {
        Write-Host "Building Android release APK..."
        gradle --console=plain :app:assembleRelease
    } finally {
        Pop-Location
    }
}

$apk = Get-ChildItem -LiteralPath $ReleaseDir -Filter "*.apk" -File |
    Sort-Object LastWriteTime, Name -Descending |
    Select-Object -First 1
$zip = Get-ChildItem -LiteralPath $ReleaseDir -Filter "*.zip" -File |
    Sort-Object LastWriteTime, Name -Descending |
    Select-Object -First 1

if (-not $apk) {
    throw "No APK found in $ReleaseDir"
}
if (-not $zip) {
    Write-Warning "No ZIP found in $ReleaseDir. Check Android Gradle release copy task."
}

if ([string]::IsNullOrWhiteSpace($ApkSigner)) {
    $sdkRoots = @($env:ANDROID_HOME, $env:ANDROID_SDK_ROOT, (Join-Path $env:LOCALAPPDATA "Android\Sdk")) |
        Where-Object { -not [string]::IsNullOrWhiteSpace($_) } |
        Select-Object -Unique
    foreach ($sdkRoot in $sdkRoots) {
        $candidate = Get-ChildItem -LiteralPath (Join-Path $sdkRoot "build-tools") -Recurse -Filter "apksigner.bat" -ErrorAction SilentlyContinue |
            Sort-Object FullName -Descending |
            Select-Object -First 1
        if ($candidate) {
            $ApkSigner = $candidate.FullName
            break
        }
    }
}
if ([string]::IsNullOrWhiteSpace($ApkSigner) -or -not (Test-Path -LiteralPath $ApkSigner)) {
    throw "apksigner.bat not found. Set ANDROID_HOME or pass -ApkSigner."
}

Write-Host "Verifying APK signature..."
& $ApkSigner verify --verbose $apk.FullName
if ($LASTEXITCODE -ne 0) {
    throw "APK signature verification failed: $($apk.FullName)"
}

Write-Host "Latest APK: $($apk.FullName)"
if ($zip) {
    Write-Host "Latest ZIP: $($zip.FullName)"
}

Write-Host ""
Write-Host "Server release-service environment example:"
Write-Host "`$env:YUQING_RELEASE_ADDR=':8099'"
Write-Host "`$env:YUQING_RELEASE_DIR='$ServerReleaseDir'"
Write-Host "`$env:YUQING_RELEASE_URL='$ReleaseBaseUrl'"
Write-Host "Set-Location C:\yuqing\go"
Write-Host ".\run.bat --skip-pull"
Write-Host ""
Write-Host "Copy command:"
Write-Host "net use $ServerShare /user:$ServerUser * /persistent:no"
Write-Host "robocopy $ReleaseDir $ServerShare *.apk *.zip /XO /R:2 /W:2"
Write-Host "net use $ServerShare /delete"
Write-Host ""
Write-Host "Verify command:"
Write-Host "Invoke-WebRequest -UseBasicParsing $($ReleaseBaseUrl.TrimEnd('/'))/release/ | Select-Object -ExpandProperty Content"
Write-Host "Invoke-RestMethod $($ReleaseBaseUrl.TrimEnd('/'))/api/v1/release/latest | ConvertTo-Json -Depth 6"
Write-Host ""

if ($SkipCopy) {
    Write-Host "SkipCopy enabled; not copying files."
    exit 0
}

if (-not $SkipNetUse) {
    net use $ServerShare /user:$ServerUser * /persistent:no
    if ($LASTEXITCODE -ne 0) {
        throw "net use failed with exit code $LASTEXITCODE"
    }
}

try {
    robocopy $ReleaseDir $ServerShare *.apk *.zip /XO /R:2 /W:2
    $robocopyCode = $LASTEXITCODE
    if ($robocopyCode -ge 8) {
        throw "robocopy failed with exit code $robocopyCode"
    }
} finally {
    if (-not $KeepConnection -and -not $SkipNetUse) {
        net use $ServerShare /delete | Out-Null
    }
}

$latestUrl = $ReleaseBaseUrl.TrimEnd("/") + "/api/v1/release/latest"
try {
    $latest = Invoke-RestMethod -UseBasicParsing -Uri $latestUrl -TimeoutSec 10
    Write-Host "Latest API response:"
    $latest | ConvertTo-Json -Depth 8
} catch {
    Write-Warning "Release latest check failed: $($_.Exception.Message)"
}
