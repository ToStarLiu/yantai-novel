[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$BackupFile,
    [Parameter(Mandatory=$true)][string]$TargetDatabase,
    [string]$ServerHost = '127.0.0.1',
    [ValidateRange(1,65535)][int]$Port = 55432,
    [string]$UserName = 'novel'
)
$ErrorActionPreference = 'Stop'
if ($TargetDatabase -notmatch '^novel_restore_[a-z0-9_]{1,40}$') { throw 'TargetDatabase must be a new name matching novel_restore_[a-z0-9_]{1,40}; existing application databases are never replaced.' }
$taskRoot = Split-Path -Parent $PSScriptRoot
$taskPgBin = Join-Path $taskRoot '.tools/pgsql/bin'
$taskRestoreTool = Join-Path $taskPgBin 'pg_restore.exe'
$taskPsqlTool = Join-Path $taskPgBin 'psql.exe'
$taskCreateTool = Join-Path $taskPgBin 'createdb.exe'
if (-not (Test-Path -LiteralPath $taskRestoreTool)) { $taskRestoreTool = (Get-Command pg_restore -ErrorAction Stop).Source }
if (-not (Test-Path -LiteralPath $taskPsqlTool)) { $taskPsqlTool = (Get-Command psql -ErrorAction Stop).Source }
if (-not (Test-Path -LiteralPath $taskCreateTool)) { $taskCreateTool = (Get-Command createdb -ErrorAction Stop).Source }
$BackupFile = [IO.Path]::GetFullPath($BackupFile)
if (-not (Test-Path -LiteralPath $BackupFile -PathType Leaf)) { throw 'BackupFile must be an existing custom-format pg_dump archive.' }
$taskMetadata = $null
if (Test-Path -LiteralPath ($BackupFile + '.json')) {
    $taskMetadata = [IO.File]::ReadAllText($BackupFile + '.json') | ConvertFrom-Json
    $taskHash = (Get-FileHash -LiteralPath $BackupFile -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($taskHash -ne $taskMetadata.sha256) { throw 'Backup checksum mismatch; target database was not created.' }
}
$taskCommon = @('-h',$ServerHost,'-p',"$Port",'-U',$UserName,'--no-password')
$taskExists = (& $taskPsqlTool @taskCommon -d postgres -X -tAc "SELECT 1 FROM pg_database WHERE datname='$TargetDatabase'" -v ON_ERROR_STOP=1) -join ''
if ($LASTEXITCODE -ne 0) { throw 'Could not check the target database.' }
if ($taskExists.Trim() -eq '1') { throw 'The target database already exists. It was left untouched; choose a new restore name.' }
& $taskCreateTool @taskCommon -E UTF8 --template=template0 $TargetDatabase
if ($LASTEXITCODE -ne 0) { throw 'Could not create a new isolated restore database.' }
& $taskRestoreTool @taskCommon -d $TargetDatabase --no-owner --no-acl --exit-on-error --single-transaction $BackupFile
if ($LASTEXITCODE -ne 0) { throw 'Restore failed. The newly created database was retained for diagnosis; the source database was not modified.' }
# A restored copy must not immediately resume billable side effects or reuse sessions.
$taskQuarantine = @'
BEGIN;
UPDATE wb_runs SET status='NeedsReconciliation',error='Restored backup: reconcile the original external call before proceeding',lease_owner='',lease_until=NULL
 WHERE status='Running';
UPDATE wb_runs SET status='Paused',error='Restored backup: verify provider settings and input before explicit resume',lease_owner='',lease_until=NULL
 WHERE status IN ('Pending','RetryScheduled');
DELETE FROM wb_sessions;
DO $restore$
BEGIN
 IF to_regclass('wb_productions') IS NOT NULL THEN
   EXECUTE 'UPDATE wb_productions SET status=''Paused'',error=''Restored backup: verify inputs and provider before explicit resume'' WHERE status IN (''Running'',''Pending'')';
 END IF;
 IF to_regclass('wb_media') IS NOT NULL THEN
   EXECUTE 'UPDATE wb_media SET status=''NeedsReconciliation'',error=''Restored backup: original media generation must be reconciled'' WHERE status=''Running''';
   EXECUTE 'UPDATE wb_media SET status=''Cancelled'',error=''Restored backup: pending media cancelled before network dispatch'' WHERE status=''Pending''';
 END IF;
 IF to_regclass('wb_candidate_groups') IS NOT NULL THEN
   EXECUTE 'UPDATE wb_candidate_groups SET status=''NeedsAttention'' WHERE status=''Running''';
 END IF;
 IF to_regclass('wb_mcp_run_inputs') IS NOT NULL THEN
   EXECUTE 'UPDATE wb_mcp_run_inputs SET status=''Failed'',error=''Restored backup: verify original tool call before using a new intent'' WHERE status=''Running''';
 END IF;
 IF to_regclass('wb_invitations') IS NOT NULL THEN
   EXECUTE 'UPDATE wb_invitations SET revoked=true WHERE NOT revoked AND accepted_by=''''';
 END IF;
 IF to_regclass('wb_presence') IS NOT NULL THEN EXECUTE 'DELETE FROM wb_presence'; END IF;
 IF to_regclass('wb_copilot_turns') IS NOT NULL THEN
   EXECUTE 'UPDATE wb_copilot_turns SET status=''NeedsReconciliation'',error=''Restored backup: reconcile original agent model call'',lease_owner='''',lease_until=NULL WHERE status=''Running''';
   EXECUTE 'UPDATE wb_copilot_turns SET status=''Cancelled'',error=''Restored backup: pending agent turn cancelled'' WHERE status=''Pending''';
 END IF;
 IF to_regclass('wb_copilot_attempts') IS NOT NULL THEN
   EXECUTE 'UPDATE wb_copilot_attempts SET status=''NeedsReconciliation'',finished_at=now() WHERE status=''Running''';
 END IF;
END
$restore$;
COMMIT;
'@
& $taskPsqlTool @taskCommon -d $TargetDatabase -X -c $taskQuarantine -v ON_ERROR_STOP=1 | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Restore completed, but task quarantine failed. Do not connect an application worker to this database.' }
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
$taskCountsJson = (& $taskPsqlTool @taskCommon -d $TargetDatabase -X -tAc $taskQuery -v ON_ERROR_STOP=1) -join ''
if ($LASTEXITCODE -ne 0) { throw 'Could not read restored verification metadata.' }
$taskCounts = $taskCountsJson | ConvertFrom-Json
$taskTablesJson = (& $taskPsqlTool @taskCommon -d $TargetDatabase -X -q -tA -f (Join-Path $PSScriptRoot 'verify-table-counts.sql') -v ON_ERROR_STOP=1) -join ''
if ($LASTEXITCODE -ne 0) { throw 'Restored database advanced table verification failed.' }
$taskTableCounts = $taskTablesJson | ConvertFrom-Json
$taskMatches = $null
if ($taskMetadata) {
    $taskMatches = $true
    foreach ($field in @('projects','chapters','revisions','revisionHashDigest','states','facts','runAttempts','runs','schemaVersion')) {
        if ("$($taskCounts.$field)" -ne "$($taskMetadata.counts.$field)") { $taskMatches = $false }
    }
    if ($taskMetadata.tableCounts) {
        foreach ($field in $taskMetadata.tableCounts.PSObject.Properties.Name) {
            if ("$($taskTableCounts.$field)" -ne "$($taskMetadata.tableCounts.$field)") { $taskMatches = $false }
        }
    }
}
$taskReceipt = [ordered]@{
    restoredAtUtc = [DateTime]::UtcNow.ToString('o')
    targetDatabase = $TargetDatabase
    backupFile = [IO.Path]::GetFileName($BackupFile)
    counts = $taskCounts
    tableCounts = $taskTableCounts
    sourceMetadataMatches = $taskMatches
    restoredTasksQuarantined = $true
    restoredSessionsCleared = $true
    note = 'Start verification with APP_PROVIDER=fake and RUN_WORKER=false. A mismatch may indicate writes occurred between pg_dump and source metadata collection; investigate before cutover.'
}
if ($taskMetadata -and $taskMetadata.providerKeyFile) {
    $taskSavedKey = Join-Path ([IO.Path]::GetDirectoryName($BackupFile)) ([IO.Path]::GetFileName($taskMetadata.providerKeyFile))
    if (-not (Test-Path -LiteralPath $taskSavedKey -PathType Leaf)) { throw 'Provider key sidecar is missing. Restored database retained; do not enable providers before restoring the matching key.' }
    if ((Get-FileHash -LiteralPath $taskSavedKey -Algorithm SHA256).Hash.ToLowerInvariant() -ne $taskMetadata.providerKeySHA256) { throw 'Provider key checksum mismatch. Active application key was not touched.' }
    $taskRestoredKeyDir = Join-Path $taskRoot '.local/restored-provider-keys'
    New-Item -ItemType Directory -Path $taskRestoredKeyDir -Force | Out-Null
    $taskRestoredKey = Join-Path $taskRestoredKeyDir ($TargetDatabase + '.key')
    if (Test-Path -LiteralPath $taskRestoredKey) { throw 'Restored provider key destination already exists and was preserved.' }
    Copy-Item -LiteralPath $taskSavedKey -Destination $taskRestoredKey
    $taskReceipt.restoredProviderKeyPath = $taskRestoredKey
    $taskReceipt.providerKeyNote = 'Use this path as PROVIDER_MASTER_KEY_PATH only when running this isolated restored database. The active application key was not changed.'
}
$taskReceiptPath = $BackupFile + '.' + $TargetDatabase + '.restore.json'
[IO.File]::WriteAllText($taskReceiptPath,($taskReceipt | ConvertTo-Json -Depth 5))
Write-Output ('Restored into new database: ' + $TargetDatabase)
Write-Output ('Verification receipt: ' + $taskReceiptPath)
if ($taskMatches -eq $false) { throw 'Restore completed, but source counts/hashes differ. See the receipt and investigate before using this restored copy.' }
Write-Output 'Task queues are paused/unknown and sessions cleared. Verify with APP_PROVIDER=fake and RUN_WORKER=false before any cutover.'
