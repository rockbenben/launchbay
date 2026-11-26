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
const refresh = () => c.ev("document.querySelector('.inline-edit .edit-icon-row button').click()");

try {
  const seeded = await fetch(srv + '/api/v1/batch', {
    method: 'POST', headers,
    body: JSON.stringify({
      profile: 'dad', by: 'cli',
      ops: [{ op: 'add', path: 'groups/tiles-1/items',
        value: { id: 'site-icon-test', name: 'Site icon fixture', url: address } }],
    }),
  });
  assert.equal(seeded.status, 200, await seeded.text());
  const login = () => c.run("fetch('/pin',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'profile=dad'}).then(r=>r.status)");
  await goHome(c, srv, login);
  await c.ev("document.querySelector('li[data-item=site-icon-test] button.pencil').click()");
  await changeIcon('lb-edit-icon');
  await c.ev("document.querySelector('.inline-edit button[type=submit]').click()");
  assert.equal(await waitFor(c, `document.querySelector('${img}')?.naturalWidth > 0`), true);
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

  await c.ev(`document.getElementById('lb-add').click();
    document.getElementById('lb-url').value = ${JSON.stringify(address + '/another')};`);
  await changeIcon('lb-icon');
  assert.equal(await waitFor(c, "document.querySelector('#lb-sheet .site-icon-status img')?.naturalWidth > 0"), true);
  console.log('PASS site icon: selection fetches, refresh updates without saving, failure retains the cached icon');
} finally {
  site.close();
  await c.close();
}
