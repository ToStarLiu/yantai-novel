[CmdletBinding()]
param([switch]$SkipBuild,[switch]$Offline)
$ErrorActionPreference='Stop'
$taskRoot=Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $taskRoot
. (Join-Path $PSScriptRoot 'dev.ps1') -Action env
$taskNodeDir=Join-Path $taskRoot '.tools/node-v24.22.3-win-x64'
if (-not (Test-Path (Join-Path $taskNodeDir 'node.exe'))) { throw 'Node.js is missing. Run scripts/setup-dev.ps1.' }
$env:PATH=$taskNodeDir+[IO.Path]::PathSeparator+$env:PATH
& (Join-Path $PSScriptRoot 'dev.ps1') -Action db-start
if (-not $SkipBuild) {
 if(-not (Test-Path 'web/node_modules')) { & (Join-Path $taskNodeDir 'npm.cmd') --prefix web ci; if($LASTEXITCODE -ne 0){throw 'Frontend install failed'} }
 & (Join-Path $taskNodeDir 'npm.cmd') --prefix web run build; if($LASTEXITCODE -ne 0){throw 'Frontend build failed'}
 if(-not(Test-Path 'harness/node_modules')){ & (Join-Path $taskNodeDir 'npm.cmd') --prefix harness ci; if($LASTEXITCODE -ne 0){throw 'Pi harness install failed'} }
 & (Join-Path $taskNodeDir 'npm.cmd') --prefix harness run build; if($LASTEXITCODE -ne 0){throw 'Pi harness build failed'}
 & '.tools/go/bin/go.exe' build -o '.tools/api.exe' ./cmd/api; if($LASTEXITCODE -ne 0){throw 'API build failed'}
}
if($Offline){$env:APP_PROVIDER='fake'}
$env:HARNESS_NODE_PATH=Join-Path $taskNodeDir 'node.exe'
$env:HARNESS_ENTRY_PATH=Join-Path $taskRoot 'harness/dist/main.js'
& '.tools/api.exe' migrate up; if($LASTEXITCODE -ne 0){throw 'Migration failed'}
Write-Output 'Starting workbench. The local author login is in .local/author-credentials.txt.'
& '.tools/api.exe'
