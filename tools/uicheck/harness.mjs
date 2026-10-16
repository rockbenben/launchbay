// 交互门禁的共享设施（2026-10-16）。
//
// **两个横扫脚本共用这一份**：CDP 连接、屏上读数、以及那条不变量。
// 抄一份到第二个脚本里，就是这个项目刚花一轮消灭的那种东西——同一个判断两处各写
// 一份，于是其中一份覆盖不到某种形状（这里覆盖不到的会是"新脚本忘了核对空组那句"）。
import fs from 'node:fs';

// seedFixture 铺一份固定的档案：四组（磁贴/单列/大卡/空组）+ 两种点不动的外部地址。
// 走的是真的 API（建档 + batch），所以它同时验一遍"CLI 通路收得了这份数据"。
//
// **幂等**：两道横扫跑在同一份数据目录上，第二道再铺一遍会整批撞 duplicate-id。
// 判据是**盘上已经有条目了**（读一次 `/api/v1/data`，用档案作用域令牌），而不是
// "档案存在"——建档那一步允许已存在，铺数据那一步不允许。
export async function seedFixture(srv, adm, wrt) {
  const post = async (path, token, body) => {
    const r = await fetch(srv + path, { method: 'POST',
      headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json; charset=utf-8' },
      body: JSON.stringify(body) });
    return { status: r.status, body: await r.text() };
  };
  const p = await post('/api/v1/profiles', adm, { id: 'dad', title: 'Dad', nopass: true });
  if (p.status !== 201 && p.status !== 409) throw new Error('建档失败: ' + p.status + ' ' + p.body);

  const cur = await fetch(srv + '/api/v1/data?profile=dad', { headers: { Authorization: 'Bearer ' + wrt } });
  if (cur.ok) {
    const j = await cur.json();
    const n = (j.data?.groups || []).reduce((a, g) => a + (g.items?.length || 0), 0);
    if (n > 0) return; // 已经铺好了（第二道横扫）：不动它
  }
  // id 必须是 ASCII slug（model.ValidSlug），中文只能当名字。
  // **每种卡片形态都要有一份**：可点的卡、大卡、外部地址（点不动的行）。外部地址
  // 尤其重要——它是"脚本按 href 读地址"那套判据读不到形状，而编辑模态正好要读它。
  const groups = [
    { id: 'tiles-1', title: '磁贴组', layout: 'tiles', items: ['甲', '乙', '丙', '丁'] },
    { id: 'list-1', title: '单列组', layout: 'list', items: ['戊', '己', '庚'] },
    { id: 'feat-1', title: '大卡组', layout: 'feature', items: ['辛', '壬', '癸'] },
    { id: 'empty-1', title: '空组', layout: 'tiles', items: [] },
  ];
  const ops = [];
  for (const g of groups) {
    ops.push({ op: 'add', path: 'groups', value: { id: g.id, title: g.title, layout: g.layout } });
    g.items.forEach((n, i) => ops.push({ op: 'add', path: `groups/${g.id}/items`,
      value: { id: `it-${g.id}-${i + 1}`, name: n, url: `https://${g.id}.example.com/${i + 1}` } }));
  }
  ops.push({ op: 'add', path: 'groups/feat-1/items', value: { id: 'it-feat-1-ext', name: '浏览器设置', url: 'chrome://settings' } });
  ops.push({ op: 'add', path: 'groups/list-1/items', value: { id: 'it-list-1-ext', name: '本地说明', url: 'file:///d/readme.md' } });
  const r = await post('/api/v1/batch', wrt, { profile: 'dad', by: 'cli', ops });
  if (r.status !== 200) throw new Error('铺数据失败: ' + r.status + ' ' + r.body);
}

export async function connect(cdp) {  const tab = await (await fetch(`${cdp}/json/new?about:blank`, { method: 'PUT' })).json();
  const ws = new WebSocket(tab.webSocketDebuggerUrl);
  let id = 0;
  const pending = new Map();
  const errs = [];
  ws.addEventListener('message', (e) => {
    const m = JSON.parse(e.data);
    if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); }
    if (m.method === 'Runtime.exceptionThrown') errs.push(m.params.exceptionDetails.text);
  });
  const send = (method, params = {}) => new Promise((r) => {
    const i = ++id; pending.set(i, r); ws.send(JSON.stringify({ id: i, method, params }));
  });
  const run = async (expr) => {
    const r = await send('Runtime.evaluate', { expression: expr, awaitPromise: true, returnByValue: true });
    if (r.result?.exceptionDetails) return { __exc: (r.result.exceptionDetails.exception?.description || '').split('\n')[0] };
    return r.result?.result?.value;
  };
  // ev = 在页面里跑一段代码并把异常也带回来（驱动用；不抛，好让失败信息进日志）。
  const ev = (js) => run(`(()=>{try{${js}}catch(e){return 'THREW '+e.message}})()`);
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
  await new Promise((r) => ws.addEventListener('open', r, { once: true }));
  await send('Page.enable');
  await send('Runtime.enable');
  await send('Network.enable');
  const close = async () => {
    try { await fetch(`${cdp}/json/close/${tab.id}`); } catch { /* 关不掉就算了 */ }
    ws.close();
  };
  return { tab, ws, send, run, ev, sleep, errs, close };
}

// 屏上那一侧的读数。**容器认两种形状**（正文 section、大卡 ul）——判据与 app.js 的
// listOf 同一个形状，这一份是它的对照。
export const INV = `(()=>{const out=[];for(const host of document.querySelectorAll('[data-gid]')){const gid=host.dataset.gid;
const list=host.tagName==='UL'?host:host.querySelector(':scope > ul');
const items=list?[...list.querySelectorAll('li[data-item]')].map(n=>n.dataset.item):[];
const hint=host.tagName==='SECTION'?host.querySelector(':scope > p.grp-empty'):null;
out.push({gid,items,hint:hint?hint.hidden:null});}
const counts={};for(const a of document.querySelectorAll('a[href^="#g-"],a[href^="#f-"]')){const m=/^#([gf])-(.+?)(?:\\?|$)/.exec(a.getAttribute('href'));if(!m)continue;const b=a.querySelector('b.cnt');if(b)counts[m[2]]=Number(b.textContent||0);}
for(const h of document.querySelectorAll('h2.feat-g[data-group]')){const n=h.querySelector('.feat-n');if(n)counts[h.dataset.group]=Number(n.textContent||0);}
return JSON.stringify({groups:out,counts,
bad:[...document.querySelectorAll('#lb-banner.bad')].map(n=>n.textContent.trim()),
ok:[...document.querySelectorAll('#lb-banner.ok')].map(n=>n.textContent.trim())})})()`;

// diffScreen：屏上与盘上分家的地方，一处一行。
export function diffScreen(inv, d) {
  const bad = [];
  const byGid = new Map(inv.groups.map((g) => [g.gid, g]));
  for (const grp of d.groups) {
    const scr = byGid.get(grp.id);
    if (!scr) { bad.push(`组 ${grp.id} 在屏上不见了`); continue; }
    const want = grp.items.map((i) => i.id);
    if (JSON.stringify(scr.items) !== JSON.stringify(want)) bad.push(`${grp.id} 屏上 ${JSON.stringify(scr.items)} ≠ 盘上 ${JSON.stringify(want)}`);
    // 空组那句：有内容就该藏起来（hidden=true），空着就该露出来。
    if (scr.hint !== null && scr.hint !== (want.length > 0)) bad.push(`${grp.id} 空组那句 hidden=${scr.hint}，盘上 ${want.length} 条`);
    if (inv.counts[grp.id] !== undefined && inv.counts[grp.id] !== want.length) bad.push(`${grp.id} 屏上计数 ${inv.counts[grp.id]} ≠ 盘上 ${want.length}`);
  }
  for (const gid of byGid.keys()) if (!d.groups.find((g) => g.id === gid)) bad.push(`屏上多了一组 ${gid}`);
  return bad;
}

// makeStep 造一个"走一步、核对不变量"的函数。
//
// 每一步之前把界面摆回干净状态（浮层、多选、上一条横幅），**除非这一步自己要验
// 离线队列**（`keepQueue`）——那个族的前提就是队列里有东西在，清掉等于没测。
// 不留干净状态的话，上一步的残局会顺着传下去：多选还开着时下一步长按一颗卡，
// 多出来的那一颗也进了选中，于是这一步删掉了两条——症状落在下一步，病因在上一步。
export function makeStep({ c, srv, dataPath, say, stats, login }) {
  const disk = () => JSON.parse(fs.readFileSync(dataPath, 'utf8'));

  return async function step(label, act, opt = {}) {
    stats.ran++;
    await c.ev(`(()=>{ document.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true}));
      for(const s of document.querySelectorAll('.sheet')) if(s.id!=='lb-sheet' && !s.classList.contains('prefs')) s.remove();
      const a=document.getElementById('lb-sheet'); if(a) a.hidden=true;
      document.getElementById('lb-banner')?.remove();
      const bar=document.querySelector('.multibar'); if(bar) bar.hidden=true;
      document.querySelectorAll('li.sel-mark').forEach(n=>n.classList.remove('sel-mark'));
      ${opt.keepQueue ? '' : "localStorage.removeItem('lb-queue-dad');"}
      return 1 })()`);
    // 前置条件：**这一步开始时页面还得是档案页**。上一步把页面弄走时（点错了链接、
    // 脚本崩了）不拦住的话，后面每一步都在一张错误页上量，读数全是「组不见了」。
    if ((await c.run(`document.querySelectorAll('[data-gid]').length`)) === 0) {
      stats.fails++; stats.ran--;
      say(`FAIL ${label}   [前置：上一步把页面弄走了，这一步量的是空页——先看上一条]`);
      await c.send('Page.navigate', { url: srv + '/' }); await c.sleep(1200);
      if (login) await login();
      await c.send('Page.navigate', { url: srv + '/' }); await c.sleep(1400);
      return;
    }
    const before = disk();
    await c.run(`sessionStorage.setItem('__sweep','1')`);
    let res = null;
    try { res = await act(); } catch (e) { res = 'THREW ' + e.message; }
    await c.sleep(opt.wait || 1300);
    const reloaded = (await c.run(`sessionStorage.getItem('__sweep')||''`)) === '';
    await c.run(`sessionStorage.removeItem('__sweep')`);
    const raw = await c.run(INV);
    const problems = [];
    if (raw && raw.__exc) problems.push('不变量读不出来：' + raw.__exc);
    const inv = raw && !raw.__exc ? JSON.parse(raw) : null;
    if (inv && !opt.noInvariant) problems.push(...diffScreen(inv, disk()));
    if (!opt.allowBad && !opt.expectBad && inv && inv.bad.length) problems.push('失败横幅没解释：' + JSON.stringify(inv.bad));
    if (opt.expectBad && (!inv || !inv.bad.length)) problems.push('该给一句失败说明的没给');
    if (opt.inPlace && reloaded) problems.push('整页重载了（这一步要求就地完成）');
    if (opt.outcome) problems.push(...(opt.outcome(inv, disk(), res, before) || []));
    if (problems.length) {
      stats.fails++;
      say(`FAIL ${label}   [act=${res}]`);
      for (const p of problems) say('     · ' + p);
    } else {
      say(`PASS ${label}`);
    }
  };
}
