// 档案页交互横扫（2026-10-16）。**这是仓库里唯一一条会真的点界面的门禁**。
//
// 为什么是「不变量」而不是逐条断言：这个项目的 bug 形状几乎都是同一句——
// **屏上说的与盘上存的分家**（顺序错、存错组、那句话没跟着变、计数没跟着变）。
// 逐条断言只能钉住已经想得到的那几种；不变量把「每一次写之后屏上必须与盘上一致」
// 变成一次机械核对，于是没想到的那些也会在这一步红。
//
// 跑法（平时用 Go 那道闸，它负责起服务与浏览器）：
//   LB_UI=1 go test ./tools/uicheck/
// 直接跑（本机已经有一个开着调试口的无头 Chrome 时）：
//   node tools/uicheck/sweep.mjs <port> <dataDir> [cdpBase]
// 数据目录要**空**：脚本自己用 API 建档并铺一份固定的分组（大卡/磁贴/单列/空组 +
// 两种点不动的外部地址）。
import fs from 'node:fs';
import path from 'node:path';

const PORT = process.argv[2] || '';
const DIR = path.resolve(process.argv[3] || 'labdata-sweep');
const CDP = process.argv[4] || 'http://127.0.0.1:9222';
const SRV = `http://127.0.0.1:${PORT}`;
const DATA = path.join(DIR, 'profiles', 'dad', 'bookmarks.json');
const ADM = process.env.LB_UI_ADMIN || '';
const WRT = process.env.LB_UI_WRITE || '';
if (!ADM || !WRT) {
  // 令牌**由 Go 那道闸通过环境变量发过来**（它同时把同样的值发给被测服务）：
  // 两边各写一份字面量，就等着下一次只改一边。
  throw new Error('缺 LB_UI_ADMIN / LB_UI_WRITE：直接跑这个脚本时自己设上（要与被测服务的令牌一致）');
}
let fails = 0, ran = 0;
const say = (s) => process.stdout.write(s + '\n');
const disk = () => JSON.parse(fs.readFileSync(DATA, 'utf8'));

// ── 铺一份固定的分组 ───────────────────────────────────────────────────────
const post = async (path, token, body) => {
  const r = await fetch(SRV + path, { method: 'POST',
    headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json; charset=utf-8' },
    body: JSON.stringify(body) });
  return { status: r.status, body: await r.text() };
};
{
  const p = await post('/api/v1/profiles', ADM, { id: 'dad', title: 'Dad', nopass: true });
  if (p.status !== 201 && p.status !== 409) throw new Error('建档失败: ' + p.status + ' ' + p.body);
}
{
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
  // 外部地址两行：大卡带里一条 chrome://、正文里一条 file://
  ops.push({ op: 'add', path: 'groups/feat-1/items', value: { id: 'it-feat-1-ext', name: '浏览器设置', url: 'chrome://settings' } });
  ops.push({ op: 'add', path: 'groups/list-1/items', value: { id: 'it-list-1-ext', name: '本地说明', url: 'file:///d/readme.md' } });
  const r = await post('/api/v1/batch', WRT, { profile: 'dad', by: 'cli', ops });
  if (r.status !== 200) throw new Error('铺数据失败: ' + r.status + ' ' + r.body);
}

// ── CDP ────────────────────────────────────────────────────────────────────
const tab = await (await fetch(CDP + '/json/new?about:blank', { method: 'PUT' })).json();
const ws = new WebSocket(tab.webSocketDebuggerUrl);
let id = 0; const pending = new Map(); const errs = [];
ws.addEventListener('message', (e) => { const m = JSON.parse(e.data);
  if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); }
  if (m.method === 'Runtime.exceptionThrown') errs.push(m.params.exceptionDetails.text); });
const send = (m, p = {}) => new Promise((r) => { const i = ++id; pending.set(i, r); ws.send(JSON.stringify({ id: i, method: m, params: p })); });
const run = async (e) => { const r = await send('Runtime.evaluate', { expression: e, awaitPromise: true, returnByValue: true });
  if (r.result?.exceptionDetails) return { __exc: (r.result.exceptionDetails.exception?.description || '').split('\n')[0] };
  return r.result?.result?.value; };
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
await new Promise((r) => ws.addEventListener('open', r, { once: true }));
await send('Page.enable'); await send('Runtime.enable');
await send('Emulation.setDeviceMetricsOverride', { width: 1500, height: 1300, deviceScaleFactor: 1, mobile: false });
await send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-reduced-motion', value: 'no-preference' }] });

// 屏上那一侧的读数。**容器认两种形状**（正文 section、大卡 ul）——判据与 app.js 的
// listOf 同一个形状，这一份是它的对照。
const INV = `(()=>{const out=[];for(const host of document.querySelectorAll('[data-gid]')){const gid=host.dataset.gid;
const list=host.tagName==='UL'?host:host.querySelector(':scope > ul');
const items=list?[...list.querySelectorAll('li[data-item]')].map(n=>n.dataset.item):[];
const hint=host.tagName==='SECTION'?host.querySelector(':scope > p.grp-empty'):null;
out.push({gid,items,hint:hint?hint.hidden:null});}
const counts={};for(const a of document.querySelectorAll('a[href^="#g-"],a[href^="#f-"]')){const m=/^#([gf])-(.+?)(?:\\?|$)/.exec(a.getAttribute('href'));if(!m)continue;const b=a.querySelector('b.cnt');if(b)counts[m[2]]=Number(b.textContent||0);}
for(const h of document.querySelectorAll('h2.feat-g[data-group]')){const n=h.querySelector('.feat-n');if(n)counts[h.dataset.group]=Number(n.textContent||0);}
return JSON.stringify({groups:out,counts,
bad:[...document.querySelectorAll('#lb-banner.bad')].map(n=>n.textContent.trim()),
ok:[...document.querySelectorAll('#lb-banner.ok')].map(n=>n.textContent.trim())})})()`;

function diff(inv, d) {
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

// 每一步之前把界面摆回干净状态（浮层、多选、上一条横幅），之后核对同一条不变量。
// **不留干净状态**的话，上一步的残局会顺着传下去：多选还开着时下一步长按一颗卡，
// 多出来的那一颗也进了选中，于是这一步删掉了两条——症状落在下一步，病因在上一步。
async function step(label, act, opt = {}) {
  ran++;
  // **只删「用完即弃」的那些对话框**：`#lb-sheet`（添加浮层）与设置面板的 class 也叫
  // `.sheet`，它们是**页面级单例**，删掉之后 app.js 的 openSheet 再也点不动——
  // 于是后面每一步都报「找不到按钮」，一条真信号被八步噪声埋掉。
  await run(`(()=>{ document.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true}));
    for(const s of document.querySelectorAll('.sheet')) if(s.id!=='lb-sheet' && !s.classList.contains('prefs')) s.remove();
    const a=document.getElementById('lb-sheet'); if(a) a.hidden=true;
    document.getElementById('lb-banner')?.remove();
    const bar=document.querySelector('.multibar'); if(bar) bar.hidden=true;
    document.querySelectorAll('li.sel-mark').forEach(n=>n.classList.remove('sel-mark'));
    localStorage.removeItem('lb-queue-dad'); return 1 })()`);
  // 前置条件：**这一步开始时页面还得是档案页**。上一步把页面弄走时（点错了链接、
  // 脚本崩了）不拦住的话，后面每一步都在一张错误页上量，读数全是「组不见了」。
  if ((await run(`document.querySelectorAll('[data-gid]').length`)) === 0) {
    fails++; ran++;
    say(`FAIL ${label}   [前置：上一步把页面弄走了，这一步量的是空页——先看上一条]`);
    await send('Page.navigate', { url: SRV + '/' }); await sleep(1400);
    await run(`fetch('/pin',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'profile=dad'}).then(r=>r.status)`);
    await send('Page.navigate', { url: SRV + '/' }); await sleep(1400);
    return;
  }
  const before = disk();
  await run(`sessionStorage.setItem('__sweep','1')`);
  let res = null;
  try { res = await act(); } catch (e) { res = 'THREW ' + e.message; }
  await sleep(opt.wait || 1300);
  const reloaded = (await run(`sessionStorage.getItem('__sweep')||''`)) === '';
  await run(`sessionStorage.removeItem('__sweep')`);
  const raw = await run(INV);
  const problems = [];
  if (raw && raw.__exc) problems.push('不变量读不出来：' + raw.__exc);
  const inv = raw && !raw.__exc ? JSON.parse(raw) : null;
  if (inv) problems.push(...diff(inv, disk()));
  if (!opt.allowBad && !opt.expectBad && inv && inv.bad.length) problems.push('失败横幅没解释：' + JSON.stringify(inv.bad));
  if (opt.expectBad && (!inv || !inv.bad.length)) problems.push('该给一句失败说明的没给');
  if (opt.inPlace && reloaded) problems.push('整页重载了（这一步要求就地完成）');
  if (opt.outcome) problems.push(...(opt.outcome(inv, disk(), res, before) || []));
  if (problems.length) {
    fails++;
    say(`FAIL ${label}   [act=${res}]`);
    for (const p of problems) say('     · ' + p);
  } else {
    say(`PASS ${label}`);
  }
}

// ── 动作小工具 ─────────────────────────────────────────────────────────────
const ev = (js) => run(`(()=>{try{${js}}catch(e){return 'THREW '+e.message}})()`);
const dragInternal = (srcSel, dstSel, where) => ev(`const dt=new DataTransfer();
  const src=document.querySelector(${JSON.stringify(srcSel)}); if(!src) return 'NO-SRC';
  const d1=new DragEvent('dragstart',{bubbles:true,cancelable:true});
  Object.defineProperty(d1,'dataTransfer',{value:dt}); src.dispatchEvent(d1);
  const dst=document.querySelector(${JSON.stringify(dstSel)}); if(!dst) return 'NO-DST';
  const r=dst.getBoundingClientRect(); const x=Math.round(r.left+r.width/2);
  const y=${where === 'tail' ? 'r.bottom-2' : 'r.top+3'};
  for (const t of ['dragover','drop']) { const e=new DragEvent(t,{bubbles:true,cancelable:true});
    Object.defineProperty(e,'dataTransfer',{value:dt});
    Object.defineProperty(e,'clientY',{value:eval(y)}); Object.defineProperty(e,'clientX',{value:x});
    dst.dispatchEvent(e); }
  return 'dropped'`);
const dropURL = (url, sel) => ev(`const dt=new DataTransfer();
  dt.setData('text/uri-list',${JSON.stringify(url)}); dt.setData('text/plain',${JSON.stringify(url)});
  const t=document.querySelector(${JSON.stringify(sel)}); if(!t) return 'NO-TARGET';
  const r=t.getBoundingClientRect();
  const e=new DragEvent('drop',{bubbles:true,cancelable:true});
  Object.defineProperty(e,'dataTransfer',{value:dt});
  Object.defineProperty(e,'clientY',{value:Math.round(r.top+r.height/2)});
  Object.defineProperty(e,'clientX',{value:Math.round(r.left+r.width/2)});
  t.dispatchEvent(e); return 'dropped'`);
const firstIn = (gid) => `[data-gid="${gid}"] li[data-item]`;
const nthIn = (gid, n) => `[data-gid="${gid}"] li[data-item]:nth-of-type(${n})`;
const count = (d, gid) => d.groups.find((g) => g.id === gid).items.length;
// 每条 outcome 都按**前后差**写，不写死数字：写死的数字在前面某一步失败时会连环报错，
// 于是「哪一步真的坏了」被淹没——上一次横扫就是这么把 1 个真信号变成 24 条红的。
const grew = (gid) => (inv, d, res, before) => count(d, gid) !== count(before, gid) + 1 ? [`${gid} 条数 ${count(before, gid)} → ${count(d, gid)}，要 +1`] : [];
const shrank = (gid) => (inv, d, res, before) => count(d, gid) !== count(before, gid) - 1 ? [`${gid} 条数 ${count(before, gid)} → ${count(d, gid)}，要 -1`] : [];
const idsOf = (d, gid) => d.groups.find((g) => g.id === gid).items.map((i) => i.id);
// 排序不只要求「换了顺序」，还要**长度不变、集合相同**（排序是搬运不是增删）。
const movedWithin = (gid, before, d, changed) => {
  const a = idsOf(before, gid), b = idsOf(d, gid);
  if (a.length !== b.length) return [`${gid} 条数变了：${a.length} → ${b.length}`];
  if (JSON.stringify([...a].sort()) !== JSON.stringify([...b].sort())) return [`${gid} 的条目集合变了`];
  return changed(a, b) ? [] : [`${gid} 的次序没变：${JSON.stringify(b)}`];
};
// 跨组：搬过去的那一条要落在**指定的那个下标**上（判据是"落在哪"，不是"首尾变了"）
const crossMoved = (before, d, src, dst, wantIndex) => {
  const a = idsOf(before, src), b = idsOf(d, src);
  if (b.length !== a.length - 1) return [`源组 ${src} ${a.length} → ${b.length}，要 -1`];
  const moved = a.filter((x) => !b.includes(x));
  if (moved.length !== 1) return [`源组少了 ${moved.length} 条，要 1 条`];
  const e = idsOf(d, dst);
  const at = e.indexOf(moved[0]);
  if (at < 0) return [`搬过去的那一条不在 ${dst} 里`];
  if (at !== wantIndex) return [`落在第 ${at} 位，要第 ${wantIndex} 位：${JSON.stringify(e)}`];
  return [];
};

// ── 场景 ───────────────────────────────────────────────────────────────────
await send('Page.navigate', { url: SRV + '/healthz' }); await sleep(300);
await run(`fetch('/pin',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'profile=dad'}).then(r=>r.status)`);
await send('Page.navigate', { url: SRV + '/' }); await sleep(1400);
await run(`(()=>{localStorage.removeItem('lb-queue-dad');return 1})()`);

const addSheet = async (url, title, desc, gid, newGroup) => {
  await ev(`document.getElementById('lb-add').click()`); await sleep(300);
  await ev(`const u=document.getElementById('lb-url'); u.value=${JSON.stringify(url)};
    u.dispatchEvent(new Event('input',{bubbles:true})); u.dispatchEvent(new Event('blur',{bubbles:true}))`);
  await sleep(1500); // resolve 读标题会覆盖标题那格，所以先让它跑完
  await ev(`document.getElementById('lb-title').value=${JSON.stringify(title || '')};`);
  await ev(`document.getElementById('lb-desc').value=${JSON.stringify(desc || '')};`);
  if (newGroup) {
    await ev(`const g=document.getElementById('lb-group'); g.value=g.querySelector('option:last-child').value;
      g.dispatchEvent(new Event('change',{bubbles:true}));
      const n=document.getElementById('lb-newgroup'); n.value=${JSON.stringify(newGroup)};
      n.dispatchEvent(new Event('input',{bubbles:true}));`);
  } else {
    await ev(`const g=document.getElementById('lb-group'); g.value=${JSON.stringify(gid)};
      g.dispatchEvent(new Event('change',{bubbles:true}));`);
  }
  await sleep(200);
  return ev(`document.querySelector('#lb-sheet button[type=submit]').click(); return 'submitted'`);
};
// 编辑模态是 `<form class="sheet inline-edit">`（**没有 id**，只有里面的 h2 有），
// 三个输入按 name 定位：name / desc / url。
const openEdit = (sel) => ev(`const li=document.querySelector(${JSON.stringify(sel)}); if(!li) return 'NO-LI';
  const p=li.querySelector('button.pencil'); if(!p) return 'NO-PENCIL'; p.click(); return 'opened'`);
const editField = (name, value) => ev(`const i=document.querySelector('.inline-edit input[name=${JSON.stringify(name)}]');
  if(!i) return 'NO-FIELD:'+[...document.querySelectorAll('.inline-edit input')].map(x=>x.name).join('|');
  i.value=${JSON.stringify(value)}; i.dispatchEvent(new Event('input',{bubbles:true})); return 'filled'`);
const editSave = () => ev(`const b=document.querySelector('.inline-edit button[type=submit]'); if(!b) return 'NO-SAVE'; b.click(); return 'saved'`);
// **当前最上面那一层「用完即弃」的对话框**：`#lb-sheet`（添加浮层）也是 `.sheet`，
// 而且它挂在 body 上比任何动态对话框都早——`document.querySelector('.sheet input')`
// 命中的是**添加浮层的地址格**，于是标签那一步把标签打进了地址框，而应用那边如实地说
// 「标签是空的，先写一个」（那一刻它是对的，错的是我的驱动）。
const topDialog = () => ev(`const ds=[...document.querySelectorAll('.sheet')].filter(n=>n.id!=='lb-sheet');
  if(!ds.length) return null; const d=ds[ds.length-1]; window.__dlg=d; return d.id||d.className`);
const editDelete = async () => {
  const r1 = await ev(`const b=document.querySelector('.inline-edit button.danger'); if(!b) return 'NO-DEL'; b.click(); return 'asked'`);
  await sleep(400);
  // 「确定删掉「X」？再按一次就删」——**同一颗键按第二次**，不是另一颗确认键。
  const r2 = await ev(`const b=document.querySelector('.inline-edit button.danger'); if(!b) return 'NO-CONFIRM'; b.click(); return 'confirmed'`);
  return r1 + ' / ' + r2;
};

let n = 0;
const U = () => `https://sweep.example.com/u${Date.now().toString(36)}${n++}`;

// 1-4) 添加浮层
await step('添加浮层 → 磁贴组', () => addSheet(U(), '扫的第一条', '说明一句', 'tiles-1'), { wait: 2500, outcome: grew('tiles-1') });
await step('添加浮层 → 大卡组', () => addSheet(U(), '扫的第二条', '', 'feat-1'), { wait: 2500, outcome: grew('feat-1') });
await step('添加浮层 → 空组', () => addSheet(U(), '扫进空组的', '', 'empty-1'), { wait: 2500, outcome: grew('empty-1') });
await step('添加浮层 → 顺带建一个新分组', () => addSheet(U(), '扫建组的', '', null, '扫出来的一组'), { wait: 2500,
  outcome: (inv, d, res, before) => {
    const g = d.groups.find((x) => x.title === '扫出来的一组');
    if (!g) return ['盘上没有那个新分组'];
    return g.items.length === 1 ? [] : [`新分组里有 ${g.items.length} 条，要 1 条`];
  } });

// 5-8) 编辑模态
await step('编辑 → 改名字', async () => {
  await openEdit(firstIn('tiles-1')); await sleep(400);
  const r = await editField('name', '扫改过的名字');
  const s = await editSave();
  return r + ' / ' + s;
}, { wait: 2200, outcome: (inv, d) => d.groups.find((g) => g.id === 'tiles-1').items.some((i) => i.name === '扫改过的名字') ? [] : ['名字没写上'] });

await step('编辑 → 改描述', async () => {
  await openEdit(firstIn('tiles-1')); await sleep(400);
  const r = await editField('desc', '扫改过的说明');
  const s = await editSave();
  return r + ' / ' + s;
}, { wait: 2200, outcome: (inv, d) => d.groups.find((g) => g.id === 'tiles-1').items.some((i) => i.desc === '扫改过的说明') ? [] : ['描述没写上'] });

await step('编辑 → 从图标库换图标', async () => {
  await openEdit(firstIn('list-1')); await sleep(400);
  const r1 = await ev(`const b=[...document.querySelectorAll('.inline-edit button')].find(x=>x.textContent.includes('图标库'));
    if(!b) return 'NO-LIB:'+[...document.querySelectorAll('.inline-edit button')].map(x=>x.textContent.trim()).join('|');
    b.click(); return 'opened'`);
  await sleep(1000);
  await topDialog();
  // 打开时浮层里是「这一份已经在用的图标」（克隆卡片上的 SVG，零请求），**方格要搜了才有**：
  // 先在这个输入框里敲一个词，才发那次 /api/v1/icons/tiles。
  const r2 = await ev(`const d=window.__dlg; if(!d) return 'NO-DIALOG';
    const i=d.querySelector('input[type=search], input'); if(!i) return 'NO-INPUT';
    i.focus(); i.value='git'; i.dispatchEvent(new Event('input',{bubbles:true}));
    return 'typed'`);
  await sleep(1400);
  const r3 = await ev(`const i=document.querySelector('.icpick-tile'); if(!i) return 'NO-TILE';
    const n=i.dataset.icon; i.click(); return 'picked ' + n`);
  await sleep(400);
  await editSave();
  return r1 + ' / ' + r2 + ' / ' + r3;
}, { wait: 2500, outcome: (inv, d) => d.groups.find((g) => g.id === 'list-1').items.some((i) => i.icon && i.icon !== 'auto') ? [] : ['图标没换上去'] });

await step('编辑 → 改分组', async () => {
  await openEdit(firstIn('list-1')); await sleep(400);
  await ev(`const g=document.getElementById('lb-edit-group'); if(!g) return 'NO-GS';
    g.value='tiles-1'; g.dispatchEvent(new Event('change',{bubbles:true})); return 'picked'`);
  const s = await editSave();
  return s;
}, { wait: 2200, outcome: (inv, d, res, before) => {
  return count(d, 'list-1') !== count(before, 'list-1') - 1 || count(d, 'tiles-1') !== count(before, 'tiles-1') + 1
    ? [`源 ${count(before, 'list-1')}→${count(d, 'list-1')}，目标 ${count(before, 'tiles-1')}→${count(d, 'tiles-1')}`] : [];
} });

await step('编辑 → 删掉这一条', async () => {
  await openEdit(firstIn('tiles-1')); await sleep(400);
  return editDelete();
}, { wait: 2200, outcome: shrank('tiles-1') });

// 外部地址（点不动的行）：编辑模态**要能读出地址**。这一格从前按 `a[href]` 读，
// 而这种行没有 href（正文那一支的地址在 <code>、大卡带那一支两样都没有）——
// 读成空串就会照发一条 `set …/url=""`，整批 422，屏上只说「这一条没存上」。
await step('编辑 → 大卡带里的外部地址（不丢地址）', async () => {
  await openEdit('[data-gid="feat-1"] li[data-item="it-feat-1-ext"]'); await sleep(400);
  const shown = await ev(`const i=document.querySelector('.inline-edit input[name=url]'); if(!i) return 'NO-FIELD'; return i.value`);
  await editSave();
  return '地址格显示=' + JSON.stringify(shown);
}, { wait: 2200, outcome: (inv, d) => {
  const it = d.groups.flatMap((g) => g.items).find((i) => i.id === 'it-feat-1-ext');
  return it && it.url === 'chrome://settings' ? [] : [`地址被写成了 ${JSON.stringify(it && it.url)}`];
} });

await step('编辑 → 正文里的外部地址（不丢地址）', async () => {
  await openEdit('[data-gid="list-1"] li[data-item="it-list-1-ext"]'); await sleep(400);
  const shown = await ev(`const i=document.querySelector('.inline-edit input[name=url]'); if(!i) return 'NO-FIELD'; return i.value`);
  await editSave();
  return '地址格显示=' + JSON.stringify(shown);
}, { wait: 2200, outcome: (inv, d) => {
  const it = d.groups.flatMap((g) => g.items).find((i) => i.id === 'it-list-1-ext');
  return it && it.url === 'file:///d/readme.md' ? [] : [`地址被写成了 ${JSON.stringify(it && it.url)}`];
} });

// 9-13) 拖网址进来
await step('拖网址 → 磁贴组的一张卡上', () => dropURL(U(), `${firstIn('tiles-1')} .card`), { wait: 4500, inPlace: true, outcome: grew('tiles-1') });
await step('拖网址 → 组标题上', () => dropURL(U(), 'h2[data-group="list-1"]'), { wait: 4500, inPlace: true, outcome: grew('list-1') });
await step('拖网址 → 大卡组的一张卡上', () => dropURL(U(), '[data-gid="feat-1"] li[data-item] .feat-card'), { wait: 4500, inPlace: true, outcome: grew('feat-1') });
await step('拖网址 → 已经有的地址', async () => {
  const href = await run(`(document.querySelector('${firstIn('tiles-1')} a.card')||{}).href||''`);
  return dropURL(href, `${firstIn('tiles-1')} .card`);
}, { wait: 2500, expectBad: true,
  outcome: (inv, d, res, before) => count(d, 'tiles-1') !== count(before, 'tiles-1') ? ['重复的那次反而存进去了'] : [] });
await step('拖网址 → 空白处（退到页面上第一组）', () => dropURL(U(), 'main'), { wait: 4500,
  outcome: (inv, d, res, before) => {
    // 「第一组」按**屏上**次序判（大卡带那一组就在最前面），不按盘上数组次序。
    const first = inv.groups[0] && inv.groups[0].gid;
    return count(d, first) !== count(before, first) + 1 ? [`屏上第一组是 ${first}，它没多那一条`] : [];
  } });

// 14-18) 站内排序
await step('排序 → 同组插到某一条前面', () => dragInternal(`${firstIn('tiles-1')} .card`, nthIn('tiles-1', 3), 'head'), { wait: 2000, inPlace: true,
  outcome: (inv, d, res, before) => movedWithin('tiles-1', before, d, (a, b) => a[0] !== b[0]) });
await step('排序 → 同组插到某一条后面', () => dragInternal(`${nthIn('tiles-1', 1)} .card`, nthIn('tiles-1', 4), 'tail'), { wait: 2000, inPlace: true,
  outcome: (inv, d, res, before) => movedWithin('tiles-1', before, d, (a, b) => JSON.stringify(a) !== JSON.stringify(b)) });
await step('排序 → 同组拖到组标题（队尾）', () => dragInternal(`${firstIn('tiles-1')} .card`, 'h2[data-group="tiles-1"]', 'head'), { wait: 2000, inPlace: true,
  outcome: (inv, d, res, before) => movedWithin('tiles-1', before, d, (a, b) => a[a.length - 1] !== b[b.length - 1] && a.length === b.length) });
await step('排序 → 跨组插到某一条前面', () => dragInternal(`${firstIn('list-1')} .card`, nthIn('feat-1', 2), 'head'), { wait: 2000, inPlace: true,
  outcome: (inv, d, res, before) => crossMoved(before, d, 'list-1', 'feat-1', 1) });
await step('排序 → 跨组拖到组标题（队尾）', () => dragInternal(`${firstIn('tiles-1')} .card`, 'h2[data-group="feat-1"]', 'head'), { wait: 2000, inPlace: true,
  outcome: (inv, d, res, before) => {
    const p = crossMoved(before, d, 'tiles-1', 'feat-1', idsOf(before, 'feat-1').length);
    return p.length ? p : [];
  } });

// 19-21) 多选
const longPress = (gid, id) => ev(`const li=document.querySelector(${JSON.stringify(`[data-gid="${gid}"] li[data-item="${id}"]`)});
  if(!li) return 'NO-LI'; li.dispatchEvent(new PointerEvent('pointerdown',{bubbles:true,button:0})); return 'held'`);
// 长按之后**必须**已经在多选里（app.js 用 body[data-multi] 报这件事）。
// 不报这一条的话，「没进多选」的下游症状是「点卡片打开了那个网址」——页面一跳，
// 后面每一步都在错误页上量，一条真信号变成八条噪声。
const multiOn = () => run(`document.body.dataset.multi === 'on' ? 'on' : 'OFF'`);
const clickCard = (gid, id) => ev(`const li=document.querySelector(${JSON.stringify(`[data-gid="${gid}"] li[data-item="${id}"]`)});
  if(!li) return 'NO-LI'; const c=li.querySelector('.card,.feat-card,.plain'); c.click(); return 'clicked'`);
const barButton = (text) => ev(`const b=[...document.querySelectorAll('.multibar button')].find(x=>x.textContent.includes(${JSON.stringify(text)}));
  if(!b) return 'NO-BAR-BTN'; b.click(); return 'clicked'`);

await step('多选 → 移动到另一组（批内顺序原样）', async () => {
  const three = disk().groups.find((g) => g.id === 'tiles-1').items.slice(0, 3).map((i) => i.id);
  globalThis.__three = three;
  await longPress('tiles-1', three[0]); await sleep(800);
  const on = await multiOn();
  for (const id of three.slice(1)) { await clickCard('tiles-1', id); await sleep(200); }
  const picked = await run(`JSON.stringify([...document.querySelectorAll('li.sel-mark')].map(n=>n.dataset.item))`);
  const r1 = await barButton('移动到'); await sleep(500);
  const r2 = await ev(`const b=[...document.querySelectorAll('.sheet button')].find(x=>x.textContent.includes('单列组'));
    if(!b) return 'NO-GROUP-BTN'; b.click(); return 'moved'`);
  return `长按后 multi=${on} 选中=${picked} / ${r1} / ${r2}`;
}, { wait: 3000, outcome: (inv, d, res, before) => {
  const got = d.groups.find((x) => x.id === 'list-1').items.map((i) => i.id).filter((x) => globalThis.__three.includes(x));
  return JSON.stringify(got) === JSON.stringify(globalThis.__three) ? [] : [`落成 ${JSON.stringify(got)}，选中 ${JSON.stringify(globalThis.__three)}`];
} });

await step('多选 → 打标签', async () => {
  const two = disk().groups.find((g) => g.id === 'tiles-1').items.slice(0, 2).map((i) => i.id);
  globalThis.__two = two;
  await longPress('tiles-1', two[0]); await sleep(800);
  const on = await multiOn();
  await clickCard('tiles-1', two[1]); await sleep(200);
  // 选中了几条要看得见：多选那一族要是没选中，ops 就是空批——**服务端照样 200、
  // 屏上一句错都没有**，于是「标签没写上」看不出是哪一环出的问题。
  const picked = await run(`JSON.stringify([...document.querySelectorAll('li.sel-mark')].map(n=>n.dataset.item))`);
  const r1 = await barButton('打标签'); await sleep(500);
  const dlg = await topDialog();
  const r2 = await ev(`const d=window.__dlg; if(!d) return 'NO-DIALOG';
    const i=d.querySelector('input'); if(!i) return 'NO-INPUT';
    i.value='扫的标签'; i.dispatchEvent(new Event('input',{bubbles:true}));
    const b=[...d.querySelectorAll('button')].find(x=>x.textContent.trim()==='标签');
    if(!b) return 'NO-OK:'+[...d.querySelectorAll('button')].map(x=>x.textContent.trim()).join('|');
    b.click(); return 'tagged'`);
  await sleep(1200);
  const banner = await run(`(document.querySelector('#lb-banner')||{}).textContent||''`);
  return `multi=${on} 选中=${picked} 对话框=${dlg} / ${r1} / ${r2} / 横幅=${JSON.stringify(banner)}`;
}, { wait: 2500, outcome: (inv, d) => {
  const got = d.groups.flatMap((g) => g.items).filter((i) => (globalThis.__two || []).includes(i.id));
  return got.length === 2 && got.every((i) => (i.tags || []).includes('扫的标签')) ? [] : ['标签没写上'];
} });

await step('多选 → 删掉', async () => {
  const one = disk().groups.find((g) => g.id === 'tiles-1').items[0].id;
  await longPress('tiles-1', one); await sleep(800);
  const on = await multiOn();
  const r1 = await barButton('删掉'); await sleep(500);
  const r2 = await ev(`const b=[...document.querySelectorAll('.sheet button.danger')].pop(); if(!b) return 'NO-CONFIRM';
    b.click(); return 'confirmed'`);
  return `multi=${on} / ${r1} / ${r2}`;
}, { wait: 3000, outcome: shrank('tiles-1') });

// 22-23) 设置面板
await step('设置面板 → 分组排法切换', () => ev(`const b=document.querySelector('section[data-gid="tiles-1"] h2 button.layoutbtn');
  if(!b) return 'NO-BTN'; b.click(); return 'clicked'`), { wait: 2500,
  outcome: (inv, d) => d.groups.find((g) => g.id === 'tiles-1').layout === 'list' ? [] : ['排法没换'] });

await step('设置面板 → 另开新标签页', async () => {
  await ev(`document.getElementById('lb-prefs-open').click()`); await sleep(1400);
  return ev(`const f=document.querySelector('form[action="/prefs/tabs"]'); if(!f) return 'NO-FORM';
    const b=f.querySelector('button[value="1"],button[value="0"]'); if(!b) return 'NO-BTN'; b.click(); return 'clicked ' + b.value`);
}, { wait: 2500, outcome: (inv, d) => d.openInNewTab !== undefined ? [] : ['盘上没那个布尔'] });

say('');
say('控制台异常：' + JSON.stringify(errs.slice(0, 5)));
say(`${ran} 步，${fails} 条红`);
// 关掉自己开的那一格：留着会在调试口里攒一堆空白标签页，下一次跑的人看着更乱。
try { await fetch(`${CDP}/json/close/${tab.id}`); } catch { /* 关不掉就算了 */ }
ws.close(); process.exit(fails ? 1 : 0);
