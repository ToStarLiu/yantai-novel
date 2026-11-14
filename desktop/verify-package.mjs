import fs from 'node:fs/promises';
import { createReadStream } from 'node:fs';
import { createHash } from 'node:crypto';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFileSync } from 'node:child_process';
import { extractFile, listPackage } from '@electron/asar';

const desktop = path.dirname(fileURLToPath(import.meta.url));
const repository = path.resolve(desktop, '..');
const shellFiles = new Set(['package.json', 'main.cjs', 'backend.cjs']);

function privatePath(relative) {
  const parts = relative.replaceAll('\\', '/').split('/');
  return parts.some((part) => /^\.env(?:\.|$)/i.test(part) || /^(?:\.local|\.git|pgdata|node_modules)$/i.test(part)) ||
    /(?:^|\/)(?:provider-master\.key|login\.txt|model\.env|author-credentials\.txt)$/i.test(relative) ||
    /\.(?:dump|backup|pfx|p12)$/i.test(relative);
}

async function sha256(file) {
  const hash = createHash('sha256');
  for await (const chunk of createReadStream(file)) hash.update(chunk);
  return hash.digest('hex');
}

async function inventory(directory, prefix = '') {
  const result = [];
  for (const entry of (await fs.readdir(directory, { withFileTypes: true })).sort((a, b) => a.name.localeCompare(b.name, 'en'))) {
    const relative = prefix ? prefix + '/' + entry.name : entry.name;
    if (privatePath(relative)) throw new Error('Private/runtime data found in release: ' + relative);
    if (entry.isSymbolicLink()) throw new Error('Release assets must not contain links: ' + relative);
    const file = path.join(directory, entry.name);
    if (entry.isDirectory()) result.push(...await inventory(file, relative));
    else if (entry.isFile()) result.push({ path: relative, bytes: (await fs.stat(file)).size, sha256: await sha256(file) });
    else throw new Error('Unsupported release asset: ' + relative);
  }
  return result;
}

export async function verifyPackage(packageDirectory, { expectedVersion, expectedCommit, expectedTree, requireClean = false } = {}) {
  const packageRoot = path.resolve(packageDirectory);
  const resources = path.join(packageRoot, 'resources');
  const backend = path.join(resources, 'backend');
  const archive = path.join(resources, 'app.asar');
  const application = JSON.parse(extractFile(archive, 'package.json').toString('utf8'));
  for (const name of listPackage(archive)) {
    const relative = name.replaceAll('\\', '/').replace(/^\//, '');
    if (!shellFiles.has(relative)) throw new Error('Unexpected file in desktop shell: ' + relative);
  }
  const manifestPath = path.join(backend, 'build-info.json');
  const manifest = JSON.parse(await fs.readFile(manifestPath, 'utf8'));
  if (manifest.formatVersion !== 1 || manifest.application?.name !== 'Yantai' || manifest.application?.platform !== 'win32' || manifest.application?.arch !== 'x64') throw new Error('Unsupported desktop build manifest.');
  if (!/^[a-f0-9]{40}$/.test(manifest.source?.commit || '') || !/^[a-f0-9]{40}$/.test(manifest.source?.tree || '') || typeof manifest.source?.dirty !== 'boolean') throw new Error('Build source metadata is invalid.');
  if (application.version !== manifest.application.version || (expectedVersion && application.version !== expectedVersion)) throw new Error('Packaged version differs from build/source version.');
  if (expectedCommit && manifest.source.commit !== expectedCommit) throw new Error('Packaged source commit differs from the requested release commit.');
  if (expectedTree && manifest.source.tree !== expectedTree) throw new Error('Packaged source tree differs from the requested release tree.');
  if (requireClean && manifest.source.dirty) throw new Error('Release package was built from an uncommitted checkout.');
  if (!Array.isArray(manifest.files) || !manifest.files.length) throw new Error('Build manifest has no resource inventory.');
  const actual = (await inventory(backend)).filter((file) => file.path !== 'build-info.json');
  const recorded = new Map(manifest.files.map((file) => [file.path, file]));
  if (recorded.size !== manifest.files.length || actual.length !== recorded.size) throw new Error('Backend resource inventory differs from the build manifest.');
  for (const file of actual) {
    const expected = recorded.get(file.path);
    if (!expected || file.bytes !== expected.bytes || file.sha256 !== expected.sha256) throw new Error('Packaged resource differs from its manifest: ' + file.path);
  }
  for (const required of [
    'api.exe', 'LICENSE', 'NOTICE.md', 'web/dist/index.html', 'web/dist/sw.js', 'web/dist/manifest.webmanifest',
    'harness/main.mjs', 'harness/THIRD-PARTY-NOTICES.txt', 'node/node.exe', 'node/LICENSE',
    'pgsql/bin/postgres.exe', 'pgsql/bin/initdb.exe', 'pgsql/bin/pg_ctl.exe', 'pgsql/bin/psql.exe', 'pgsql/bin/pg_dump.exe',
    'pgsql/server_license.txt', 'pgsql/commandlinetools_3rd_party_licenses.txt',
    'assets/fonts/NotoSansSC-Regular.ttf', 'assets/fonts/OFL-NotoSansSC.txt', 'assets/licenses/THIRD-PARTY-NOTICES.txt',
  ]) {
    if (!recorded.get(required)?.bytes) throw new Error('Missing required runtime resource: ' + required);
  }
  if (!actual.some((file) => file.path.startsWith('harness/licenses/'))) throw new Error('Missing Pi runtime licenses.');
  if (!Array.isArray(manifest.shellFiles) || manifest.shellFiles.length !== 2) throw new Error('Shell source hashes are missing.');
  for (const shell of manifest.shellFiles) {
    if (!['main.cjs', 'backend.cjs'].includes(shell.path)) throw new Error('Unexpected shell source hash.');
    const hash = createHash('sha256').update(extractFile(archive, shell.path)).digest('hex');
    if (hash !== shell.sha256) throw new Error('Packaged shell differs from the recorded source: ' + shell.path);
  }
  for (const required of ['Yantai.exe', 'LICENSE', 'LICENSES.chromium.html']) {
    const stat = await fs.lstat(path.join(packageRoot, required));
    if (!stat.isFile() || !stat.size) throw new Error('Missing Electron runtime resource: ' + required);
  }
  return {
    application: 'Yantai', version: application.version, platform: 'win32', arch: 'x64',
    sourceCommit: manifest.source.commit, sourceTree: manifest.source.tree, sourceDirty: manifest.source.dirty,
    backendFiles: actual.length, backendBytes: actual.reduce((sum, file) => sum + file.bytes, 0),
    manifestSha256: await sha256(manifestPath),
    apiSha256: recorded.get('api.exe').sha256,
    webIndexSha256: recorded.get('web/dist/index.html').sha256,
    harnessSha256: recorded.get('harness/main.mjs').sha256,
    shellArchiveSha256: await sha256(archive),
    executableSha256: await sha256(path.join(packageRoot, 'Yantai.exe')),
    privatePathCheck: 'passed', resourceHashes: 'matched',
  };
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const args = process.argv.slice(2);
    const directory = args.find((arg) => !arg.startsWith('--')) || path.join(desktop, 'dist', 'Yantai-win32-x64');
    const application = JSON.parse(await fs.readFile(path.join(desktop, 'package.json'), 'utf8'));
    const tree = execFileSync('git', ['rev-parse', 'HEAD^{tree}'], { cwd: repository, encoding: 'utf8', windowsHide: true }).trim();
    const result = await verifyPackage(directory, { expectedVersion: application.version, expectedTree: tree, requireClean: args.includes('--require-clean') });
    console.log(JSON.stringify(result, null, 2));
  } catch (error) { console.error(error.message); process.exitCode = 1; }
}
