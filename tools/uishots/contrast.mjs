// 对比度量尺：把「看不见」变成一个数。
//   node contrast.mjs <port> <cdp>
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
  const parseA = (s) => { const m = s.match(/[\\d.]+/g); if(!m) return null; return [Number(m[0]),Number(m[1]),Number(m[2]), m[3]===undefined?1:Number(m[3])]; };
  const ratio = (a,b) => { const A=lum(a), B=lum(b); return ((Math.max(A,B)+0.05)/(Math.min(A,B)+0.05)); };
  // **底色要一层层合成**（2026-10-21 修）。从前这一步是"往上找到第一个不透明的
  // backgroundColor 就用它"，而卡片那一层是半透明白（rgba(255,255,255,.055)）——
  // 于是它被当成了**纯白**：暗色主题里「卡片名字」被算成 1.17:1（真值 14.7:1），
  // 「把手」2.12:1（真值 8.1:1）。**一把量出假警报的尺子比没有尺子更糟**——
  // 人会照着 1.17:1 去"修"一个根本没坏的地方。
  // 合成：从元素往上收集半透明层，叠在**下面**那层（多半是 --bg）上。
  const bgOf = (el) => {
    const layers = [];
    for (let n = el; n; n = n.parentElement) {
      const c = parseA(getComputedStyle(n).backgroundColor);
      if (c && c[3] > 0) { layers.push(c); if (c[3] >= 1) break; }
    }
    let out = [0,0,0];
    for (let i = layers.length - 1; i >= 0; i--) {
      const [r,g,b,a] = layers[i];
      out = [r*a + out[0]*(1-a), g*a + out[1]*(1-a), b*a + out[2]*(1-a)];
    }
    return out;
  };
  // **元素的透明也算**：.pencil 静息时 opacity .7，它压到卡上的**有效**颜色才是
  // 真正被眼睛看到的那个。元素 opacity 不看会把它当成全不透明而虚报对比度。
  const effOf = (el, bg) => {
    const fg = parseA(getComputedStyle(el).color); if (!fg) return null;
    const op = parseFloat(getComputedStyle(el).opacity);
    const a = fg[3] * (isNaN(op) ? 1 : op);
    return [fg[0]*a + bg[0]*(1-a), fg[1]*a + bg[1]*(1-a), fg[2]*a + bg[2]*(1-a)];
  };
  const rows = [];
  const add = (label, el) => { if (!el) { rows.push(label + ' → 没有'); return; }
    const bg = bgOf(el); const fg = effOf(el, bg);
    if (!fg) { rows.push(label + ' → 量不出来'); return; }
    const op = parseFloat(getComputedStyle(el).opacity);
    // **静息不可见的东西要说明它是不可见**，而不是甩一个 1.00:1
    // （那看着像"这块坏了"，而真相是"精确指针那一档它指到才显形"）。
    if (!isNaN(op) && op === 0) { rows.push(label + ' → 静息不可见（opacity 0，指到/键盘走进这一行才显形）'); return; }
    rows.push(label + ' 字 rgb(' + fg.map(v=>Math.round(v)).join(',') + ') / 底 rgb(' + bg.map(v=>Math.round(v)).join(',') + ') = ' + ratio(fg,bg).toFixed(2) + ':1（opacity ' + op + '）'); };
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
