import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { connect, goHome, seedFixture, waitFor } from './harness.mjs';

const srv = `http://127.0.0.1:${process.argv[2]}`;
await seedFixture(srv, process.env.LB_UI_ADMIN, process.env.LB_UI_WRITE);
const c = await connect(process.argv[4]);
let color = 'blue';
const site = createServer((req, res) => {
  if (req.url !== '/icon.svg') {
    res.writeHead(200, { 'Content-Type': 'text/html' });
    res.end('<title>Site icon fixture</title><link rel="icon" href="/icon.svg">');
  } else if (color) {
    res.writeHead(200, { 'Content-Type': 'image/svg+xml' });
    res.end(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><rect width="16" height="16" fill="${color}"/></svg>`);
  } else {
    res.writeHead(404).end();
  }
});
await new Promise(resolve => site.listen(0, '127.0.0.1', resolve));
const address = `http://127.0.0.1:${site.address().port}/bookmark`;
const headers = { Authorization: 'Bearer ' + process.env.LB_UI_WRITE, 'Content-Type': 'application/json' };
const img = 'li[data-item="site-icon-test"] .ic-img';
const iconSrc = () => c.run(`document.querySelector('${img}')?.getAttribute('src') || ''`);
const revision = async () => (await (await fetch(srv + '/api/v1/data?profile=dad', { headers })).json()).rev;
const changeIcon = id => c.ev(`{
  const select = document.getElementById('${id}');
  select.value = 'auto';
  select.dispatchEvent(new Event('change', { bubbles: true }));
}`);
const refresh = () => c.ev("document.querySelector('.inline-edit .site-icon-origin').click()");

try {
  const seeded = await fetch(srv + '/api/v1/batch', {
    method: 'POST', headers,
    body: JSON.stringify({
      profile: 'dad', by: 'cli',
      ops: [
        { op: 'add', path: 'groups/tiles-1/items',
          value: { id: 'site-icon-test', name: 'Site icon fixture', url: address } },
        { op: 'add', path: 'groups/tiles-1/items',
          value: { id: 'site-icon-peer', name: 'Same site', url: address + '/peer', icon: 'auto' } },
        { op: 'add', path: 'groups/tiles-1/items',
          value: { id: 'backup-icon-test', name: 'Backup icon fixture', url: 'https://linux.do/docs?private=1', icon: 'tabler:home' } },
      ],
    }),
  });
  assert.equal(seeded.status, 200, await seeded.text());
  const login = () => c.run("fetch('/pin',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'profile=dad'}).then(r=>r.status)");
  await goHome(c, srv, login);
  assert.equal(await c.run("!!document.querySelector('li[data-item=site-icon-peer] [data-icon-missing]')"), true);
  await c.ev("document.querySelector('li[data-item=site-icon-test] button.pencil').click()");
  await changeIcon('lb-edit-icon');
  assert.equal(await waitFor(c, "!document.querySelector('li[data-item=site-icon-peer] [data-icon-missing]')"), true);
  await c.ev("document.querySelector('.inline-edit button[type=submit]').click()");
  assert.equal(await waitFor(c, `document.querySelector('${img}')?.naturalWidth > 0`), true);
  assert.equal(await c.run("!!document.querySelector('li[data-item=site-icon-peer] [data-icon-missing]')"), false);
  const oldIcon = await iconSrc();
  const rev = await revision();

  await c.ev("document.querySelector('li[data-item=site-icon-test] button.pencil').click()");
  color = 'red';
  await refresh();
  assert.equal(await waitFor(c, `document.querySelector('${img}')?.getAttribute('src') !== ${JSON.stringify(oldIcon)}
    && document.querySelector('.inline-edit .site-icon-status img')?.naturalWidth > 0`), true);
  const newIcon = await iconSrc();
  assert.equal(await revision(), rev);
  assert.equal(await c.run("document.getElementById('lb-edit-icon').value"), 'auto');

  color = '';
  await refresh();
  assert.equal(await waitFor(c, "document.querySelector('.inline-edit .site-icon-status').textContent.includes('未能获取网站图标')"), true);
  assert.equal(await iconSrc(), newIcon);
  await c.ev("document.querySelector('.inline-edit button[type=submit]').click()");
  assert.equal(await revision(), rev);

  const addIconSource = "#lb-sheet .site-icon-field a";
  const backupButton = scope => scope + ' .site-icon-backup';
  const backupHidden = () => c.run(`document.querySelector('${backupButton('#lb-sheet')}').hidden`);
  await c.ev("document.getElementById('lb-add').click()");
  assert.equal(await c.run(`document.querySelector('${addIconSource}').parentElement.hidden`), true);
  const typeAddress = url => c.ev(`{
    const input = document.getElementById('lb-url');
    input.value = ${JSON.stringify(url)};
    input.dispatchEvent(new Event('input', { bubbles: true }));
  }`);
  await typeAddress('https://user:secret@example.org:8443/private?q=token#section');
  assert.equal(await c.run(`document.querySelector('${addIconSource}').href`), 'https://example.org:8443/favicon.ico');
  assert.equal(await backupHidden(), false);
  for (const privateURL of ['http://nas/', 'http://printer.local/', 'http://device.home.arpa/', 'http://127.0.0.1/', 'http://[::1]/']) {
    await typeAddress(privateURL);
    assert.equal(await backupHidden(), true, privateURL);
  }
  await typeAddress('chrome://settings');
  assert.equal(await c.run(`document.querySelector('${addIconSource}').hasAttribute('href')`), false);
  await typeAddress(address + '/another');
  assert.equal(await c.run(`document.querySelector('${addIconSource}').href`), new URL(address).origin + '/favicon.ico');
  await changeIcon('lb-icon');
  assert.equal(await waitFor(c, "document.querySelector('#lb-sheet .site-icon-status img')?.naturalWidth > 0"), true);
  assert.equal(await c.run(`document.querySelector('${addIconSource}').getAttribute('href')`), newIcon);
  await typeAddress('https://example.net/docs');
  assert.equal(await c.run(`document.querySelector('${addIconSource}').href`), 'https://example.net/favicon.ico');
  assert.equal(await backupHidden(), false);
  assert.equal(await c.run("performance.getEntriesByType('resource').some(e => e.name.endsWith('/api/v1/icons/fallback'))"), false);
  await c.ev("document.querySelector('#lb-sheet button.close').click()");
  await c.ev("document.querySelector('li[data-item=site-icon-test] button.pencil').click()");
  assert.equal(await c.run("document.querySelector('.inline-edit .site-icon-field a').getAttribute('href')"), newIcon);
  const setIconFile = (name, content) => c.ev(`{
    const input = document.querySelector('.inline-edit input[type=file]');
    const transfer = new DataTransfer();
    transfer.items.add(new File([${JSON.stringify(content)}], ${JSON.stringify(name)}));
    input.files = transfer.files;
    input.dispatchEvent(new Event('change', { bubbles: true }));
  }`);
  await setIconFile('bad.png', 'not an image');
  assert.equal(await waitFor(c, "document.querySelector('.inline-edit .site-icon-status').textContent.includes('无法读取图片')"), true);
  assert.equal(await c.run("document.getElementById('lb-edit-icon').value"), 'auto');

  await setIconFile('custom.svg', '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 8"><rect width="16" height="8" fill="green"/></svg>');
  assert.equal(await waitFor(c, "document.querySelector('.inline-edit .site-icon-status img')?.naturalWidth === 64"), true);
  assert.equal(await iconSrc(), newIcon);
  assert.equal(await revision(), rev);
  await c.ev("document.querySelector('.inline-edit button[type=submit]').click()");
  assert.equal(await waitFor(c, `document.querySelector('${img}')?.src.startsWith('data:image/png;base64,')
    && document.querySelector('${img}')?.naturalWidth === 64`), true);
  assert.equal(await revision(), rev + 1);
  await c.ev("document.querySelector('li[data-item=site-icon-test] button.pencil').click()");
  assert.equal(await waitFor(c, "document.querySelector('.inline-edit .site-icon-status img')?.naturalWidth === 64"), true);
  await c.ev("document.querySelector('.inline-edit button[type=submit]').click()");
  assert.equal(await revision(), rev + 1);
  await c.ev("document.querySelector('li[data-item=backup-icon-test] button.pencil').click()");
  assert.equal(await c.run("document.querySelector('.inline-edit .edit-icon-row').firstElementChild.matches('.site-icon-backup.is-suggested')"), true);
  const fallbackImage = 'data:image/svg+xml;base64,' + Buffer.from(
    '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"><rect width="16" height="16" fill="green"/></svg>'
  ).toString('base64');
  await c.ev(`{
    const original = window.fetch;
    window.fallbackRequests = [];
    window.originAvailable = false;
    window.fetch = (path, options) => {
      if (path === '/api/v1/resolve') return Promise.resolve(window.originAvailable
        ? Response.json({ favicon: ${JSON.stringify(newIcon)}, iconCandidates: [] })
        : Response.json({ kind: 'target-unreachable' }, { status: 502 }));
      if (path !== '/api/v1/icons/fallback') return original(path, options);
      window.fallbackRequests.push(JSON.parse(options.body));
      if (window.fallbackRequests.length === 1) return Promise.resolve(new Response('{}', { status: 502 }));
      return Promise.resolve(Response.json({ image: ${JSON.stringify(fallbackImage)} }));
    };
  }`);
  await refresh();
  assert.equal(await waitFor(c, "document.querySelector('.inline-edit .site-icon-status').textContent.includes('目标网页无法访问') && !document.querySelector('.inline-edit .site-icon-origin').disabled"), true);
  assert.equal(await c.run("document.querySelector('.inline-edit .site-icon-backup').classList.contains('is-suggested')"), true);
  await c.ev("window.originAvailable = true");
  await refresh();
  assert.equal(await waitFor(c, "document.querySelector('.inline-edit .site-icon-status img')?.naturalWidth > 0"), true);
  assert.equal(await c.run("document.querySelector('.inline-edit .site-icon-backup').hidden"), false);
  assert.equal(await c.run("window.fallbackRequests.length"), 0);
  const clickBackup = () => c.ev(`document.querySelector('${backupButton('.inline-edit')}').click()`);
  await clickBackup();
  assert.equal(await waitFor(c, "document.querySelector('.inline-edit .site-icon-status').textContent.includes('未能从 DuckDuckGo 获取图标')"), true);
  assert.equal(await c.run("document.getElementById('lb-edit-icon').value"), 'tabler:home');
  await clickBackup();
  assert.equal(await waitFor(c, "document.querySelector('.inline-edit .site-icon-status img')?.naturalWidth === 64"), true);
  assert.equal(await c.run("JSON.stringify(window.fallbackRequests)"), JSON.stringify([{ host: 'linux.do' }, { host: 'linux.do' }]));
  assert.equal(await revision(), rev + 1);
  await c.ev("document.querySelector('.inline-edit button[type=submit]').click()");
  assert.equal(await waitFor(c, "document.querySelector('li[data-item=backup-icon-test] .ic-img')?.naturalWidth === 64"), true);
  assert.equal(await revision(), rev + 2);
  console.log('PASS site icon: backup selection and upload both preview before saving');
} finally {
  site.close();
  await c.close();
}
