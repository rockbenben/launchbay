import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import assert from 'node:assert/strict';
import { connect, goHome as home, seedFixture, waitFor } from './harness.mjs';

const srv = `http://127.0.0.1:${process.argv[2]}`;
const dir = path.resolve(process.argv[3]);
const wrt = process.env.LB_UI_WRITE;
await seedFixture(srv, process.env.LB_UI_ADMIN, wrt);
const c = await connect(process.argv[4]);
const disk = () => JSON.parse(fs.readFileSync(path.join(dir, 'profiles/dad/bookmarks.json'), 'utf8'));
const profile = () => JSON.parse(fs.readFileSync(path.join(dir, 'profiles/dad/profile.json'), 'utf8'));
const login = () => c.run("fetch('/pin',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'profile=dad'}).then(r=>r.status)");
const go = () => home(c, srv, login);
const primary = 'http://10.0.0.5:5000/dual';
const wan = 'https://nas.example.com/a?q=%22x%22&y=1';
const item = () => disk().groups.flatMap(g => g.items).find(i => i.name === 'Dual URL test');
const selector = () => `li[data-item="${item().id}"]`;
const openEdit = async () => {
  await c.ev(`document.querySelector(${JSON.stringify(selector())}).querySelector('button.pencil').click()`);
  assert.equal(await waitFor(c, `!!document.querySelector('.inline-edit input[name=url]')`), true);
};
const saveEdit = async () => {
  await c.ev(`document.querySelector('.inline-edit button[type=submit]').click()`);
  await c.sleep(1200);
};
const setMode = async mode => {
  assert.equal(await c.run(`fetch('/prefs',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'linkMode=${mode}'}).then(r=>r.status)`), 200);
  await go();
  assert.equal(profile().linkMode, mode);
};

try {
  await c.send('Emulation.setDeviceMetricsOverride', { width: 1500, height: 1100, deviceScaleFactor: 1, mobile: false });
  await go();
  await c.ev(`document.getElementById('lb-add').click()`);
  await c.ev(`document.getElementById('lb-url').value=${JSON.stringify(primary)};
    document.getElementById('lb-wan').value=${JSON.stringify(wan)};
    document.getElementById('lb-title').value='Dual URL test';
    const group=document.getElementById('lb-group');group.value='tiles-1';group.dispatchEvent(new Event('change',{bubbles:true}));
    document.querySelector('#lb-sheet button[type=submit]').click()`);
  await c.sleep(1800);
  assert.equal(item()?.url, primary);
  assert.equal(item()?.urls?.wan, wan);
  console.log('PASS dual URL: add sheet writes both addresses');

  await setMode('wan');
  assert.equal(await c.run(`document.querySelector(${JSON.stringify(selector())}).querySelector('a[href]').getAttribute('href')`), wan);
  assert.equal(await c.run(`document.querySelector(${JSON.stringify(selector())}).getAttribute('data-addr-wan')`), wan);
  const shots = fs.mkdtempSync(path.join(os.tmpdir(), 'launchbay-dual-url-ui-'));
  for (const width of [1500, 390, 320]) {
    await c.send('Emulation.setDeviceMetricsOverride', { width, height: 1000, deviceScaleFactor: 1, mobile: width < 500 });
    await c.sleep(250);
    const shot = await c.send('Page.captureScreenshot', { format: 'png' });
    const file = path.join(shots, 'wan-' + width + '.png');
    fs.writeFileSync(file, Buffer.from(shot.result.data, 'base64'));
    console.log('SCREENSHOT ' + file);
    assert.equal(await c.run('document.documentElement.scrollWidth <= innerWidth'), true, 'horizontal overflow at ' + width);
  }
  await c.send('Emulation.setDeviceMetricsOverride', { width: 1500, height: 1100, deviceScaleFactor: 1, mobile: false });
  await openEdit();
  assert.equal(await c.run(`document.querySelector('.inline-edit input[name=url]').value`), primary);
  assert.equal(await c.run(`document.getElementById('lb-edit-wan').value`), wan);
  const rev = disk().rev;
  await saveEdit();
  assert.equal(disk().rev, rev);
  console.log('PASS dual URL: WAN editor roundtrip preserves bytes and sends zero ops');

  await openEdit();
  await c.ev(`document.querySelector('.inline-edit input[name=url]').value='http://10.0.0.6:5000/dual'`);
  await saveEdit();
  assert.equal(item().url, 'http://10.0.0.6:5000/dual');
  assert.equal(item().urls.wan, wan);
  await openEdit();
  await c.ev(`document.getElementById('lb-edit-wan').value='https://nas.example.com/changed'`);
  await saveEdit();
  assert.equal(item().url, 'http://10.0.0.6:5000/dual');
  assert.equal(item().urls.wan, 'https://nas.example.com/changed');
  console.log('PASS dual URL: editing each slot preserves the other');

  for (const mode of ['lan', 'wan']) {
    await setMode(mode);
    if (await waitFor(c, '!!navigator.serviceWorker.controller')) {
      const expected = mode === 'wan' ? item().urls.wan : item().url;
      await c.send('Network.emulateNetworkConditions', { offline: true, latency: 0, downloadThroughput: 0, uploadThroughput: 0 });
      await c.send('Page.navigate', { url: srv + '/' });
      await c.sleep(1700);
      const cached = await c.run(`document.querySelector(${JSON.stringify(selector())}).querySelector('a[href]').getAttribute('href')`);
      await c.send('Network.emulateNetworkConditions', { offline: false, latency: 0, downloadThroughput: -1, uploadThroughput: -1 });
      assert.equal(cached, expected);

    } else {
      throw new Error('Service Worker did not control the dual-URL page');
    }
  }
  console.log('PASS dual URL: real Service Worker returns LAN and WAN pages offline');

  for (const mode of ['lan', 'wan']) {
    await setMode(mode);
    for (const [word, wanted] of [['10.0.0.6', mode === 'lan'], ['nas.example.com', mode === 'wan']]) {
      await c.send('Input.dispatchKeyEvent', { type: 'keyDown', key: 'k', code: 'KeyK', modifiers: 2, windowsVirtualKeyCode: 75 });
      await c.send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'k', code: 'KeyK', modifiers: 2, windowsVirtualKeyCode: 75 });
      assert.equal(await waitFor(c, '!!document.getElementById("lb-k")'), true);
      await c.ev(`const input=document.getElementById('lb-k'); input.value=${JSON.stringify(word)}; input.dispatchEvent(new Event('input',{bubbles:true}));`);
      await c.sleep(400);
      assert.equal(await c.run(`[...document.querySelectorAll('.cmdk li b')].some(n=>n.textContent.trim()==='Dual URL test')`), wanted);
      await c.ev(`document.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true}))`);
      await c.send('Page.navigate', { url: srv + '/?q=' + encodeURIComponent(word) }); await c.sleep(800);
      assert.equal(await c.run(`!!document.querySelector(${JSON.stringify(selector())})`), wanted);
      await go();
    }
  }
  console.log('PASS dual URL: command panel and server search only match the active address');

  // The server-rendered href remains usable when page scripts are disabled.
  for (const mode of ['lan', 'wan']) {
    await setMode(mode);
    await c.send('Emulation.setScriptExecutionDisabled', { value: true });
    await c.send('Page.navigate', { url: srv + '/' }); await c.sleep(1000);
    assert.equal(await c.run(`document.querySelector(${JSON.stringify(selector())}).querySelector('a[href]').getAttribute('href')`), mode === 'wan' ? item().urls.wan : item().url);
    await c.send('Emulation.setScriptExecutionDisabled', { value: false });
  }
  console.log('PASS dual URL: LAN and WAN href work with JavaScript disabled');
  await go();
  // A valid URL can contain a literal trailing path space; unchanged data must not be normalized by opening the editor.
  const spaceURL = 'https://nas.example.com/path ';
  const batch = { profile: 'dad', by: 'web', expectedRev: disk().rev, ops: [{ op: 'set', path: 'groups/tiles-1/items/' + item().id + '/urls/wan', value: spaceURL }] };
  assert.equal(await c.run(`fetch('/api/v1/batch', {method:'POST',headers:{'Content-Type':'application/json'},body:${JSON.stringify(JSON.stringify(batch))}}).then(r=>r.status)`), 200);
  await go();
  await openEdit();
  const beforeSpace = disk().rev;
  await saveEdit();
  assert.equal(disk().rev, beforeSpace);
  assert.equal(item().urls.wan, spaceURL);
  console.log('PASS dual URL: unchanged raw URL bytes do not create an edit');
  await openEdit();
  await c.ev(`document.getElementById('lb-edit-wan').value=''`);
  await saveEdit();
  assert.equal(item().urls, undefined);
  assert.equal(await c.run(`document.querySelector(${JSON.stringify(selector())}).hasAttribute('data-addr-lan')`), false);
  console.log('PASS dual URL: clearing override removes urls and extra DOM attributes');
  assert.deepEqual(c.errs, []);
} finally {
  await c.send('Network.emulateNetworkConditions', { offline: false, latency: 0, downloadThroughput: -1, uploadThroughput: -1 });
  await c.send('Emulation.setScriptExecutionDisabled', { value: false });
  await c.close();
}
