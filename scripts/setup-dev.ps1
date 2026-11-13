[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
$taskTools = Join-Path $taskRoot '.tools'
New-Item -ItemType Directory -Path $taskTools -Force | Out-Null

if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT -or -not [Environment]::Is64BitOperatingSystem) {
    throw 'This setup script requires Windows x64. On other platforms install the pinned Go, sqlc and PostgreSQL versions using their official distributions.'
}
# TLS 1.2 is needed by older Windows PowerShell installations. This changes only this process.
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

function Get-TaskArchive {
    param([string]$Url, [string]$Path, [string]$ExpectedSha256)
    if (-not (Test-Path -LiteralPath $Path)) {
        $partialPath = $Path + '.partial'
        Invoke-WebRequest -Uri $Url -OutFile $partialPath -UseBasicParsing
        if ($ExpectedSha256) {
            $actual = (Get-FileHash -LiteralPath $partialPath -Algorithm SHA256).Hash
            if ($actual -ne $ExpectedSha256) { throw 'Archive checksum mismatch. The partial download was not installed.' }
        }
        Move-Item -LiteralPath $partialPath -Destination $Path
    }
    $archiveHash = (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash
    if ($ExpectedSha256 -and $archiveHash -ne $ExpectedSha256) {
        throw 'Cached archive checksum mismatch. The archive was not installed.'
    }
    return $archiveHash
}

$goExe = Join-Path $taskTools 'go/bin/go.exe'
if (-not (Test-Path -LiteralPath $goExe)) {
    $archive = Join-Path $taskTools 'go1.27.4.windows-amd64.zip'
    Get-TaskArchive -Url 'https://go.dev/dl/go1.27.4.windows-amd64.zip' -Path $archive -ExpectedSha256 '178ab98950ac69ab25069307dc6ade79f3816e414cce95b4da166a115cde1e7e' | Out-Null
    Expand-Archive -LiteralPath $archive -DestinationPath $taskTools
}
$goVersion = & $goExe version
if ($LASTEXITCODE -ne 0 -or $goVersion -notmatch '^go version go1\.27\.4 windows/amd64$') { throw 'Expected the pinned Go 1.27.4 Windows x64 toolchain.' }
Write-Output $goVersion

$sqlcExe = Join-Path $taskTools 'sqlc/sqlc.exe'
if (-not (Test-Path -LiteralPath $sqlcExe)) {
    $archive = Join-Path $taskTools 'sqlc.zip'
    Get-TaskArchive -Url 'https://github.com/sqlc-dev/sqlc/releases/download/v1.31.1/sqlc_1.31.1_windows_amd64.zip' -Path $archive -ExpectedSha256 '352711fa7dcb05dcdfefca0ad71b2c9a74fd090f8d7fc609419de4cbc725429f' | Out-Null
    Expand-Archive -LiteralPath $archive -DestinationPath (Join-Path $taskTools 'sqlc')
}
$sqlcVersion = & $sqlcExe version
if ($LASTEXITCODE -ne 0 -or $sqlcVersion -ne 'v1.31.1') { throw 'Expected the pinned sqlc 1.31.1 binary.' }
Write-Output ('sqlc ' + $sqlcVersion)

$postgresExe = Join-Path $taskTools 'pgsql/bin/postgres.exe'
if (-not (Test-Path -LiteralPath $postgresExe)) {
    $archive = Join-Path $taskTools 'postgresql-18.6.zip'
    # EDB does not expose a checksum for this historical archive on its download page.
    # The locally recorded hash is diagnostic, not an upstream integrity assertion.
    $archiveHash = Get-TaskArchive -Url 'https://get.enterprisedb.com/postgresql/postgresql-18.6-1-windows-x64-binaries.zip' -Path $archive
    [IO.File]::WriteAllText((Join-Path $taskTools 'postgresql-18.6.download.sha256'), $archiveHash + "`n")
    Expand-Archive -LiteralPath $archive -DestinationPath $taskTools
}
$postgresVersion = & $postgresExe --version
if ($LASTEXITCODE -ne 0 -or $postgresVersion -notmatch '^postgres \(PostgreSQL\) 18\.6$') { throw 'Expected the pinned PostgreSQL 18.6 binary.' }
Write-Output $postgresVersion

$taskNodeFolder='node-v24.22.3-win-x64'
$taskNodeExe=Join-Path $taskTools ($taskNodeFolder+'/node.exe')
if(-not(Test-Path -LiteralPath $taskNodeExe)){
 $nodeArchive=Join-Path $taskTools 'node.zip'
 Get-TaskArchive -Url 'https://nodejs.org/dist/v24.22.3/node-v24.22.3-win-x64.zip' -Path $nodeArchive -ExpectedSha256 'e612af99f626ab415ea225af6bce5c971feb448cb5cae250bb1c3fa50c5f7d9f' | Out-Null
 Expand-Archive -LiteralPath $nodeArchive -DestinationPath $taskTools
}
$taskNodeVersion=& $taskNodeExe --version
if($LASTEXITCODE -ne 0 -or $taskNodeVersion -ne 'v24.22.3'){throw 'Expected Node.js v24.22.3'}
Write-Output ('Node.js '+$taskNodeVersion)
Write-Output 'Local tools are ready. This script did not change the system PATH or install a Windows service.'
