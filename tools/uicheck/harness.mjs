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
  //
  // **长文本也要有一份**（2026-10-21 加）。这一组数据从前**全是两三个字的名字**
  // （磁贴组 / 甲乙丙丁），于是「长文本」这一整类缺陷在闸里**根本不可见**：
  // 2026-10-21 修的那个「长分组名把整页撑成横向可滚」（390 档 scrollWidth 474
  // vs 视口 390）明明有现成的「窄屏 390 → 不许横向溢出」那一步盯着，
  // 却因为固定数据里没有长名字而**一次都没触发过**。
  // 同一批里还藏着两个：名字长到与图标挤不到同一行时**图标被独自留在第一行**
  // （紧凑档那一格从 40px 长到 93px），以及卡片名被挤到第二行。
  // 三个都是同一件事的三个切面：**长文本**。它不铺进来，闸就看不见这一族。
  const groups = [
    { id: 'tiles-1', title: '磁贴组', layout: 'tiles', items: ['甲', '乙', '丙', '丁'] },
    { id: 'list-1', title: '单列组', layout: 'list', items: ['戊', '己', '庚'] },
    { id: 'feat-1', title: '大卡组', layout: 'feature', items: ['辛', '壬', '癸'] },
    { id: 'empty-1', title: '空组', layout: 'tiles', items: [] },
    // **长名字的分组**（横向可滚那一族）与**长名字的条目**（图标孤儿那一族）
    { id: 'long-1', title: '一个名字特别长的分组用来看看标题被截断的时候是什么样',
      layout: 'list', items: ['一个名字也特别长的书签条目用来测试截断与换行', '短'] },
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

// assertDesktopPointer：闸量的是**桌面档**的形状，而"是不是桌面档"这件事
// **浏览器自己报**（`matchMedia('(hover: hover) and (pointer: fine)')`）。
//
// 紧凑档的"每条一行"（40px）是**精确指针那一档**的形状：base.css 里那族去壳规则
// ——去边框、去内衬、图标收到 15px、把手回流到行尾——整族写在
// `@media (hover:hover) and (pointer:fine)` 里面，**触摸设备一个字都不省**
//（那里 §7.5 的 44px 地板接着管，密集不能拿触摸目标去换）。
//
// **2026-10-21 的 CI 实录**：CI runner 上的无头 Chrome（Linux，没有 X、没有鼠标）
// 报的是 `pointer: none`——那族去壳规则整族不生效，紧凑档的卡于是保持着整副壳，
// 量到 69–82px，而那一档的契约是 40px。闸于是报"紧凑档里有 82px 高的条目"，
// **读起来像界面坏了，其实量的是另一个档**。本机（Windows 的浏览器本来就报精确
// 指针）读到 40px，全绿——**同一条闸在两台机器上答案相反**，绿的那一次才让人
// 以为"界面验过了"。与 2026-10-21 那次"站级默认语言"是同一族病：环境没钉住。
//
// 闸自己起的那台浏览器由启动参数钉死（uicheck_test.go 的 `--blink-settings`）；
// **这一格是最后一道**：参数哪天失效了、或者用 LB_CDP 复用了一台别人开着的
// 浏览器（那台报什么就是什么），这里**立刻说破**，而不是让 54 步拿错形状量一遍。
export async function assertDesktopPointer(run) {
  const raw = await run(`JSON.stringify({悬停:matchMedia('(hover: hover)').matches,
    精确:matchMedia('(pointer: fine)').matches, 粗:matchMedia('(pointer: coarse)').matches,
    无:matchMedia('(pointer: none)').matches})`);
  const p = JSON.parse(raw);
  if (p.精确 && p.悬停) return p;
  throw new Error('这台浏览器没报「精确指针 + 能悬停」（读到 ' + raw + '）——'
    + '闸量的是桌面档的形状：紧凑档的去壳规则只对精确指针生效（触摸设备保留 44px 的地板）。'
    + '闸自己起的那台浏览器由启动参数钉死；若用 LB_CDP 复用别人开着的浏览器，'
    + '那台浏览器得自己报精确指针，否则量出来的不是这一档。');
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
  // 前置检查失败时**先把连接收了再抛**：node 在 Windows 上带着一个开着的
  // WebSocket 退出时会崩在 libuv 的断言上（退出码 0xc0000409），
  // 于是那句真正要说的话被一行断言刷过去——**报错要说得出话**。
  try {
    await assertDesktopPointer(run);
  } catch (e) {
    try { await fetch(`${cdp}/json/close/${tab.id}`); } catch { /* 关不掉就算了 */ }
    ws.close();
    throw e;
  }
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

// waitFor：轮询一个屏上表达式，直到它为真。
//
// **为什么是轮询而不是固定等待**：设置面板那几格里有几格是 **`location.reload()`**
// （theme 就地、其余重载，见 app.js 那段），于是按下去之后屏上要过一会儿才换成
// 换过的那一份——固定等待在慢机器上会读到"重载还没发生"的那一页，于是判据自己
// 骗自己（它报的是"没变"，而其实只是还没变完）。
//
// **它治不了另一件事**：点「设置」那一下落在**正在被替换的旧文档**上时，那一下
// 根本不会生效（见 goHome 那一段），轮询多久都等不到面板。
export async function waitFor(c, expr, ms = 6000) {
  const until = Date.now() + ms;
  for (;;) {
    const v = await c.run(`(()=>{try{return ${expr}}catch(e){return false}})()`);
    if (v) return v;
    if (Date.now() > until) return false;
    await c.sleep(200);
  }
}

// goHome：回档案页，并**等它真的稳住**（两道横扫共用这一份）。
//
// 为什么要多这一次导航：上一步可能刚触发过**整页重载**（分组排法切换、视图、
// 标题那几格都是），而在**正在被替换的旧文档**上点「设置」是点不动的——那一下的
// fetch 跟着旧文档一起被丢掉，而新文档从没收到过那一下，于是面板没开、按键一个
// 都找不到，症状却读作「设置面板里没有那颗键」（2026-10-19 实测：这道闸就是这么
// 红的，而它红得毫无道理）。
//
// 登录也在里面：会话是 cookie 里的，而导航会带上它；先导航一次再登一次，是为了
// 「会话还没建好时首页落在选择页」那一种形状也归到同一处。
export async function goHome(c, srv, login) {
  await c.send('Page.navigate', { url: srv + '/' }); await c.sleep(1500);
  if (login) await login();
  await c.send('Page.navigate', { url: srv + '/' }); await c.sleep(1500);
}

// openPanel：点顶栏那颗「设置」，并**等面板真的装上**。
//
// 面板是 app.js fetch('/prefs') 之后注入的（零 JS 时它就是一次普通跳转），而
// 上一步可能刚触发过整页重载——在正在被替换的页面上点设置是点不动的，于是面板
// 没开、按键一个都找不到。两道横扫共用这一份。
export async function openPanel(c) {
  await c.ev(`document.getElementById('lb-prefs-open')?.click(); return 'clicked'`);
  const ok = await waitFor(c, `!!document.querySelector('form[action="/prefs"]')`, 8000);
  return ok ? 'opened' : 'PANEL-NOT-OPEN';
}

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
