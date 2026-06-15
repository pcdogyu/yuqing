param(
    [string]$SQLitePath = "data\yuqing.db",
    [string]$HostName = "10.15.0.19",
    [int]$Port = 5432,
    [string]$Database = "yuqing",
    [string]$User = "admin",
    [string]$Password = "",
    [string]$SchemaPath = "",
    [switch]$Truncate
)

$ErrorActionPreference = "Stop"

if (-not $SchemaPath) {
    $SchemaPath = Join-Path $PSScriptRoot "..\db\postgres_schema.sql"
}
$SchemaPath = (Resolve-Path $SchemaPath).Path

$go = Get-Command go -ErrorAction SilentlyContinue
if (-not $go) {
    throw "go was not found. Install Go or run a prebuilt migrate-sqlite-to-postgres binary."
}

$moduleRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Push-Location $moduleRoot
try {
    $args = @(
        "run", ".\cmd\migrate-sqlite-to-postgres",
        "-sqlite", $SQLitePath,
        "-host", $HostName,
        "-port", [string]$Port,
        "-database", $Database,
        "-user", $User,
        "-password", $Password,
        "-schema", $SchemaPath
    )
    if ($Truncate) {
        $args += "-truncate"
    }
    & $go.Source @args
    if ($LASTEXITCODE -ne 0) {
        throw "sqlite to postgres migration failed"
    }
}
finally {
    Pop-Location
}
