[CmdletBinding(PositionalBinding = $false)]
param(
    [ValidateSet('db-start', 'db-stop', 'test', 'eval', 'migrate-up', 'migrate-down', 'generate', 'env')]
    [string]$Action = 'env',
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$Arguments
)

$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
$taskTools = Join-Path $taskRoot '.tools'
$taskLocal = Join-Path $taskRoot '.local'
$taskPgData = Join-Path $taskLocal 'pgdata'
$taskPgBin = Join-Path $taskTools 'pgsql/bin'
$taskGoExe = Join-Path $taskTools 'go/bin/go.exe'
$taskSqlcExe = Join-Path $taskTools 'sqlc/sqlc.exe'

# Literal KEY=VALUE only: no shell execution, interpolation or printing of values.
# Existing process environment takes precedence over .env.
$taskEnvPath = Join-Path $taskRoot '.env'
if (Test-Path -LiteralPath $taskEnvPath) {
    foreach ($envLine in [IO.File]::ReadAllLines($taskEnvPath)) {
        $trimmedLine = $envLine.Trim()
        if (-not $trimmedLine -or $trimmedLine.StartsWith('#')) { continue }
        if ($trimmedLine -notmatch '^(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=(.*)$') {
            throw 'Invalid .env assignment. Use KEY=VALUE and put comments on their own lines.'
        }
        $envName = $Matches[1]
        $envValue = $Matches[2].Trim()
        if ($envValue.Length -ge 2 -and (($envValue.StartsWith('"') -and $envValue.EndsWith('"')) -or ($envValue.StartsWith("'") -and $envValue.EndsWith("'")))) {
            $envValue = $envValue.Substring(1, $envValue.Length - 2)
        }
        if ([Environment]::GetEnvironmentVariable($envName, 'Process') -eq $null) {
            [Environment]::SetEnvironmentVariable($envName, $envValue, 'Process')
        }
    }
}

$taskPathEntries = @((Join-Path $taskTools 'go/bin'), (Join-Path $taskTools 'sqlc'), $taskPgBin,(Join-Path $taskTools 'node-v24.22.3-win-x64'))
$env:PATH = ($taskPathEntries -join [IO.Path]::PathSeparator) + [IO.Path]::PathSeparator + $env:PATH
if (-not $env:GOPROXY) { $env:GOPROXY = 'https://goproxy.cn,direct' }
if (-not $env:DATABASE_URL) { $env:DATABASE_URL = 'postgres://novel@127.0.0.1:55432/novel?sslmode=disable' }
if (-not $env:TEST_DATABASE_URL) { $env:TEST_DATABASE_URL = $env:DATABASE_URL }

function Invoke-TaskTool {
    param([string]$Executable, [string[]]$ToolArguments)
    if (-not (Test-Path -LiteralPath $Executable)) { throw 'A required local tool is missing. Run scripts/setup-dev.ps1 first.' }
    & $Executable @ToolArguments
    if ($LASTEXITCODE -ne 0) { throw ('Development command failed with exit code ' + $LASTEXITCODE + '.') }
}

function Start-TaskDatabase {
    $pgCtl = Join-Path $taskPgBin 'pg_ctl.exe'
    if (-not (Test-Path -LiteralPath $pgCtl)) { throw 'PostgreSQL is missing. Run scripts/setup-dev.ps1 first.' }
    New-Item -ItemType Directory -Path $taskLocal -Force | Out-Null
    if (-not (Test-Path -LiteralPath (Join-Path $taskPgData 'PG_VERSION'))) {
        if ((Test-Path -LiteralPath $taskPgData) -and @(Get-ChildItem -LiteralPath $taskPgData -Force).Count -ne 0) {
            throw 'The local database directory is not empty and has no PG_VERSION. It was left untouched.'
        }
        Invoke-TaskTool -Executable (Join-Path $taskPgBin 'initdb.exe') -ToolArguments @('-D', $taskPgData, '-U', 'novel', '-A', 'trust', '-E', 'UTF8', '--locale=C')
    }
    if ([IO.File]::ReadAllText((Join-Path $taskPgData 'PG_VERSION')).Trim() -ne '18') {
        throw 'The local data directory belongs to a different PostgreSQL major version. It was left untouched.'
    }
    & $pgCtl -D $taskPgData status *> $null
    if ($LASTEXITCODE -ne 0) {
        Invoke-TaskTool -Executable $pgCtl -ToolArguments @('-D', $taskPgData, '-l', (Join-Path $taskLocal 'postgres.log'), '-w', '-t', '30', 'start', '-o', '-h 127.0.0.1 -p 55432')
    }
    $psql = Join-Path $taskPgBin 'psql.exe'
    $databaseExists = & $psql -X -h 127.0.0.1 -p 55432 -U novel -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='novel'" -v ON_ERROR_STOP=1
    if ($LASTEXITCODE -ne 0) { throw 'Could not connect to the local development database.' }
    if (($databaseExists -join '').Trim() -ne '1') {
        Invoke-TaskTool -Executable (Join-Path $taskPgBin 'createdb.exe') -ToolArguments @('-h', '127.0.0.1', '-p', '55432', '-U', 'novel', '-E', 'UTF8', 'novel')
    }
    Write-Output 'Development PostgreSQL is ready on 127.0.0.1:55432 (database: novel).'
}

Push-Location $taskRoot
try {
    switch ($Action) {
        'env' { } # Dot-source this script to retain the environment in your current shell.
        'db-start' { Start-TaskDatabase }
        'db-stop' {
            if (Test-Path -LiteralPath (Join-Path $taskPgData 'PG_VERSION')) {
                $pgCtl = Join-Path $taskPgBin 'pg_ctl.exe'
                & $pgCtl -D $taskPgData status *> $null
                if ($LASTEXITCODE -eq 0) {
                    Invoke-TaskTool -Executable $pgCtl -ToolArguments @('-D', $taskPgData, '-w', '-t', '30', 'stop', '-m', 'fast')
                } else { Write-Output 'Development PostgreSQL is already stopped.' }
            } else { Write-Output 'No local development database has been initialized.' }
        }
        'test' { Invoke-TaskTool -Executable $taskGoExe -ToolArguments (@('test', './...', '-count=1') + $Arguments) }
        'eval' { Invoke-TaskTool -Executable $taskGoExe -ToolArguments (@('run', './cmd/eval') + $Arguments) }
        'migrate-up' { Invoke-TaskTool -Executable $taskGoExe -ToolArguments @('run', './cmd/eval', 'migrate', 'up') }
        'migrate-down' { Invoke-TaskTool -Executable $taskGoExe -ToolArguments @('run', './cmd/eval', 'migrate', 'down') }
        'generate' { Invoke-TaskTool -Executable $taskSqlcExe -ToolArguments @('generate') }
    }
} finally { Pop-Location }
