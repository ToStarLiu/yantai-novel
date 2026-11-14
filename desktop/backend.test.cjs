const { test } = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');
const os = require('node:os');
const { randomUUID } = require('node:crypto');
const { startBackend } = require('./backend.cjs');

test('an incomplete packaged app fails before starting or connecting to another workbench', async () => {
  const descriptor = Object.getOwnPropertyDescriptor(process, 'resourcesPath');
  const remote = process.env.NOVEL_DESKTOP_URL;
  delete process.env.NOVEL_DESKTOP_URL;
  Object.defineProperty(process, 'resourcesPath', { value: path.join(os.tmpdir(), 'yantai-missing-' + randomUUID()), configurable: true });
  try {
    await assert.rejects(startBackend({ isPackaged: true, getPath() { throw new Error('User data must remain untouched'); } }), /桌面包缺少本地服务/);
  } finally {
    if (descriptor) Object.defineProperty(process, 'resourcesPath', descriptor);
    else delete process.resourcesPath;
    if (remote === undefined) delete process.env.NOVEL_DESKTOP_URL;
    else process.env.NOVEL_DESKTOP_URL = remote;
  }
});
