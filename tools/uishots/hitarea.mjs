// 命中区量尺：量真实的可点范围（含 ::before 扩展），不是盒子本身。
//   node hitarea.mjs <port> <cdp> [selector]
const SRV = 'http://127.0.0.1:' + (process.argv[2] || 7881);
const CDP = process.argv[3] || 'http://127.0.0.1:9222';
const SEL = process.argv[4] || 'button.layoutbtn';
const tab = await (await fetch(`${CDP}/json/new?about:blank`, { method: 'PUT' })).json();
const ws = new WebSocket(tab.webSocketDebuggerUrl);
let id = 0; const pending = new Map();
ws.addEventListener('message', (e) => { const m = JSON.parse(e.data); if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); } });
const send = (m, p = {}) => new Promise((r) => { const i = ++id; pending.set(i, r); ws.send(JSON.stringify({ id: i, method: m, params: p })); });
const run = async (e) => { const r = await send('Runtime.evaluate', { expression: e, awaitPromise: true, returnByValue: true }); if (r.result?.exceptionDetails) return 'EXC ' + (r.result.exceptionDetails.exception?.description||'').split('\n')[0]; return r.result?.result?.value; };
const sleep = (ms) => new Promise(r => setTimeout(r, ms));
await new Promise(r => ws.addEventListener('open', r, { once: true }));
await send('Page.enable'); await send('Runtime.enable');
await send('Emulation.setDeviceMetricsOverride', { width: 1440, height: 900, deviceScaleFactor: 1, mobile: false });
await send('Page.navigate', { url: SRV + '/healthz' }); await sleep(300);
await run(`fetch('/pin',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'profile=dad'})`);
await send('Page.navigate', { url: SRV + '/' }); await sleep(1800);

console.log(await run(`(()=>{
  const out = [];
  for (const sel of ${JSON.stringify([SEL, '#lb-add', '.pencil', 'button.card', '.search input'])}) {
    const b = document.querySelector(sel);
    if (!b) { out.push(sel + ' → 没有这个元素'); continue; }
    const r = b.getBoundingClientRect();
    const cx = r.left + r.width/2, cy = r.top + r.height/2;
    // 从中心往四个方向探，量到「点上去不是它」为止 = 真实可点范围。
    // **探到上限要说明是上限**（2026-10-21 修）：探针只走 80px，走到底就返回 80，
    // 两边相加报成「命中 160x137」——那个数真正的意思是"**两边都没探到边**"，
    // 而它看起来像"命中区有 160px 宽"。**一个越界的数字会被当成结论**，
    // 而它其实只说明"量不出来"。所以探到边就报边，探不到就报「≥」。
    const LIMIT = 80;
    const probe = (dx,dy) => { let d=0; while (d<LIMIT) { d+=1; const el=document.elementFromPoint(cx+dx*(d/4), cy+dy*(d/4)); if (el !== b && !b.contains(el)) return d; } return null; };
    const l = probe(-1,0), r2 = probe(1,0), u = probe(0,-1), d2 = probe(0,1);
    const side = (a,b2) => (a === null || b2 === null) ? '≥' + (a ?? LIMIT) + '/' + (b2 ?? LIMIT) : (a+b2);
    const s = getComputedStyle(b);
    const before = getComputedStyle(b, '::before');
    out.push(JSON.stringify({ sel, 盒子: Math.round(r.width)+'x'+Math.round(r.height),
      命中: side(l,r2) + 'x' + side(u,d2),
      命中说明: (l===null||r2===null||u===null||d2===null) ? '探针上限 80px：有一侧没探到边，那里只能报「≥」' : '四侧都探到了边',
      beforeInset: [before.top, before.right, before.bottom, before.left].join(','),
      色: s.color, 对比底: s.backgroundColor }));
  }
  return out.join('\\n');
})()`));
ws.close(); process.exit(0);
