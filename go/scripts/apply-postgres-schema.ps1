param(
    [string]$HostName = "10.15.0.19",
    [int]$Port = 5432,
    [string]$Database = "yuqing",
    [string]$User = "postgres",
    [string]$Password = "",
    [string]$SchemaPath = "",
    [string]$DockerImage = "postgres:14"
)

$ErrorActionPreference = "Stop"

if ($Database -notmatch '^[A-Za-z0-9_]+$') {
    throw "Database name must contain only letters, digits, and underscore."
}

if (-not $SchemaPath) {
    $SchemaPath = Join-Path $PSScriptRoot "..\db\postgres_schema.sql"
}
$SchemaPath = (Resolve-Path $SchemaPath).Path

$psql = Get-Command psql -ErrorAction SilentlyContinue
$docker = Get-Command docker -ErrorAction SilentlyContinue
$go = Get-Command go -ErrorAction SilentlyContinue
if (-not $psql -and -not $docker -and -not $go) {
    throw "Neither psql, docker, nor go was found. Install PostgreSQL client tools or Go, then rerun this script."
}

function Invoke-Psql {
    param(
        [string]$DbName,
        [string[]]$ExtraArgs
    )

    $oldActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    $common = @("-h", $HostName, "-p", [string]$Port, "-U", $User, "-v", "ON_ERROR_STOP=1", "-d", $DbName)
    try {
        if ($psql) {
            & $psql.Source @common @ExtraArgs *> $null
            $code = $LASTEXITCODE
            return $code
        }

        $envArgs = @()
        if ($Password) {
            $envArgs = @("-e", "PGPASSWORD=$Password")
        }
        $schemaDir = Split-Path -Parent $SchemaPath
        $schemaFile = Split-Path -Leaf $SchemaPath
        $dockerArgs = @("run", "--rm") + $envArgs + @("-v", "${schemaDir}:/schema:ro", $DockerImage, "psql") + $common
        $mappedExtraArgs = @()
        foreach ($arg in $ExtraArgs) {
            if ($arg -eq $SchemaPath) {
                $mappedExtraArgs += "/schema/$schemaFile"
            } else {
                $mappedExtraArgs += $arg
            }
        }
        & $docker.Source @dockerArgs @mappedExtraArgs *> $null
        $code = $LASTEXITCODE
        return $code
    }
    finally {
        $ErrorActionPreference = $oldActionPreference
    }
}

$oldPassword = $env:PGPASSWORD
try {
    if ($Password) {
        $env:PGPASSWORD = $Password
    }

    if (-not $psql -and -not $docker) {
        $moduleRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
        Push-Location $moduleRoot
        try {
            & $go.Source run .\cmd\apply-postgres-schema `
                -host $HostName `
                -port ([string]$Port) `
                -database $Database `
                -user $User `
                -password $Password `
                -schema $SchemaPath
            if ($LASTEXITCODE -ne 0) {
                throw "failed to apply schema with go fallback"
            }
        }
        finally {
            Pop-Location
        }
        return
    }

    $exitCode = Invoke-Psql -DbName $Database -ExtraArgs @("-c", "SELECT 1;")
    if ($exitCode -ne 0) {
        Write-Host "database '$Database' is not reachable, trying to create it from postgres database..."
        $exitCode = Invoke-Psql -DbName "postgres" -ExtraArgs @("-c", "CREATE DATABASE `"$Database`";")
        if ($exitCode -ne 0) {
            throw "failed to create database '$Database'"
        }
    }

    $exitCode = Invoke-Psql -DbName $Database -ExtraArgs @("-f", $SchemaPath)
    if ($exitCode -ne 0) {
        throw "failed to apply schema"
    }

    Write-Host "postgres schema applied: host=$HostName port=$Port database=$Database user=$User schema=$SchemaPath"
}
finally {
    $env:PGPASSWORD = $oldPassword
}
