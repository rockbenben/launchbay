// UI 走查拍摄：把每个页面在三个宽度 × 两套主题下拍出来，供人眼逐张看。
//   node uishots.mjs <port> <outDir> [theme]
import fs from 'node:fs';
const SRV = 'http://127.0.0.1:' + (process.argv[2] || 7881);
const OUT = process.argv[3] || 'design-preview/ui-shots';
fs.mkdirSync(OUT, { recursive: true });

const tab = await (await fetch('http://127.0.0.1:9222/json/new?about:blank', { method: 'PUT' })).json();
const ws = new WebSocket(tab.webSocketDebuggerUrl);
let id = 0; const pending = new Map();
ws.addEventListener('message', (e) => { const m = JSON.parse(e.data); if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); } });
const send = (m, p = {}) => new Promise((r) => { const i = ++id; pending.set(i, r); ws.send(JSON.stringify({ id: i, method: m, params: p })); });
const run = async (e) => { const r = await send('Runtime.evaluate', { expression: e, awaitPromise: true, returnByValue: true }); return r.result?.result?.value; };
const sleep = (ms) => new Promise(r => setTimeout(r, ms));
await new Promise(r => ws.addEventListener('open', r, { once: true }));
await send('Page.enable'); await send('Runtime.enable');

const sizes = [[1440, 900, 'w'], [700, 900, 'm'], [390, 844, 'n']];
const pages = [
  ['profile', '/'],
  ['switch', '/switch'],
  ['prefs-look', '/prefs'],
  ['prefs-groups', '/prefs?tab=groups'],
  ['revert', '/revert'],
  ['history', '/history'],
  ['import', '/import'],
  ['admin', '/admin'],
  ['adminlogin', '/admin/login'],
];
const themes = (process.argv[4] || 'dark,light').split(',');

async function shot(name) {
  const r = await send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: true });
  fs.writeFileSync(`${OUT}/${name}.png`, Buffer.from(r.result.data, 'base64'));
}
async function setTheme(t) {
  await run(`fetch('/prefs',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'theme=${t}'})`);
}
async function loginProfile() {
  await send('Page.navigate', { url: SRV + '/healthz' }); await sleep(300);
  await run(`fetch('/pin',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'profile=dad'})`);
}
async function loginAdmin() {
  await run(`fetch('/admin/login',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'key=ui-admin-key'})`);
}

for (const theme of themes) {
  await send('Page.navigate', { url: SRV + '/' }); await sleep(600);
  await loginProfile(); await loginAdmin();
  await setTheme(theme);
  for (const [w, h, tag] of sizes) {
    await send('Emulation.setDeviceMetricsOverride', { width: w, height: h, deviceScaleFactor: 1, mobile: w < 500 });
    for (const [name, url] of pages) {
      await send('Page.navigate', { url: SRV + url });
      await sleep(name.startsWith('profile') ? 1500 : 1000);
      await shot(`${theme}-${tag}-${name}`);
      process.stdout.write(`${theme}/${tag}/${name} `);
    }
    await loginProfile(); await loginAdmin();
  }
  process.stdout.write('\n');
}
console.log('\nshots in', OUT);
try { await fetch('http://127.0.0.1:9222/json/close/' + tab.id); } catch { /* ignore */ }
ws.close(); process.exit(0);
