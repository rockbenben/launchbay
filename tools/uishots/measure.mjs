// 走查量尺：把肉眼看到的几处拿去量。node measure.mjs <port> <cdp>
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
await send('Page.navigate', { url: SRV + '/healthz' }); await sleep(300);
await run(`fetch('/pin',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'profile=dad'})`);
await send('Page.navigate', { url: SRV + '/' }); await sleep(1800);

console.log('① 页面高度 / 视口：', await run(`document.documentElement.scrollHeight + ' / ' + window.innerHeight`));
console.log('② 正文里每一组（屏上）：', await run(`JSON.stringify([...document.querySelectorAll('[data-gid]')].map(h=>h.dataset.gid+'('+h.querySelectorAll('li[data-item]').length+'条,hint='+(h.querySelector(':scope > p.grp-empty')?.hidden)+')'))`));
console.log('③ 侧栏那几格：', await run(`JSON.stringify([...document.querySelectorAll('a[href^="#g-"], a[href^="#f-"]')].map(a=>{const r=a.getBoundingClientRect();return {t:a.textContent.trim().replace(/\\s+/g,' '),色:getComputedStyle(a).getPropertyValue('--sec-accent').trim()||'（无）',色点:a.querySelector('[style*=--sec-accent]')?'有':'无'}})().slice(0,6))`));
console.log('④ 名字就是地址的卡片：', await run(`(()=>{const all=[...document.querySelectorAll('li[data-item] b')];const bad=all.filter(b=>/^https?:\\/\\//.test(b.textContent.trim()));return bad.length+' / '+all.length})()`));
console.log('⑤ 排法切换键（.layoutbtn）看得见吗：', await run(`(()=>{const b=document.querySelector('button.layoutbtn'); if(!b) return 'NO-BTN'; const s=getComputedStyle(b), r=b.getBoundingClientRect(); return JSON.stringify({字:b.textContent,色:s.color,背景:s.backgroundColor,字号:s.fontSize,尺寸:Math.round(r.width)+'x'+Math.round(r.height),不透明:s.opacity})})()`));
console.log('⑥ 卡片里第二行（说明/主机名）是否存在：', await run(`(()=>{const cards=[...document.querySelectorAll('li[data-item]')].filter(li=>li.querySelector('.card, .plain'));const withSub=cards.filter(li=>li.querySelector('i, .host, .addr code'));return withSub.length+' / '+cards.length})()`));
console.log('⑦ 顶栏各件的高度：', await run(`JSON.stringify([...document.querySelectorAll('header *, .topbar *')].filter(n=>n.children.length===0&&n.textContent.trim()).slice(0,8).map(n=>n.tagName+':'+Math.round(n.getBoundingClientRect().height)))`));
console.log('⑧ 一屏能放下几张卡（第一屏）：', await run(`(()=>{const cards=[...document.querySelectorAll('li[data-item]')];let n=0;for(const c of cards){if(c.getBoundingClientRect().top<window.innerHeight)n++;else break}return n+' / '+cards.length})()`));
ws.close(); process.exit(0);
