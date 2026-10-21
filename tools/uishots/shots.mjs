// UI 走查拍摄：把每个页面在三个宽度 × 两套主题下拍出来，供人眼逐张看。
//
//   node shots.mjs <port> <outDir> [theme]
//
// 位置参数：<port> 被测服务（已经在跑） · <outDir> 拍到哪儿 · [theme] dark,light（默认两个都拍）
// 环境变量：
//   LB_SHOTS_CDP         调试口（默认 http://127.0.0.1:9222）
//   LB_SHOTS_EMPTY_PORT  **空机器**的端口——引导页只在一台零档案的机器上出现
//   LB_SHOTS_PROFILE     走查用哪一份（默认 dad）
//   LB_SHOTS_ADMIN       管理密钥（默认 ui-admin-key）
//   LB_SHOTS_PIN         那几份的 PIN（默认空 = 它不设防）。**没设 PIN 的档案给不给
//                        PIN 都进得去**（auth.VerifyEntry：没设 PIN 就直接放行），
//                        所以一把钥匙可以同时开"不设防的那份"与"就设成这一把的那份"。
//
// **这轮修的三件事**（2026-10-21）：
//
// ① **清单跟着路由表走**。从前那两条是死的：`/switch` 与 `/admin/login`
//    **只有 POST**（路由表 internal/httpapi/routes.go 是唯一的注册来源），
//    拿 GET 去拍，拍到的是 404 页——而那两张拍出来**逐字节相同**（MD5 一致），
//    也就是说清单里有两个格子在骗人。清单改成路由表里真有的那些页，并补上
//    从前没有的：面板新出的两节（out/pin）、选择页、引导页、坏档页、404 页。
//    **导入页有两屏**（import.tmpl 的三个 Mode）：档案空 → 选文件那一屏；
//    档案非空 → 「挡住了」那一屏（**没有表单**，只有一个链接）。两屏都拍。
//
// ② **拍到的必须是对的那一屏**。每页声明一个**结构性钩子**（选择器，不靠
//    中文文案——文案会翻、结构不会），拍完验一次；对不上就把话说出来
//    （「拿到的是 X，不是要拍的那一屏」），而不是照样存一张看着像那么回事的图。
//    **一把量出假警报的尺子比没有尺子更糟**——走查的人只看图，不会去核对
//    它是不是那一页，于是"那两格是 404"这件事没人会发现。
//
// ③ **空白卡不是空白**。全局那条 `section{content-visibility:auto}` 让离屏的
//    section 不进渲染，于是**整页截图里那些格子是空矩形**。实测（390 档管理页
//    的「管理密钥」那一格）：innerText **0 字**、那块区域里**一个文字像素都没有**、
//    格子高 726px（那是 `contain-intrinsic-size` 的占位估值，不是真高度）。
//    **滚一遍也不补**（布局高度对了、画没补上：墨水还是 102）——
//    `captureBeyondViewport` 自己不补画离屏那一块。
//    办法是**把视口拉成整页高再拍**：所有 section 都在"屏内"，浏览器就真的
//    把它们画出来（**同一块区域里文字像素 0 → 14424**、格子高 726 → 208px、
//    图高 2920 → 2145），**而 CSS 一行没动**（containment 还在，真实浏览器里
//    被裁掉的那截溢出在照片里也照样被裁）。另一条路是强制
//    `content-visibility:visible`——它也补上内容，但顺带去掉了 containment，
//    于是照片里会出现真实浏览器里看不见的那截溢出。**照片要等于屏上那一屏**，
//    所以走拉高视口那条。
//
// **拍不到就说拍不到**：引导页要一台零档案的机器（没给 LB_SHOTS_EMPTY_PORT
// 就不拍，并说清为什么），坏档页要那台机器上真的有坏档（问服务端要，见下）。
// 一张拍不到的照片用一张别的页充数，比不拍更坏。
import fs from 'node:fs';

const SRV = 'http://127.0.0.1:' + (process.argv[2] || 7881);
const OUT = process.argv[3] || 'design-preview/ui-shots';
const THEMES = (process.argv[4] || 'dark,light').split(',');
const CDP = process.env.LB_SHOTS_CDP || 'http://127.0.0.1:9222';
const EMPTY_PORT = process.env.LB_SHOTS_EMPTY_PORT || '';
const PROFILE = process.env.LB_SHOTS_PROFILE || 'dad';
const ADMIN_KEY = process.env.LB_SHOTS_ADMIN || 'ui-admin-key';
const PIN = process.env.LB_SHOTS_PIN || '';
fs.mkdirSync(OUT, { recursive: true });

const tab = await (await fetch(`${CDP}/json/new?about:blank`, { method: 'PUT' })).json();
const ws = new WebSocket(tab.webSocketDebuggerUrl);
let id = 0; const pending = new Map();
ws.addEventListener('message', (e) => { const m = JSON.parse(e.data); if (m.id && pending.has(m.id)) { pending.get(m.id)(m); pending.delete(m.id); } });
const send = (m, p = {}) => new Promise((r) => { const i = ++id; pending.set(i, r); ws.send(JSON.stringify({ id: i, method: m, params: p })); });
// **带超时的求值**：表单提交会销毁执行上下文，那一次 evaluate 可能**永远不回话**
//（CDP 那边等的是一个已经不存在的上下文）。没有超时的话整个工具就挂在那儿。
const run = async (e) => {
  const r = await Promise.race([
    send('Runtime.evaluate', { expression: e, awaitPromise: true, returnByValue: true }),
    new Promise((r2) => setTimeout(() => r2({ __timeout: true }), 6000)),
  ]);
  if (r.__timeout) return null;
  if (r.result?.exceptionDetails) return null;
  return r.result?.result?.value;
};
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
await new Promise((r) => ws.addEventListener('open', r, { once: true }));
await send('Page.enable'); await send('Runtime.enable'); await send('Network.enable');

// ── 页面清单：每一行都对着路由表认得出来的那条路 ─────────────────────────────
//
// `expect` 是**结构性钩子**：拍完拿它验一次"这一屏是不是要的那一屏"。
// 不用中文文案判定——文案会翻，结构不会。
const PAGES = [
  { name: 'profile', url: '/', state: 'profile', expect: 'body[data-profile]' },
  { name: 'picker', url: '/', state: 'none', expect: 'form[action="/pin"]' },
  { name: 'prefs-look', url: '/prefs', state: 'profile', expect: '#prefs-tab-look:checked' },
  { name: 'prefs-groups', url: '/prefs?tab=groups', state: 'profile', expect: '#prefs-tab-groups:checked' },
  { name: 'prefs-out', url: '/prefs?tab=out', state: 'profile', expect: '#prefs-tab-out:checked' },
  { name: 'prefs-pin', url: '/prefs?tab=pin', state: 'profile', expect: '#prefs-tab-pin:checked' },
  { name: 'revert', url: '/revert', state: 'profile', expect: 'form[action="/revert"]' },
  { name: 'history', url: '/history', state: 'profile', expect: 'ul.hist' },
  // 导入**两屏**（import.tmpl 的 Mode）：选文件那一屏只对**空档案**开放，
  // 档案非空时给的是「挡住了」那一屏（没有表单，只有一个链接）。
  { name: 'import', url: '/import', state: 'empty', expect: 'form[action="/import"]' },
  { name: 'import-blocked', url: '/import', state: 'nonempty', expect: 'main:not(.chooser):has(p.notice):not(:has(form))' },
  { name: 'admin', url: '/admin', state: 'admin', expect: 'main.adm-page' },
  { name: 'adminlogin', url: '/admin', state: 'profile', expect: 'form[action="/admin/login"]' },
  // 坏档页**不导航**：进门那一次 POST 的应答**就是**它（profile.json 读不动时，
  // handlePin 直接把坏档页吐出来，不发会话；bookmarks.json 读不动时那次 POST
  // 303 回 /，屏上也是它）。再导航一次就把这一屏冲掉了。
  { name: 'broken', url: '/', state: 'broken', expect: 'form[action="/switch"]', noNav: true },
  { name: 'notfound', url: '/nope', state: 'none', expect: 'main.chooser:not(:has(form))' },
  { name: 'setup', url: '/', state: 'none', expect: 'form[action="/setup"]', on: 'empty' },
];

// ── 身份：每一页拍之前，浏览器得是谁 ────────────────────────────────────────
//
// **进门走表单提交**（注入一张表单再 submit），不是 fetch：fetch 不会导航，
// 而"进不去"这件事在屏上才看得见——**坏档那一页只出现在 POST 的应答上**。
const submit = async (action, fields) => {
  // **字段用一条循环灌进去，不是一个一个 `const i=…`**：两个字段就撞
  // 「Identifier 'i' has already been declared」，而那个异常是在**表单提交
  // 之前**抛的——于是这一格安静地没进门，而症状是"进不去这一份"。
  const kv = JSON.stringify(Object.entries(fields));
  try {
    const r = await run(`(()=>{const f=document.createElement('form');f.method='POST';f.action=${JSON.stringify(action)};for(const p of ${kv}){const el=document.createElement('input');el.name=p[0];el.value=p[1];f.appendChild(el);}document.body.appendChild(f);f.submit();return 'submitted'})()`);
    // **表单没发出去要说出来**：上面那类异常是在提交**之前**抛的，吞掉它
    // 就等于"安静地没进门"，而症状看着像"进不去这一份"——那是两件事。
    return r === 'submitted' ? 'submitted' : `表单没发出去（${r}）`;
  } catch {
    return 'submitted'; // 页面已经在导航了：评估上下文没了是正常的
  } finally {
    await sleep(1400);
  }
};

async function setState(srv, state, theme) {
  await send('Network.clearBrowserCookies');
  if (state === 'none') return 'ok';
  // 先到这台机器上（表单要有个同源的落点）
  await send('Page.navigate', { url: srv + '/healthz' }); await sleep(400);
  if (state === 'broken') {
    // 坏档那一页**只在 POST 的应答上**（profile.json 读不动 → handlePin 直接吐它，
    // 不发会话）。所以它没有会话，于是**主题也设不了**——它跟系统（见下）。
    if (!BROKEN_ID) return '这台机器上没有坏档';
    const s = await submit('/pin', { profile: BROKEN_ID });
    return s === 'submitted' ? 'ok' : `进坏档那一步没成（${s}）`;
  }
  // 导入那一族要**两份形状不同的档案**：选文件那一屏只对空档案开放，
  // 「挡住了」那一屏只对非空的档案开放。两份都从服务端那份聚合里找。
  const which = state === 'empty' ? EMPTY_ID : state === 'nonempty' ? NONEMPTY_ID : '';
  if (state === 'empty' || state === 'nonempty') {
    if (!which) return state === 'empty' ? '这台机器上没有空档案（导入那一屏只对空档案开放）' : '这台机器上所有档案都是空的';
  }
  const who = which || PROFILE;
  const fields = { profile: who };
  if (PIN) fields.pin = PIN;
  const s = await submit('/pin', fields);
  if (s !== 'submitted') return `进门那一步没成（${s}）`;
  const on = await run(`(document.body.dataset.profile || '')`);
  if (on !== who) return `进不去 ${who}（它有 PIN？这台机器上的钥匙是 ${PIN ? '给定的' : '空的'}）`;
  // 主题：**属于档案**的那几页用 /prefs 显式设（要会话，所以登录之后才设得了）；
  // **不属于任何档案**的那几页（选择页/坏档页/管理页/引导页）压根没有 data-theme
  // 字段（exec.go 的 Theme 那段），它们跟系统——那是 prefers-color-scheme 说了算。
  // 两条都摆上，页面走哪条都落在这一档。
  await submit('/prefs', { theme });
  if (state === 'admin') await submit('/admin/login', { key: ADMIN_KEY });
  return 'ok';
}

// ── 候选档案：问服务端要，别在盘上猜 ────────────────────────────────────────
//
// GET /api/v1/status 那份聚合里有三样判据（服务端自己的，不是这个工具的）：
//   broken  =  这一份的数据文件读不动（profile.json 或 bookmarks.json）
//   items   =  这一份现在有几条书签
//   disabled=  这一份被停用了（进不去）
// 于是三样都要拍的页面各有各的那一份档案，**问出来**而不是写死：
// 坏档页要 broken 的那一份，导入的两屏要空的那一份与非空的那一份。
let BROKEN_ID = '', EMPTY_ID = '', NONEMPTY_ID = '';
async function findCandidates(srv) {
  try {
    const r = await fetch(srv + '/api/v1/status', { headers: { Authorization: 'Bearer ' + ADMIN_KEY } });
    if (!r.ok) return;
    const j = await r.json();
    for (const p of j.profiles || []) {
      if (p.disabled) continue;
      if (p.broken && !BROKEN_ID) { BROKEN_ID = p.id; continue; }
      if (p.items === 0 && !EMPTY_ID) EMPTY_ID = p.id;
      else if (p.items > 0 && !NONEMPTY_ID) NONEMPTY_ID = p.id;
    }
  } catch { /* 问不到就当没有——那几屏会被报成"没拍" */ }
}

// ── 拍 ──────────────────────────────────────────────────────────────────────
let W = 1440, H = 900;

async function capture(name) {
  // **先把视口拉成整页高**：见头注释 ③——离屏的 section 只有在"屏内"才会被画。
  const tall = Math.min(Math.ceil((await run('document.documentElement.scrollHeight')) || 900) + 600, 30000);
  await send('Emulation.setDeviceMetricsOverride', { width: W, height: tall, deviceScaleFactor: 1, mobile: W < 500 });
  await sleep(450);
  // 画完再量一次真实内容底边：body 有 min-height:100vh，视口一高就多出一截
  // 背景，裁掉它，照片才等于那一屏本身（固定定位的那几件不算——它们不在流里）。
  const contentH = await run(`(()=>{let b=0;for(const e of document.body.querySelectorAll('*')){if(getComputedStyle(e).position==='fixed')continue;const r=e.getBoundingClientRect();if(r.bottom>b)b=r.bottom;}return Math.ceil(b+scrollY)})()`);
  const s = await send('Page.captureScreenshot', {
    format: 'png', captureBeyondViewport: true,
    clip: { x: 0, y: 0, width: W, height: Math.max(contentH || 1, 1), scale: 1 },
  });
  fs.writeFileSync(`${OUT}/${name}.png`, Buffer.from(s.result.data, 'base64'));
  await send('Emulation.setDeviceMetricsOverride', { width: W, height: H, deviceScaleFactor: 1, mobile: W < 500 });
  return contentH;
}

// ── 走一遍 ──────────────────────────────────────────────────────────────────
const sizes = [[1440, 900, 'w'], [700, 900, 'm'], [390, 844, 'n']];
let ok = 0, bad = 0;

await findCandidates(SRV);

for (const theme of THEMES) {
  for (const [w, h, tag] of sizes) {
    W = w; H = h;
    await send('Emulation.setDeviceMetricsOverride', { width: w, height: h, deviceScaleFactor: 1, mobile: w < 500 });
    // 主题对**不属于任何档案**的那几页（选择页/坏档页/管理页/引导页）是
    // prefers-color-scheme 说了算（它们没有 data-theme 字段，见 exec.go 的
    // Theme 那段）；属档案的那几页由 /prefs 显式设。两边都摆上。
    await send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-color-scheme', value: theme }] });
    for (const page of PAGES) {
      const srv = page.on === 'empty' ? (EMPTY_PORT ? 'http://127.0.0.1:' + EMPTY_PORT : '') : SRV;
      if (!srv) {
        process.stdout.write(`${theme}/${tag}/${page.name} — 没拍：引导页只在一台零档案的机器上出现（给 LB_SHOTS_EMPTY_PORT）\n`);
        bad++;
        continue;
      }
      const st = await setState(srv, page.state, theme);
      if (st !== 'ok') {
        process.stdout.write(`${theme}/${tag}/${page.name} — 没拍：${st}\n`);
        bad++;
        continue;
      }
      if (!page.noNav) {
        await send('Page.navigate', { url: srv + page.url });
        await sleep(page.name === 'profile' ? 1600 : 1100);
      }
      const got = await run(`!!document.querySelector(${JSON.stringify(page.expect)})`);
      const title = await run(`document.title || ''`);
      if (!got) {
        process.stdout.write(`${theme}/${tag}/${page.name} ✗ 拿到的是「${title}」，不是要拍的那一屏（${page.expect}）\n`);
        bad++;
        continue;
      }
      const contentH = await capture(`${theme}-${tag}-${page.name}`);
      process.stdout.write(`${theme}/${tag}/${page.name} ✓ 「${title}」 h=${contentH}\n`);
      ok++;
    }
  }
}

console.log(`\n${ok} 张拍成，${bad} 张没拍或没对上 —— ${OUT}`);
try { await fetch(`${CDP}/json/close/` + tab.id); } catch { /* ignore */ }
ws.close();
await sleep(150);
process.exit(0);
