// 补拍选择页：它不是 GET /switch（那条只收 POST），而是 POST /switch 之后落到的那一屏。
//   node picker.mjs <port> <outDir> <cdp>
import fs from 'node:fs';
const SRV = 'http://127.0.0.1:' + (process.argv[2] || 7881);
const OUT = process.argv[3] || 'design-preview/ui-picker';
const CDP = process.argv[4] || 'http://127.0.0.1:9222';
fs.mkdirSync(OUT, { recursive: true });

const tab = await (await fetch(`${CDP}/json/new?about:blank`, { method: 'PUT' })).json();
const ws = new WebSocket(tab.webSocketDebuggerUrl);
let id = 0; const pending = new Map();
ws.addEventListener('message', (e) => { const m = JSON.parse(e.data); if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); } });
const send = (m, p = {}) => new Promise((r) => { const i = ++id; pending.set(i, r); ws.send(JSON.stringify({ id: i, method: m, params: p })); });
const run = async (e) => { const r = await send('Runtime.evaluate', { expression: e, awaitPromise: true, returnByValue: true }); if (r.result?.exceptionDetails) return 'EXC'; return r.result?.result?.value; };
const sleep = (ms) => new Promise(r => setTimeout(r, ms));
await new Promise(r => ws.addEventListener('open', r, { once: true }));
await send('Page.enable'); await send('Runtime.enable');
const shot = async (n) => { const r = await send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: true }); fs.writeFileSync(`${OUT}/${n}.png`, Buffer.from(r.result.data, 'base64')); };

for (const [w, h, tag] of [[1440, 900, 'w'], [390, 844, 'n']]) {
  await send('Emulation.setDeviceMetricsOverride', { width: w, height: h, deviceScaleFactor: 1, mobile: w < 500 });
  for (const theme of ['dark', 'light']) {
    await send('Page.navigate', { url: SRV + '/' }); await sleep(600);
    await run(`fetch('/pin',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'profile=dad'})`);
    await run(`fetch('/prefs',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'theme=${theme}'})`);
    // 回选择页：POST /switch 清掉会话后 303 到 /。**fetch 不会导航**——它只发请求，
    // 屏上还是刚才那一页，于是拍出来的是档案页而不是选择页（第一次拍就是这么错的）。
    // 选择页是"没有会话时的那一屏"：清掉 cookie 再导航。
    // POST /switch 不导航（fetch 只发请求），而它自己也只是回 "/"——拍到的是旧页面。
    await send('Network.enable');
    await send('Network.clearBrowserCookies');
    await send('Page.navigate', { url: SRV + '/' });
    await sleep(1400);
    const url = await run(`location.pathname`);
    await shot(`${theme}-${tag}-picker`);
    console.log(`${theme}/${tag} → ${url} · 有卡片吗=${await run(`!!document.querySelector('.pick, .picker, form[action="/pin"]')`)}`);
  }
}
try { await fetch(`${CDP}/json/close/` + tab.id); } catch { /* ignore */ }
ws.close(); process.exit(0);
