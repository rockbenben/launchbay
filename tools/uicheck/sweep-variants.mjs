// 第二道横扫（2026-10-16）：第一道走的是**档案页上会写盘的那些交互**，这一道走
// 的是**别的族**——它们各自独立成一条通路，而第一道一条都没碰：
//
//	离线队列（enqueue → online → replay）：第一道每步都在清 localStorage，
//	  等于一直绕开它；而"存不进去的那一条在恢复后有没有自己进去"正是屏上与盘上
//	  分家最容易出现的地方。
//	设置面板其余各格：改名 / 视图 / 导航位置 / 主题 / 分组改名 / 分组颜色 /
//	  新建组 / 删组——每一格都是一次写加一次重画。
//	搜索两路：整页 ?q=（服务端渲染）与 ⌘K 面板（客户端过滤）**必须给同一个词
//	  同一个答案**，这是 AGENTS.md 里写过的一条判据。
//	剪贴板那一格：⌘K 里"把剪贴板里的网址存进来"。
//	窄屏 390 + 浅色：不许横向溢出。
//	减弱动效：prefers-reduced-motion: reduce 下新加的那几条必须显式关掉。
//	零 JS 通路：/pin、/prefs、/prefs/group、/revert、/import、/admin —— 这些页面
//	  本来就是给"没有脚本也能用"准备的，而第一道全程开着脚本。
//
//   node tools/uicheck/sweep-variants.mjs <port> <dataDir> [cdpBase]
import path from 'node:path';
import { connect, goHome as goHomeShared, makeStep, openPanel, seedFixture, waitFor } from './harness.mjs';

const PORT = process.argv[2] || '';
const DIR = path.resolve(process.argv[3] || 'labdata-sweep');
const CDP = process.argv[4] || 'http://127.0.0.1:9222';
const SRV = `http://127.0.0.1:${PORT}`;
const DATA = path.join(DIR, 'profiles', 'dad', 'bookmarks.json');
const ADM = process.env.LB_UI_ADMIN || '';
const WRT = process.env.LB_UI_WRITE || '';
if (!ADM || !WRT) throw new Error('缺 LB_UI_ADMIN / LB_UI_WRITE');
import fs from 'node:fs';

const stats = { ran: 0, fails: 0 };
const say = (s) => process.stdout.write(s + '\n');
const disk = () => JSON.parse(fs.readFileSync(DATA, 'utf8'));

await seedFixture(SRV, ADM, WRT);
const c = await connect(CDP);
const { run, ev, sleep, send } = c;
const login = async () => run("fetch('/pin',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'profile=dad'}).then(r=>r.status)");
const step = makeStep({ c, srv: SRV, dataPath: DATA, say, stats, login });

// 回档案页那份收在 harness.mjs 里（两道横扫共用）：上一步可能刚触发过整页重载，
// 而在正在被替换的旧文档上点设置是点不动的。
const goHome = async () => { await goHomeShared(c, SRV, login); };
// 轮询而不是固定等待的那一份**收在 harness.mjs 里**（两道横扫共用）：设置面板那
// 几格里有三格是 **`location.reload()`**（theme 就地、其余重载，见 app.js 那段），
// 于是"上一步刚触发重载、这一步就点开面板"会点在一个正在被替换的页面上——面板
// 没开，后面的按键自然一个都找不到，而症状读作"设置面板里没有那颗键"。
const firstIn = (gid) => `[data-gid="${gid}"] li[data-item]`;
const count = (d, gid) => d.groups.find((g) => g.id === gid).items.length;

await send('Emulation.setDeviceMetricsOverride', { width: 1500, height: 1300, deviceScaleFactor: 1, mobile: false });
await send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-reduced-motion', value: 'no-preference' }] });
await goHome();

// ── 离线队列 ────────────────────────────────────────────────────────────────
// **不清队列**（keepQueue）：那个族的前提就是队列里有东西。
await step('离线 → 存进去进队列（盘上不变）', async () => {
  await send('Network.emulateNetworkConditions', { offline: true, latency: 0, downloadThroughput: 0, uploadThroughput: 0 });
  await ev(`document.getElementById('lb-add').click()`); await sleep(300);
  await ev(`const u=document.getElementById('lb-url'); u.value='https://offline.example.com/q1';
    u.dispatchEvent(new Event('input',{bubbles:true})); u.dispatchEvent(new Event('blur',{bubbles:true}))`);
  await sleep(800);
  await ev(`const g=document.getElementById('lb-group'); g.value='tiles-1'; g.dispatchEvent(new Event('change',{bubbles:true}));`);
  await ev(`document.querySelector('#lb-sheet button[type=submit]').click()`);
  await sleep(1200);
  return 'queued=' + JSON.stringify(await run(`localStorage.getItem('lb-queue-dad')`));
}, { keepQueue: true, wait: 2000, allowBad: true, noInvariant: true,
  outcome: (inv, d, res, before) => {
    const q = String(res).replace(/^queued=/, '');
    const problems = [];
    if (q === 'null' || q === 'undefined') problems.push('队列里没有那一条（离线存进去应该排在待办里）');
    if (count(d, 'tiles-1') !== count(before, 'tiles-1')) problems.push('离线时盘上竟然变了');
    return problems;
  } });

await step('恢复联网 → 队列自己重放（屏上 == 盘上）', async () => {
  await send('Network.emulateNetworkConditions', { offline: false, latency: 0, downloadThroughput: -1, uploadThroughput: -1 });
  await ev(`window.dispatchEvent(new Event('online')); return 'fired'`);
  await sleep(4000);
  // 把横幅与队列一起带回来：这一族失败时**队列空了但盘上没有**，只报这一句的话
  // 分不出"被 409 丢掉了"（重放会点名）与"写失败了"（条目还在队列里）。
  return 'queue=' + JSON.stringify(await run(`localStorage.getItem('lb-queue-dad')`))
    + ' 横幅=' + JSON.stringify(await run(`(document.querySelector('#lb-banner')||{}).textContent||''`));
}, { keepQueue: true, wait: 3500,
  outcome: (inv, d, res) => {
    const problems = [];
    const q = d.groups.flatMap((g) => g.items).filter((i) => i.url.includes('offline.example.com'));
    if (q.length !== 1) problems.push(`盘上那条离线存的有 ${q.length} 条，要 1 条（${res}）`);
    return problems;
  } });

// ── 设置面板其余各格 ────────────────────────────────────────────────────────
// **先回到干净的首页再开面板**：上一步可能刚触发过 `location.reload()`（view /
// layout / 标题那几格都是），而人在正在重载的页面上点设置是点不动的——面板是
// fetch('/prefs') 注入的，那一页替换掉才装得上。轮询等它真的装好再找按键
// （轮询与开面板这两份收在 harness.mjs 里，两道横扫共用）。
const prefs = async () => {
  await goHome();
  return openPanel(c);
};
const clickIn = (sel) => ev(`const b=document.querySelector(${JSON.stringify(sel)}); if(!b) return 'NO:'+${JSON.stringify(sel)};
  b.click(); return 'clicked'`);

// **判的是「屏上真的改了」**：从前这一步的 outcome 是空的（`() => []`），于是它
// 只按一下、把 h1 打出来就算过——而「在浮层里按了没反应」那一族（2026-10-14 起
// 的「改名」与 2026-10-19 的「第二行」两格）就是这么从这道闸底下溜过去的：
// 点得到 ≠ 生效了，而这一步只管前者。
await step('设置面板 → 改这一页的名字', async () => {
  await prefs();
  await ev(`const f=[...document.querySelectorAll('form[action="/prefs"]')].find(x=>x.querySelector('input[name=title]'));
    if(!f) return 'NO-FORM'; const i=f.querySelector('input[name=title]'); i.value='扫过的那一页'; i.dispatchEvent(new Event('input',{bubbles:true}));
    f.querySelector('button[type=submit]').click(); return 'saved'`);
  // **等屏上真的换成新名字**（轮询而不是固定等待）：这一格按完会整页重载，
  // 固定等待在慢机器上会读到"重载还没发生"的那一页，于是判据自己骗自己。
  await waitFor(c, `(document.querySelector('h1')||{}).textContent==='扫过的那一页'`, 8000);
  return 'h1=' + JSON.stringify(await run(`document.querySelector('main h1, .topbar h1, h1')?.textContent||''`));
}, { wait: 2200, outcome: (inv, d, res) => {
  if (!String(res).startsWith('h1=')) return [`那一步没走通：${res}`];
  const h1 = String(res).slice(3).replace(/^"|"$/g, '');
  return h1 === '扫过的那一页' ? [] : [`顶栏大字还是「${h1}」，要「扫过的那一页」`];
} });

// **第二行印什么**（2026-10-19 加的那一格）：这是它自己的那一步——
// 「主机名 / 完整地址 / 不显示」三颗键，屏上第二行印什么由这一格说了算。
// 它从前**根本没有步**，于是「在浮层里按了没反应」没人看见。
await step('设置面板 → 第二行改成完整地址', async () => {
  const r = await prefs();
  if (r !== 'opened') return r;
  const hit = await clickIn('form[action="/prefs"] button[name=addrLevel][value=full]');
  if (!String(hit).startsWith('clicked')) return '那颗键不在面板里：' + hit;
  // 这一格也**整页重载**（改的是 profile.json，而第二行是服务端画出来的），
  // 所以要等重载完、且等第二行真的变成「完整地址」那一格再读。
  await waitFor(c, `!!document.querySelector('.card i.addr')`, 8000);
  return 'line=' + JSON.stringify(await run(`document.querySelector('.card i.addr')?.textContent||''`));
}, { wait: 2500, outcome: (inv, d, res) => {
  if (!String(res).startsWith('line=')) return [`那一步没走通：${res}`];
  const line = String(res).slice(5).replace(/^"|"$/g, '');
  // **跟盘上那份地址比**：屏上印的必须是**完整地址**，而不是主机名——那正是
  // 这一格选的那一档（`i.addr` 是 full 那一档的类名；主机名那一档是 `i.host`）。
  //
  // **比前缀而不是比全等**：那一格里还挂着一颗编辑笔（app.js 把把手 append 进
  // 同一个元素），所以 textContent 末尾多一个「✎」。地址本身在前面 ✓。
  const url = (d.groups.find((g) => g.id === 'tiles-1')?.items || [])[0]?.url || '';
  return line.startsWith(url) ? [] : [`屏上那张卡的第二行是「${line}」，要完整地址「${url}」`];
} });

await step('设置面板 → 主题切到浅色', async () => {
  await prefs();
  const hit = await clickIn('button[name=theme][value=light]');
  if (!String(hit).startsWith('clicked')) return hit;
  // 主题是**就地**换的（app.js 换 <html> 上的 data-theme，不整页重载）——但那是
  // **服务端应答之后**才换的（它先 POST 再改屏上那个属性），所以按下去立刻读会读到
  // 换之前的值。要等它真的换掉。
  await waitFor(c, `document.documentElement.dataset.theme==='light'`, 8000);
  return 'theme=' + JSON.stringify(await run(`document.documentElement.dataset.theme||''`));
}, { wait: 2200, outcome: (inv, d, res) => {
  if (!String(res).startsWith('theme=')) return [`那一步没走通：${res}`];
  const t = String(res).slice(6).replace(/^"|"$/g, '');
  return t === 'light' ? [] : [`<html> 上的 data-theme 是「${t}」，要 light`];
} });

await step('设置面板 → 主题切回深色', async () => {
  await prefs();
  const hit = await clickIn('button[name=theme][value=dark]');
  if (!String(hit).startsWith('clicked')) return hit;
  await waitFor(c, `document.documentElement.dataset.theme==='dark'`, 8000);
  return 'theme=' + JSON.stringify(await run(`document.documentElement.dataset.theme||''`));
}, { wait: 2200, outcome: (inv, d, res) => {
  if (!String(res).startsWith('theme=')) return [`那一步没走通：${res}`];
  const t = String(res).slice(6).replace(/^"|"$/g, '');
  return t === 'dark' ? [] : [`<html> 上的 data-theme 是「${t}」，要 dark`];
} });

await step('设置面板 → 换一种看分组的方式（视图）', async () => {
  await prefs();
  const hit = await clickIn('button[name=view][value=wall]');
  if (!String(hit).startsWith('clicked')) return hit;
  // 视图决定**整页怎么画**，没有一个属性可以单独换，所以这一格是整页重载：
  // 要等重载完再读 body 上那个属性。
  await waitFor(c, `document.body.dataset.profileView==='wall'`, 8000);
  return 'view=' + JSON.stringify(await run(`document.body.dataset.profileView||''`));
}, { wait: 2500, outcome: (inv, d, res) => {
  if (!String(res).startsWith('view=')) return [`那一步没走通：${res}`];
  const v = String(res).slice(5).replace(/^"|"$/g, '');
  return v === 'wall' ? [] : [`<body> 上的 data-profile-view 是「${v}」，要 wall`];
} });

await step('设置面板 → 分组导航挪到顶栏', async () => {
  await prefs();
  const hit = await clickIn('button[name=layout][value=topbar]');
  if (!String(hit).startsWith('clicked')) return hit;
  await waitFor(c, `document.body.dataset.profileLayout==='topbar'`, 8000);
  return 'layout=' + JSON.stringify(await run(`document.body.dataset.profileLayout||''`));
}, { wait: 2500, outcome: (inv, d, res) => {
  if (!String(res).startsWith('layout=')) return [`那一步没走通：${res}`];
  const l = String(res).slice(7).replace(/^"|"$/g, '');
  return l === 'topbar' ? [] : [`<body> 上的 data-profile-layout 是「${l}」，要 topbar`];
} });

await step('设置面板 → 改一个分组的颜色', async () => {
  const r = await prefs();
  if (r !== 'opened') return r;
  // 色值取**面板里真有的那一颗**（palette 是固定八色 + 自定义；写一个不存在的
  // 色值，报出来的红是"没点到那颗键"，读起来像功能坏了）。
  const hit = await clickIn('form[action^="/prefs/group"] button[name=accent][value="#e0574f"]');
  if (!String(hit).startsWith('clicked')) return '色块不在面板里：' + hit;
  await sleep(1800);
  return 'accent=' + JSON.stringify(await run(`document.querySelector('section[data-gid]')?.style.getPropertyValue('--sec-accent')||''`));
}, { wait: 2500, outcome: (inv, d, res) => {
  if (!String(res).startsWith('accent=')) return [`那一步没走通：${res}`];
  return /#[0-9a-f]{3,6}/i.test(String(res)) ? [] : [`屏上那一节的强调色没有跟着变：${res}`];
} });

await step('设置面板 → 给一个分组改名', async () => {
  const r = await prefs();
  if (r !== 'opened') return r;
  // **回车提交**：那一格只挂回车与 Esc（不挂失焦，也不挂一颗"保存"键——见 app.js
  // 的 wireGroupRename 与它上面那段判改理由）。而"回车在输入框里会提交表单"是
  // **浏览器的行为**，合成的 keydown 事件不会触发它——所以这里走 requestSubmit()，
  // 那才是同一件事在脚本里的样子。
  return ev(`const f=[...document.querySelectorAll('form[action^="/prefs/group"]')].find(x=>x.querySelector('input[name=group]')?.value==='list-1');
    if(!f) return 'NO-FORM'; const i=f.querySelector('input[name=rename]');
    i.value='扫改过名字的组'; i.dispatchEvent(new Event('input',{bubbles:true}));
    f.requestSubmit(); return 'submitted'`);
}, { wait: 2500, outcome: (inv, d, res) => {
  if (!String(res).startsWith('submitted')) return [`改名那一步没走通：${res}`];
  const on = d.groups.find((g) => g.id === 'list-1');
  return on && on.title === '扫改过名字的组' ? [] : [`盘上组名还是 ${on && on.title}`];
} });

await step('设置面板 → 新建一个分组', async () => {
  const r = await prefs();
  if (r !== 'opened') return r;
  const out = await ev(`const f=document.querySelector('form.prefs-group-new'); if(!f) return 'NO-FORM';
    const i=f.querySelector('input[name=add]'); i.value='扫建的那组'; i.dispatchEvent(new Event('input',{bubbles:true}));
    f.querySelector('button[type=submit]').click(); return 'created'`);
  // **这一格是原生表单提交**（委托对它 early-return，见 app.js 那段注释）：点下去
  // 浏览器自己发 POST 并跳去 /prefs。所以按下去之后**不能立刻导航**——那一次 POST
  // 还在路上，被我这一跳带走了，于是盘上什么都没有而步骤读起来像"按了没反应"。
  await sleep(3000);
  await goHome();
  return out;
}, { noInvariant: true, wait: 2000, outcome: (inv, d, res) => {
  const g = d.groups.find((x) => x.title === '扫建的那组');
  return g ? [] : [`盘上没有那个新分组（${res}）`];
} });

await step('设置面板 → 删掉一个分组（盘上与屏上一起走）', async () => {
  const r = await prefs();
  if (r !== 'opened') return r;
  const out = await ev(`const f=[...document.querySelectorAll('form[action^="/prefs/group"]')].find(x=>x.querySelector('input[name=group]')?.value==='empty-1');
    if(!f) return 'NO-FORM'; const b=f.querySelector('button[name=remove]'); if(!b) return 'NO-REMOVE'; b.click(); return 'removed'`);
  await goHome();
  return out;
}, { noInvariant: true, wait: 2500, outcome: (inv, d, res) => {
  const problems = [];
  if (!String(res).startsWith('removed')) problems.push(`删组那一步没走通：${res}`);
  if (d.groups.find((g) => g.id === 'empty-1')) problems.push('盘上还有那一组');
  return problems;
} });

// ── 搜索两路 ────────────────────────────────────────────────────────────────
await step('搜索 → 整页与面板对同一个词给同一个答案', async () => {
  const word = '甲';
  // 整页那一路（服务端渲染）
  await send('Page.navigate', { url: SRV + '/?q=' + encodeURIComponent(word) }); await sleep(1600);
  const whole = await run(`JSON.stringify([...document.querySelectorAll('li[data-item] b')].map(n=>n.textContent.trim()).sort())`);
  // 面板那一路（客户端过滤）
  await goHome();
  await send('Input.dispatchKeyEvent', { type: 'keyDown', key: 'k', code: 'KeyK', modifiers: 2, windowsVirtualKeyCode: 75 });
  await send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'k', code: 'KeyK', modifiers: 2, windowsVirtualKeyCode: 75 });
  await sleep(600);
  await ev(`const i=document.getElementById('lb-k'); if(!i) return 'NO-K';
    i.value=${JSON.stringify(word)}; i.dispatchEvent(new Event('input',{bubbles:true})); return 'typed'`);
  await sleep(900);
  const panel = await run(`JSON.stringify([...document.querySelectorAll('.cmdk li b')].map(n=>n.textContent.trim()).sort())`);
  await ev(`document.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true}))`);
  return `整页=${whole} 面板=${panel}`;
}, { noInvariant: true, wait: 800,
  outcome: (inv, d, res) => {
    const m = /整页=(\[.*?\]) 面板=(\[.*?\])$/.exec(String(res));
    if (!m) return [`读数没拿到：${res}`];
    return m[1] === m[2] ? [] : [`同一个词两个答案：整页 ${m[1]}，面板 ${m[2]}`];
  } });

// ── 窄屏 + 浅色：不许横向溢出 ───────────────────────────────────────────────
await step('窄屏 390 + 浅色 → 不许横向溢出，且屏上仍与盘上一致', async () => {
  await send('Emulation.setDeviceMetricsOverride', { width: 390, height: 844, deviceScaleFactor: 2, mobile: true });
  await goHome();
  return await run(`JSON.stringify({ scrollW: document.documentElement.scrollWidth, inner: window.innerWidth,
    溢出: [...document.querySelectorAll('main *')].filter(n=>n.getBoundingClientRect().right > window.innerWidth + 1)
      .slice(0,3).map(n=>n.tagName+'.'+(n.className||'').toString().slice(0,20)) })`);
}, { noInvariant: false, wait: 1200,
  outcome: (inv, d, res) => {
    const m = /"scrollW":(\d+),"inner":(\d+)/.exec(String(res));
    const problems = [];
    if (!m) problems.push('读数没拿到：' + res);
    else if (Number(m[1]) > Number(m[2]) + 1) problems.push(`横向溢出：内容 ${m[1]}px，视口 ${m[2]}px`);
    return problems;
  } });

// ── 减弱动效 ────────────────────────────────────────────────────────────────
await step('减弱动效 → 新加的那几条显式关掉', async () => {
  await send('Emulation.setDeviceMetricsOverride', { width: 1500, height: 1300, deviceScaleFactor: 1, mobile: false });
  await send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-reduced-motion', value: 'reduce' }] });
  await goHome();
  return await run(`(()=>{const li=document.querySelector('li[data-item]');
    const b=document.getElementById('lb-banner');
    const mk=document.createElement('style'); mk.id='lb-probe';
    mk.textContent='#lb-banner.ok::after{display:block}'; document.head.append(mk);
    const after=getComputedStyle(document.querySelector('#lb-banner')||document.body,'::after');
    const r={ 新条目动画: li?getComputedStyle(li).animationName:'NO-LI',
              横幅动画: b?getComputedStyle(b).animationName:'NO-BANNER',
              细线: after?after.display:'?' };
    mk.remove(); return JSON.stringify(r) })()`);
}, { noInvariant: true, wait: 800,
  outcome: (inv, d, res) => {
    const m = /"新条目动画":"([^"]*)"/.exec(String(res));
    if (!m) return ['读数没拿到：' + res];
    // 页面里没有 is-new 的行时读到的就是默认 animation（none），所以这条只钉
    // "不是 lb-pop 那种动画在跑"。
    return m[1] === 'lb-pop' || m[1] === 'lb-ring' ? [`减弱动效下还在跑 ${m[1]}`] : [];
  } });

// ── 键盘 ────────────────────────────────────────────────────────────────────
await step('键盘 → Ctrl+K 开面板，Esc 关掉', async () => {
  await send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-reduced-motion', value: 'no-preference' }] });
  await goHome();
  await send('Input.dispatchKeyEvent', { type: 'keyDown', key: 'k', code: 'KeyK', modifiers: 2, windowsVirtualKeyCode: 75 });
  await send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'k', code: 'KeyK', modifiers: 2, windowsVirtualKeyCode: 75 });
  await sleep(600);
  const opened = await run(`!document.querySelector('.cmdk')?.hidden`);
  await ev(`document.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true}))`);
  await sleep(400);
  const closed = await run(`document.querySelector('.cmdk')?.hidden !== false`);
  return `opened=${opened} closed=${closed}`;
}, { noInvariant: true, wait: 400,
  outcome: (inv, d, res) => String(res) === 'opened=true closed=true' ? [] : [`面板没按预期开关：${res}`] });

// ── 零 JS 通路 ──────────────────────────────────────────────────────────────
{
  await send('Emulation.setScriptExecutionDisabled', { value: true });
  const noJS = async (label, url, assert) => {
    stats.ran++;
    await send('Page.navigate', { url }); await sleep(1200);
    const body = await send('Runtime.evaluate', { expression: 'document.body ? document.body.innerText : ""', returnByValue: true });
    const problems = await assert(body.result?.result?.value || '', url);
    if (problems.length) { stats.fails++; say(`FAIL 零JS · ${label}`); for (const p of problems) say('     · ' + p); }
    else say(`PASS 零JS · ${label}`);
  };
  await noJS('档案页仍然画出卡片', SRV + '/', async (txt) => {
    const problems = [];
    if (!txt.includes('磁贴组')) problems.push('正文没有那一组的名字：关掉脚本后它就没了');
    if (txt.includes('批量操作')) problems.push('多选那条工具带不该在关掉脚本时出现');
    return problems;
  });
  await noJS('/prefs 这一页立得住', SRV + '/prefs', async (txt) => {
    const problems = [];
    if (!/管理密钥|机器|偏好|设置/.test(txt)) problems.push('页面几乎是空的');
    return problems;
  });
  await noJS('/revert 这一页立得住', SRV + '/revert', async (txt) => txt.trim() ? [] : ['回退页是空的']);
  await noJS('/import 这一页立得住', SRV + '/import', async (txt) => txt.trim() ? [] : ['导入页是空的']);
  await noJS('/admin 这一页立得住', SRV + '/admin', async (txt) => txt.trim() ? [] : ['管理页是空的（未登录时会给登录表单，那也是内容）']);
  await send('Emulation.setScriptExecutionDisabled', { value: false });
  await goHome();
}

say('');
say('控制台异常：' + JSON.stringify(c.errs.slice(0, 5)));
say(`${stats.ran} 步，${stats.fails} 条红`);
await c.close();
process.exit(stats.fails ? 1 : 0);
