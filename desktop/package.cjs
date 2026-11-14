const fs = require('node:fs/promises');
const path = require('node:path');
const { createHash } = require('node:crypto');
const { createReadStream } = require('node:fs');
const { execFileSync } = require('node:child_process');

const desktop = path.resolve(__dirname);
const root = path.resolve(desktop, '..');
const backend = path.join(desktop, 'resources', 'backend');
const packageOutput = path.join(desktop, 'dist', 'Yantai-win32-x64');

async function readJSON(file) {
  return JSON.parse(await fs.readFile(file, 'utf8'));
}

// Both our cleanup and Packager's overwrite must stay in these generated directories.
async function assertGeneratedDirectory(target, relative) {
  const expected = path.resolve(desktop, ...relative);
  if (path.resolve(target) !== expected || !expected.startsWith(desktop + path.sep)) {
    throw new Error('Invalid generated output directory');
  }
  let current = desktop;
  for (const segment of ['', ...relative]) {
    if (segment) current = path.join(current, segment);
    let stat;
    try { stat = await fs.lstat(current); } catch (error) {
      if (error.code === 'ENOENT') continue;
      throw error;
    }
    if (stat.isSymbolicLink() || !stat.isDirectory()) {
      throw new Error('Generated output ancestors must be ordinary directories: ' + path.relative(root, current));
    }
  }
}

async function requiredFile(file) {
  const stat = await fs.lstat(file);
  if (!stat.isFile() || stat.size === 0) throw new Error('Required build input is missing or empty: ' + path.relative(root, file));
}

async function sha256(file) {
  const hash = createHash('sha256');
  for await (const chunk of createReadStream(file)) hash.update(chunk);
  return hash.digest('hex');
}

function privatePath(relative) {
  const parts = relative.replaceAll('\\', '/').split('/');
  return parts.some((part) => /^\.env(?:\.|$)/i.test(part) || /^(?:\.local|\.git|pgdata|node_modules)$/i.test(part)) ||
    /(?:^|\/)(?:provider-master\.key|login\.txt|model\.env|author-credentials\.txt)$/i.test(relative) ||
    /\.(?:dump|backup|pfx|p12)$/i.test(relative);
}

async function inventory(directory, prefix = '') {
  const result = [];
  for (const entry of (await fs.readdir(directory, { withFileTypes: true })).sort((a, b) => a.name.localeCompare(b.name, 'en'))) {
    const relative = prefix ? prefix + '/' + entry.name : entry.name;
    if (privatePath(relative)) throw new Error('Private/runtime data is not a release asset: ' + relative);
    if (entry.isSymbolicLink()) throw new Error('Release assets must not contain links: ' + relative);
    const file = path.join(directory, entry.name);
    if (entry.isDirectory()) result.push(...await inventory(file, relative));
    else if (entry.isFile()) result.push({ path: relative, bytes: (await fs.stat(file)).size, sha256: await sha256(file) });
    else throw new Error('Unsupported release asset: ' + relative);
  }
  return result;
}

async function findElectronZip(directory, filename) {
  let entries;
  try { entries = await fs.readdir(directory, { withFileTypes: true }); } catch (error) {
    if (error.code === 'ENOENT') return undefined;
    throw error;
  }
  for (const entry of entries) {
    if (entry.isSymbolicLink()) continue;
    const file = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      const found = await findElectronZip(file, filename);
      if (found) return found;
    } else if (entry.isFile() && entry.name === filename) return file;
  }
}

(async () => {
  if (process.platform !== 'win32' || process.arch !== 'x64') throw new Error('The portable package currently requires a Windows x64 build host.');
  const [application, web, harness] = await Promise.all([
    readJSON(path.join(desktop, 'package.json')),
    readJSON(path.join(root, 'web', 'package.json')),
    readJSON(path.join(root, 'harness', 'package.json')),
  ]);
  if (!/^\d+\.\d+\.\d+$/.test(application.version) || web.version !== application.version || harness.version !== application.version) {
    throw new Error('Desktop, Web and Pi package versions must match.');
  }
  const go = path.join(root, '.tools', 'go', 'bin', 'go.exe');
  for (const file of [go, path.join(root, 'web', 'dist', 'index.html'), path.join(root, 'harness', 'bundle', 'main.mjs'), path.join(root, 'harness', 'bundle', 'THIRD-PARTY-NOTICES.txt')]) await requiredFile(file);
  const sourceCommit = execFileSync('git', ['rev-parse', 'HEAD'], { cwd: root, encoding: 'utf8', windowsHide: true }).trim();
  if (!/^[a-f0-9]{40}$/.test(sourceCommit)) throw new Error('Cannot identify the source revision.');
  const sourceTree = execFileSync('git', ['rev-parse', 'HEAD^{tree}'], { cwd: root, encoding: 'utf8', windowsHide: true }).trim();
  if (!/^[a-f0-9]{40}$/.test(sourceTree)) throw new Error('Cannot identify the source tree.');
  const sourceDirty = execFileSync('git', ['status', '--porcelain', '--untracked-files=normal'], { cwd: root, encoding: 'utf8', windowsHide: true }).trim() !== '';
  const { packager } = await import('@electron/packager');
  await import('../scripts/collect-runtime-licenses.mjs');
  await assertGeneratedDirectory(backend, ['resources', 'backend']);
  await assertGeneratedDirectory(packageOutput, ['dist', 'Yantai-win32-x64']);
  await fs.rm(backend, { recursive: true, force: true });
  await fs.mkdir(backend, { recursive: true });

  // Always build from this checkout; a leftover development executable is never copied.
  execFileSync(go, ['build', '-trimpath', '-o', path.join(backend, 'api.exe'), './cmd/api'], { cwd: root, stdio: 'inherit', windowsHide: true });
  for (const item of ['LICENSE', 'NOTICE.md']) await fs.copyFile(path.join(root, item), path.join(backend, item));
  for (const item of ['web/dist', 'assets/fonts', 'assets/licenses']) await fs.cp(path.join(root, item), path.join(backend, item), { recursive: true, force: true });
  await fs.mkdir(path.join(backend, 'harness'), { recursive: true });
  for (const item of ['main.mjs', 'THIRD-PARTY-NOTICES.txt', 'licenses']) await fs.cp(path.join(root, 'harness', 'bundle', item), path.join(backend, 'harness', item), { recursive: true, force: true });
  await fs.mkdir(path.join(backend, 'node'), { recursive: true });
  for (const item of ['node.exe', 'LICENSE']) await fs.copyFile(path.join(root, '.tools', 'node-v24.22.3-win-x64', item), path.join(backend, 'node', item));
  for (const item of ['bin', 'lib', 'share']) await fs.cp(path.join(root, '.tools', 'pgsql', item), path.join(backend, 'pgsql', item), { recursive: true, force: true });
  for (const item of ['server_license.txt', 'commandlinetools_3rd_party_licenses.txt']) await fs.copyFile(path.join(root, '.tools', 'pgsql', item), path.join(backend, 'pgsql', item));
  const manifest = {
    formatVersion: 1,
    application: { name: 'Yantai', version: application.version, platform: 'win32', arch: 'x64' },
    source: { commit: sourceCommit, tree: sourceTree, dirty: sourceDirty },
    shellFiles: await Promise.all(['main.cjs', 'backend.cjs'].map(async (file) => ({ path: file, sha256: await sha256(path.join(desktop, file)) }))),
    createdAt: new Date().toISOString(),
    files: await inventory(backend),
  };
  await fs.writeFile(path.join(backend, 'build-info.json'), JSON.stringify(manifest, null, 2) + '\n');

  const electronVersion = application.devDependencies.electron;
  const filename = 'electron-v' + electronVersion + '-win32-x64.zip';
  const cache = path.join(root, '.local', 'electron-zip');
  await fs.mkdir(cache, { recursive: true });
  const zip = process.env.LOCALAPPDATA ? await findElectronZip(path.join(process.env.LOCALAPPDATA, 'electron', 'Cache'), filename) : undefined;
  if (zip) await fs.copyFile(zip, path.join(cache, filename));
  const shellFiles = new Set(['', '/package.json', '/main.cjs', '/backend.cjs']);
  // The shell uses only built-ins. Do not package arbitrary files from desktop/.
  const outputs = await packager({
    dir: desktop, name: 'Yantai', platform: 'win32', arch: 'x64',
    appVersion: application.version, buildVersion: application.version,
    out: path.join(desktop, 'dist'), overwrite: true, prune: false, asar: true,
    ignore: (relative) => !shellFiles.has(relative.replaceAll('\\', '/')),
    extraResource: [backend], electronZipDir: zip ? cache : undefined,
  });
  if (outputs.length !== 1 || path.resolve(outputs[0]) !== packageOutput) throw new Error('Unexpected desktop package output.');
  const currentCommit = execFileSync('git', ['rev-parse', 'HEAD'], { cwd: root, encoding: 'utf8', windowsHide: true }).trim();
  const currentDirty = execFileSync('git', ['status', '--porcelain', '--untracked-files=normal'], { cwd: root, encoding: 'utf8', windowsHide: true }).trim() !== '';
  if (currentCommit !== sourceCommit || (!sourceDirty && currentDirty)) throw new Error('The checkout changed during packaging; rebuild from the final release commit.');
  const { verifyPackage } = await import('./verify-package.mjs');
  const verification = await verifyPackage(packageOutput, { expectedVersion: application.version, expectedCommit: sourceCommit, expectedTree: sourceTree });
  await fs.writeFile(path.join(desktop, 'dist', 'package-verification.json'), JSON.stringify(verification, null, 2) + '\n');
  console.log(JSON.stringify(verification, null, 2));
  console.log(outputs.join('\n'));
})().catch((error) => { console.error(error); process.exitCode = 1; });
