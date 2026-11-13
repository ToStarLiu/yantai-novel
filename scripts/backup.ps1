[CmdletBinding()]
param(
    [string]$Database = 'novel',
    [string]$ServerHost = '127.0.0.1',
    [ValidateRange(1,65535)][int]$Port = 55432,
    [string]$UserName = 'novel',
    [string]$OutputDirectory = ''
)
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
$taskPgBin = Join-Path $taskRoot '.tools/pgsql/bin'
$taskDumpTool = Join-Path $taskPgBin 'pg_dump.exe'
$taskPsqlTool = Join-Path $taskPgBin 'psql.exe'
if (-not (Test-Path -LiteralPath $taskDumpTool)) { $taskDumpTool = (Get-Command pg_dump -ErrorAction Stop).Source }
if (-not (Test-Path -LiteralPath $taskPsqlTool)) { $taskPsqlTool = (Get-Command psql -ErrorAction Stop).Source }
if (-not $OutputDirectory) { $OutputDirectory = Join-Path $taskRoot '.local/backups' }
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
$taskStamp = [DateTime]::UtcNow.ToString('yyyyMMddTHHmmssfffZ')
$taskSafeDatabase = $Database -replace '[^A-Za-z0-9_-]','_'
$taskBackupFile = Join-Path $OutputDirectory ($taskSafeDatabase + '-' + $taskStamp + '.dump')
if (Test-Path -LiteralPath $taskBackupFile) { throw 'Backup destination already exists; no file was overwritten.' }
# Passwords are supplied by PGPASSFILE/PGPASSWORD if required, never command args.
$taskConnection = @('-h',$ServerHost,'-p',"$Port",'-U',$UserName,'-d',$Database,'--no-password')
& $taskDumpTool @taskConnection --format=custom --no-owner --no-acl '--exclude-schema=test_*' --file $taskBackupFile
if ($LASTEXITCODE -ne 0) { throw 'pg_dump failed. An incomplete dump may remain; it is not a verified backup.' }
$taskQuery = @'
SELECT json_build_object(
 'projects',(SELECT count(*) FROM wb_projects),
 'chapters',(SELECT count(*) FROM wb_chapters),
 'revisions',(SELECT count(*) FROM wb_revisions),
 'revisionHashDigest',(SELECT md5(COALESCE(string_agg(id||':'||hash,'|' ORDER BY id),'')) FROM wb_revisions),
 'states',(SELECT count(*) FROM wb_states),
 'facts',(SELECT count(*) FROM wb_facts),
 'runAttempts',(SELECT count(*) FROM wb_run_attempts),
 'runs',(SELECT count(*) FROM wb_runs),
 'schemaVersion',(SELECT max(version) FROM schema_migrations));
'@
$taskCountsJson = (& $taskPsqlTool @taskConnection -X -tAc $taskQuery -v ON_ERROR_STOP=1) -join ''
if ($LASTEXITCODE -ne 0) { throw 'Backup created, but verification metadata could not be read. Do not treat it as a verified backup.' }
$taskCounts = $taskCountsJson | ConvertFrom-Json

$taskTablesJson = (& $taskPsqlTool @taskConnection -X -q -tA -f (Join-Path $PSScriptRoot 'verify-table-counts.sql') -v ON_ERROR_STOP=1) -join ''
if ($LASTEXITCODE -ne 0) { throw 'Backup created, but advanced table verification failed.' }
$taskMetadata = [ordered]@{
    formatVersion = 1
    createdAtUtc = [DateTime]::UtcNow.ToString('o')
    database = $Database
    archive = [IO.Path]::GetFileName($taskBackupFile)
    sha256 = (Get-FileHash -LiteralPath $taskBackupFile -Algorithm SHA256).Hash.ToLowerInvariant()
    counts = $taskCounts
    tableCounts = ($taskTablesJson | ConvertFrom-Json)
    note = 'pg_dump uses a consistent database snapshot. Counts are sampled immediately afterward; pause application writes when comparing them.'
}
$taskVaultCount = ($taskTablesJson | ConvertFrom-Json).wb_provider_secrets
if ($taskVaultCount -gt 0) {
    $taskKeySource = [Environment]::GetEnvironmentVariable('PROVIDER_MASTER_KEY_PATH')
    if (-not $taskKeySource) { $taskKeySource = Join-Path $taskRoot '.local/provider-master.key' }
    if (-not (Test-Path -LiteralPath $taskKeySource -PathType Leaf)) { throw 'Encrypted provider credentials exist, but their master key is unavailable. Database archive was retained; backup is incomplete.' }
    $taskKeyCopy = $taskBackupFile + '.provider-master.key'
    Copy-Item -LiteralPath $taskKeySource -Destination $taskKeyCopy
    $taskMetadata.providerKeyFile = [IO.Path]::GetFileName($taskKeyCopy)
    $taskMetadata.providerKeySHA256 = (Get-FileHash -LiteralPath $taskKeyCopy -Algorithm SHA256).Hash.ToLowerInvariant()
}
[IO.File]::WriteAllText(($taskBackupFile + '.json'),($taskMetadata | ConvertTo-Json -Depth 5))
Write-Output ('Backup created: ' + $taskBackupFile)
Write-Output ('Verification metadata: ' + $taskBackupFile + '.json')
