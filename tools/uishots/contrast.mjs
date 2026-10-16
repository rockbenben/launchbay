// 对比度量尺：把「看不见」变成一个数。
//   node uicontract.mjs <port> <cdp>
const SRV = 'http://127.0.0.1:' + (process.argv[2] || 7881);
const CDP = process.argv[3] || 'http://127.0.0.1:9222';
const tab = await (await fetch(`${CDP}/json/new?about:blank`, { method: 'PUT' })).json();
const ws = new WebSocket(tab.webSocketDebuggerUrl);
let id = 0; const pending = new Map();
ws.addEventListener('message', (e) => { const m = JSON.parse(e.data); if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); } });
const send = (m, p = {}) => new Promise((r) => { const i = ++id; pending.set(i, r); ws.send(JSON.stringify({ id: i, method: m, params: p })); });
const run = async (e) => { const r = await send('Runtime.evaluate', { expression: e, awaitPromise: true, returnByValue: true }); if (r.result?.exceptionDetails) return 'EXC'; return r.result?.result?.value; };
const sleep = (ms) => new Promise(r => setTimeout(r, ms));
await new Promise(r => ws.addEventListener('open', r, { once: true }));
await send('Page.enable'); await send('Runtime.enable');
await send('Emulation.setDeviceMetricsOverride', { width: 1440, height: 900, deviceScaleFactor: 1, mobile: false });

const CONTRAST = `(()=>{
  const lum = (rgb) => { const [r,g,b] = rgb.map(v => { v/=255; return v<=0.03928 ? v/12.92 : Math.pow((v+0.055)/1.055,2.4); }); return 0.2126*r+0.7152*g+0.0722*b; };
  const parse = (s) => { const m = s.match(/[\\d.]+/g); return m ? m.slice(0,3).map(Number) : null; };
  const ratio = (a,b) => { const A=lum(a), B=lum(b); return ((Math.max(A,B)+0.05)/(Math.min(A,B)+0.05)); };
  // 一层一层往上找**不透明**的背景色：元素自己的底可能是透明的
  const bgOf = (el) => { for (let n=el; n; n=n.parentElement) { const c=parse(getComputedStyle(n).backgroundColor); if (c && getComputedStyle(n).backgroundColor !== 'rgba(0, 0, 0, 0)') return c; } return [0,0,0]; };
  const rows = [];
  const add = (label, el) => { if (!el) { rows.push(label + ' → 没有'); return; }
    const fg = parse(getComputedStyle(el).color); const bg = bgOf(el);
    rows.push(label + ' 字 ' + getComputedStyle(el).color + ' / 底 rgb(' + bg + ') = ' + ratio(fg,bg).toFixed(2) + ':1'); };
  add('组标题 h2', document.querySelector('section h2'));
  add('卡片名字 b', document.querySelector('li[data-item] b'));
  add('卡片第二行 i', document.querySelector('li[data-item] i') || document.querySelector('li[data-item] .host'));
  add('排法键 .layoutbtn', document.querySelector('button.layoutbtn'));
  add('把手 .pencil', document.querySelector('button.pencil'));
  add('侧栏分组名', document.querySelector('a[href^="#g-"]'));
  add('搜索框占位符', document.querySelector('.search input'));
  return rows.join('\\n');
})()`;

for (const theme of ['dark', 'light']) {
  await send('Page.navigate', { url: SRV + '/' }); await sleep(700);
  await run(`fetch('/pin',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'profile=dad'})`);
  await run(`fetch('/prefs',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'theme=${theme}'})`);
  await send('Page.navigate', { url: SRV + '/' }); await sleep(1600);
  console.log('=== ' + theme + ' ===');
  console.log(await run(CONTRAST));
}
ws.close(); process.exit(0);
