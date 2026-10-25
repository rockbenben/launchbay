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
//	待办删掉最后一条：外层提示框一起走，不留空壳。
//	bookmarklet 预填：?add= 打开浮层，读取要有一个字的状态。
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

// 待办清单「删掉最后一条」：外层提示框必须跟着走（2026-10-24）。
// 清单从这一版起包在 `.notice` 里；只摘 `#lb-qlist` 会留下一个空壳，而离线时
// 那颗「补进去」不画、又没有关闭键——空壳会一直挂到刷新。判据是壳本身消失，
// 不只是列表消失。
await step('待办 → 删掉最后一条后不留空壳', async () => {
  await send('Network.emulateNetworkConditions', { offline: true, latency: 0, downloadThroughput: 0, uploadThroughput: 0 });
  await ev(`document.getElementById('lb-add').click()`); await sleep(300);
  await ev(`const u=document.getElementById('lb-url'); u.value='https://offline.example.com/drop';
    u.dispatchEvent(new Event('input',{bubbles:true})); u.dispatchEvent(new Event('blur',{bubbles:true}))`);
  await sleep(500);
  await ev(`const g=document.getElementById('lb-group'); g.value='tiles-1'; g.dispatchEvent(new Event('change',{bubbles:true}));`);
  await ev(`document.querySelector('#lb-sheet button[type=submit]').click()`);
  await sleep(1000);
  await ev(`document.getElementById('lb-q')?.click()`);
  await sleep(400);
  await ev(`document.querySelector('#lb-qlist button')?.click()`);
  await sleep(400);
  const res = await run(`JSON.stringify({
    queue: localStorage.getItem('lb-queue-dad'),
    chip: !!document.getElementById('lb-q'),
    box: !!document.getElementById('lb-queue'),
    list: !!document.getElementById('lb-qlist') })`);
  await send('Network.emulateNetworkConditions', { offline: false, latency: 0, downloadThroughput: -1, uploadThroughput: -1 });
  await ev(`window.dispatchEvent(new Event('online')); return 'online'`);
  await sleep(800);
  await goHome();
  return res;
}, { noInvariant: true, allowBad: true, wait: 400,
  outcome: (inv, d, res, before) => {
    let m;
    try { m = JSON.parse(String(res)); } catch { return ['读数没拿到：' + res]; }
    const problems = [];
    if (m.queue !== '[]') problems.push(`队列里还剩 ${m.queue}，要清空`);
    if (m.chip) problems.push('待办 chip 还在');
    if (m.list) problems.push('清单 ul 还在');
    if (m.box) problems.push('清单摘空了但外层提示框还在（离线时没有关闭键，会一直挂到刷新）');
    if (JSON.stringify(d) !== JSON.stringify(before)) problems.push('删一条待办竟然动了盘上的书签');
    return problems;
  } });

// bookmarklet 预填（?add=）会在模块初始化时调用 resolveNow（2026-10-24）。
// 它用到的那一格地址形状闸如果写在调用点之后，会撞 TDZ：浮层照开、但读取整个
// 不发生，状态行空着，错误只在 console。判据是状态行有结论，不只是浮层开了。
await step('bookmarklet 预填 → 浮层打开且读取有结论', async () => {
  const target = SRV + '/healthz';
  await send('Page.navigate', { url: SRV + '/?add=' + encodeURIComponent(target) + '&t=Bookmarklet' });
  await sleep(1800);
  const res = await run(`(()=>{const d=[...document.querySelectorAll('[role=dialog]')].find(x=>getComputedStyle(x).display!=='none');
    if(!d) return JSON.stringify({sheet:false});
    return JSON.stringify({sheet:true, url:d.querySelector('input[type=url]')?.value||'',
      statuses:[...d.querySelectorAll('.sheet-status')].map(n=>n.textContent.trim()).filter(Boolean)});})()`);
  await goHome();
  return res;
}, { noInvariant: true, allowBad: true, wait: 400,
  outcome: (inv, d, res) => {
    let m;
    try { m = JSON.parse(String(res)); } catch { return ['读数没拿到：' + res]; }
    const problems = [];
    if (!m.sheet) problems.push('?add= 没有打开添加浮层');
    if (!String(m.url).includes('/healthz')) problems.push(`浮层里的地址不对：${m.url}`);
    if (!m.statuses || !m.statuses.length) problems.push('读取没有一个字的状态：resolveNow 没跑（TDZ 或调用顺序又断了）');
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

// **「我的 PIN」那一格**（2026-10-20）：浮层里改自己那道门**不许整页跳走**。
//
// 这一格是面板里最后加的一族，而 app.js 那几份提交委托里**没有它**——于是它在浮层
// 里走原生提交、303 回 `/prefs?tab=pin`，症状是「设置变成单独页面了」（实拍复现）。
// 与 2026-10-14 给「点开书签」那一格修过的是同一件事。
//
// 这一步走三趟：**设上 → 用错的旧 PIN → 取消**。三趟都是「填一格、按一下」，而
// **夹具最后回到原样**（dad 仍然不设防）——后面那些步骤要免 PIN 进门，把门锁上
// 就把它们全带崩了。
await step('设置面板 → 改自己那道门（不跳走）', async () => {
  const r = await prefs();
  if (r !== 'opened') return r;
  // 浮层里那列 radio 只切本地（不改 URL），所以要自己点过去。
  await ev(`const x=document.querySelector('#lb-prefs #prefs-tab-pin'); if(!x) return 'NO-RADIO';
    x.checked=true; x.dispatchEvent(new Event('change',{bubbles:true})); return 'ok'`);
  await sleep(300);
  const url0 = await run(`location.pathname+location.search`);
  // ① 设上：这一份本来不设防，所以没有「现在的」那一格，只填「新的」。
  await ev(`const f=document.querySelector('#lb-prefs form[action^="/prefs/pin"]'); if(!f) return 'NO-FORM';
    const n=f.querySelector('input[name=new]'); n.value='4321'; n.dispatchEvent(new Event('input',{bubbles:true}));
    f.requestSubmit(); return 'set'`);
  await sleep(2000);
  const after1 = await run(`JSON.stringify({url:location.pathname+location.search, sheet:!!document.querySelector('#lb-prefs:not([hidden])'), hasOld:!!document.querySelector('#lb-prefs input[name=old]')})`);
  // ② 用错的旧 PIN：错话要留在浮层里，而且**门没被改**。
  await ev(`const f=document.querySelector('#lb-prefs form[action^="/prefs/pin"]'); if(!f) return 'NO-FORM';
    const o=f.querySelector('input[name=old]'); const n=f.querySelector('input[name=new]');
    if(o) o.value='9999'; n.value='5678'; f.requestSubmit(); return 'wrong'`);
  await sleep(2000);
  const after2 = await run(`JSON.stringify({url:location.pathname+location.search, sheet:!!document.querySelector('#lb-prefs:not([hidden])'), msg:(document.querySelector('#lb-prefs .notice')||{}).textContent||''})`);
  // ③ 取消：把夹具还原成不设防。
  await ev(`const f=document.querySelector('#lb-prefs form[action^="/prefs/pin"]'); if(!f) return 'NO-FORM';
    const o=f.querySelector('input[name=old]'); const n=f.querySelector('input[name=new]');
    if(o) o.value='4321'; n.value=''; f.requestSubmit(); return 'cancel'`);
  await sleep(2000);
  const after3 = await run(`JSON.stringify({url:location.pathname+location.search, sheet:!!document.querySelector('#lb-prefs:not([hidden])'), hasOld:!!document.querySelector('#lb-prefs input[name=old]')})`);
  return `url0=${url0} ①${after1} ②${after2} ③${after3}`;
}, { wait: 1500, outcome: (inv, d, res) => {
  const problems = [];
  const s = String(res);
  const url0 = (/url0=(\S+)/.exec(s) || [])[1] || '';
  const states = [...s.matchAll(/[①②③](\{.*?\})/g)].map((m) => JSON.parse(m[1]));
  if (states.length !== 3) return [`那一步没走通：${s}`];
  for (const [i, st] of states.entries()) {
    if (st.url !== url0) problems.push(`第 ${i + 1} 趟之后 URL 从 ${url0} 变成了 ${st.url}——整页跳走了（症状是「设置变成单独页面了」）`);
    if (!st.sheet) problems.push(`第 ${i + 1} 趟之后浮层不见了`);
  }
  if (!states[0].hasOld) problems.push('设上之后「现在的 PIN」那一格没长出来——形状没跟着变');
  if (!/现在的 PIN 不对/.test(states[1].msg || '')) problems.push(`旧 PIN 填错时浮层里没有那句话：${JSON.stringify(states[1].msg)}`);
  if (states[2].hasOld) problems.push('取消之后「现在的 PIN」那一格还在——形状没跟着变');
  return problems;
} });

// ── 对齐（2026-10-21 实拍后加）───────────────────────────────────────────────
//
// 这一族量的是**像素**，不是功能：键照样能按、图标照样能点，而屏上看着没对齐。
// 这一族的两条判据都来自用户实拍：
//   ① 台面面板「我的 PIN」那一行：键浮在输入框上方 15px（键 145..189、框 160..204）。
//   ② 档案页的**页脚**掉进某一栏里（全局档它碎成两片：栏 2 底部 + 栏 3 顶部）。
//
// **为什么放在这道横扫而不是只写一条 CSS 断言**：CSS 断言钉的是「规则还在」，
// 而这里钉的是「**现在真的平**」——两份标记（独立页与浮层注入）共用同一份 CSS，
// 而浮层那一份是 fetch 进来再注入的，它有没有真的拿到那条规则，只有浏览器知道。
await step('台面面板 → 「我的 PIN」那一行的键与输入框平齐', async () => {
  const r = await prefs();
  if (r !== 'opened') return r;
  // 浮层里那列 radio 只切本地（不改 URL），所以要自己点过去。
  await ev(`const x=document.querySelector('#lb-prefs #prefs-tab-pin'); if(!x) return 'NO-RADIO';
    x.checked=true; x.dispatchEvent(new Event('change',{bubbles:true})); return 'ok'`);
  await sleep(300);
  return await run(`(()=>{const f=document.querySelector('#lb-prefs form[action^="/prefs/pin"]'); if(!f) return 'NO-FORM';
    const inp=f.querySelector('input[name=new]'); const btn=f.querySelector('button[type=submit]');
    if(!inp||!btn) return 'NO-FIELD';
    const a=inp.getBoundingClientRect(), b=btn.getBoundingClientRect();
    return JSON.stringify({inp:[Math.round(a.top),Math.round(a.bottom)],btn:[Math.round(b.top),Math.round(b.bottom)]});})()`);
}, { noInvariant: true, wait: 600,
  outcome: (inv, d, res) => {
    const m = /"inp":\[(-?\d+),(-?\d+)\],"btn":\[(-?\d+),(-?\d+)\]/.exec(String(res));
    if (!m) return [`读数没拿到：${res}`];
    const dTop = Number(m[3]) - Number(m[1]);
    const dBot = Number(m[4]) - Number(m[2]);
    const problems = [];
    // 1px 的容差：亚像素布局下 getBoundingClientRect 会给出 0.5px 的碎值，
    // 而肉眼看不出的那一档不值得让这道闸红。
    if (Math.abs(dTop) > 1) problems.push(`键比输入框高了 ${dTop}px（输入框 ${m[1]}..${m[2]}，键 ${m[3]}..${m[4]}）`);
    if (Math.abs(dBot) > 1) problems.push(`键比输入框低了 ${-dBot}px（输入框 ${m[1]}..${m[2]}，键 ${m[3]}..${m[4]}）`);
    return problems;
  } });

await step('全局视图 → 页脚横跨整幅，不在栏里', async () => {
  // 视图是**全机器的偏好**（全档案共享），所以先记住原来那一档，最后改回去。
  const view0 = await run(`document.documentElement.dataset.profileView || ''`);
  await run(`fetch('/prefs',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'view=wall'})`);
  await sleep(900);
  await goHome();
  const read = await run(`(()=>{const f=document.querySelector('footer'); const main=document.querySelector('main');
    if(!f||!main) return 'NO-FOOTER';
    const fr=f.getBoundingClientRect(), mr=main.getBoundingClientRect();
    const secs=[...main.querySelectorAll(':scope > section')].map(s=>Math.round(s.getBoundingClientRect().bottom));
    return JSON.stringify({fw:Math.round(fr.width),mw:Math.round(mr.width),ft:Math.round(fr.top),
      lowest:secs.length?Math.max(...secs):0,frags:f.getClientRects().length});})()`);
  // 还原视图（后面那些步骤要的是原来那一档的版面）。
  await run(`fetch('/prefs',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'view=${view0 || 'groups'}'})`);
  await sleep(700);
  await goHome();
  return `view0=${view0} ${read}`;
}, { noInvariant: true, wait: 800,
  outcome: (inv, d, res) => {
    const problems = [];
    const m = /"fw":(\d+),"mw":(\d+),"ft":(\d+),"lowest":(\d+),"frags":(\d+)/.exec(String(res));
    if (!m) return [`读数没拿到：${res}`];
    const [, fw, mw, ft, lowest, frags] = m.map(Number);
    // 页脚必须是**一整块**：碎成几片就是被栏切了（实测全局档碎成栏 2 底部 + 栏 3 顶部两片）。
    if (frags !== 1) problems.push(`页脚碎成了 ${frags} 片——它被多栏切开了`);
    // 与正文同宽：它是这一页的页脚，不该是某一栏里的一个盒子。
    if (Math.abs(fw - mw) > 2) problems.push(`页脚宽 ${fw}px，正文宽 ${mw}px——它没有横跨整幅`);
    // 落在所有分组下面：页脚在正文列的末尾，不在某一栏的中间。
    if (ft < lowest) problems.push(`页脚顶边 ${ft}px 还在某个分组（最低 ${lowest}px）上面——它不是页脚`);
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

// 分组名换到第二行时，「分组」把手必须和**第一行**齐平（2026-10-25）。
// 从前 drawer 用 align-items:center，把手被放在两行之间，屏上读成「第一组在上面、
// 分组在下面」——分组导航的层级反了。夹具里的 long-1 组保证这里会换行。
await step('窄屏 390 · 分组把手与第一行齐平', async () => {
  await send('Emulation.setDeviceMetricsOverride', { width: 390, height: 844, deviceScaleFactor: 2, mobile: true });
  // 上一段设置步骤把分组导航挪到了顶栏；这一条量的是侧栏抽屉，先挪回来。
  const r = await prefs();
  if (r !== 'opened') return 'PREFS ' + r;
  const hit = await clickIn('button[name=layout][value=sidebar]');
  if (!String(hit).startsWith('clicked')) return 'CLICK ' + hit;
  await waitFor(c, `document.body.dataset.profileLayout==='sidebar'`, 8000);
  await goHome();
  return await run(`(() => {
    const d=document.querySelector('.side .drawer');
    const s=d && d.querySelector('summary');
    const n=d && d.querySelector('.gnav');
    if(!d||!s||!n) return JSON.stringify({err:'missing'});
    const sr=s.getBoundingClientRect(), nr=n.getBoundingClientRect();
    const tops=[...n.querySelectorAll('li')].map(li=>Math.round(li.getBoundingClientRect().top));
    return JSON.stringify({summaryTop:Math.round(sr.top), navTop:Math.round(nr.top), tops});
  })()`);
}, { wait: 300,
  outcome: (inv, d, res) => {
    let x;
    try { x = JSON.parse(String(res)); } catch { return ['读数拿不到：' + res]; }
    if (x.err) return ['分组抽屉不在：' + res];
    const problems = [];
    if (Math.abs(x.summaryTop - x.navTop) > 2) {
      problems.push(`「分组」把手 y=${x.summaryTop}，分组名第一行 y=${x.navTop}，要齐平`);
    }
    if (!Array.isArray(x.tops) || x.tops.length < 2) {
      problems.push('夹具只量到一组，量不到换行形状');
    }
    return problems;
  } });

// 无结果那一句会把查询词原样印出来（2026-10-24）：贴一条没有空格的长地址
// 进来搜是常事，而它不会自己折行——实测窄屏视口 486、整页 scrollWidth 1593，
// 那一句与「看全部」都被推到屏幕外；修复后这一步在 390 档复测。屏上还得真有
// 那一句，否则这一步在空转。
await step('窄屏 390 · 搜索一个无空格长词 → 不许横向溢出', async () => {
  await send('Emulation.setDeviceMetricsOverride', { width: 390, height: 844, deviceScaleFactor: 2, mobile: true });
  await send('Page.navigate', { url: SRV + '/?q=' + 'a'.repeat(200) });
  await sleep(1500);
  const res = await run(`JSON.stringify({ scrollW: document.documentElement.scrollWidth, inner: window.innerWidth,
    has: !!document.querySelector('.nomatch') })`);
  // 下一步的前置是「开始时页面还是档案页」，而搜索结果页没有 [data-gid]：
  // 量完把页面放回去，别让下一条替我背这个锅。
  await goHome();
  return res;
}, { noInvariant: true, wait: 800,
  outcome: (inv, d, res) => {
    const m = /"scrollW":(\d+),"inner":(\d+),"has":(true|false)/.exec(String(res));
    if (!m) return ['读数没拿到：' + res];
    const problems = [];
    if (Number(m[1]) > Number(m[2]) + 1) problems.push(`横向溢出：内容 ${m[1]}px，视口 ${m[2]}px`);
    if (m[3] !== 'true') problems.push('没有命中无结果那一句——这一步没走到要量的地方');
    return problems;
  } });

// ── 版式三件事（2026-10-21 加）────────────────────────────────────────────────
// 这一族全是**同一件事的三个切面**：长文本与行内对齐。
//
//	① 同一排里各卡的**图标**要齐平（名字长到与图标挤不到同一行时，图标会被
//	  独自留在第一行——那一格于是从 40px 长到 93px）；
//	② **紧凑档每条一行**（这一档的契约就是这八个字）；
//	③ 横幅上那颗**关闭键不许折行**（中文到处可断，它的自动最小尺寸只剩一个字，
//	  于是「知道了」被压成「知道 / 了」两行）。
//
// 它们从前都不在闸里，于是能一直坏着。**长文本铺进固定数据**（harness.mjs 的
// seedFixture）之后，这一族才在闸里可见——2026-10-21 那三个修法就是靠这批数据
// 量出来的。

// ① 同一排里图标与名字齐平
await step('卡片 → 同一排里图标与名字齐平（没有孤儿图标）', async () => {
  await send('Emulation.setDeviceMetricsOverride', { width: 1400, height: 1000, deviceScaleFactor: 1, mobile: false });
  await goHome();
  return await run(`(()=>{
    const bad=[];
    for(const li of document.querySelectorAll('li[data-item]')){
      const a=li.querySelector('.card,.plain'); if(!a) continue;
      const ic=a.querySelector('.ic,.ic-svg,.ic-img'); const b=a.querySelector('b');
      if(!ic||!b) continue;
      const d=Math.abs(ic.getBoundingClientRect().top-b.getBoundingClientRect().top);
      if(d>6) bad.push((b.textContent||'').trim().slice(0,10)+' 差'+Math.round(d)+'px');
    }
    return JSON.stringify({孤儿:bad.length, 例:bad.slice(0,3)});
  })()`);
}, { noInvariant: true, wait: 500,
  outcome: (inv, d, res) => {
    const m = /"孤儿":(\d+)/.exec(String(res));
    if (!m) return ['读数没拿到：' + res];
    return Number(m[1]) > 0 ? [`有 ${m[1]} 张卡的图标与名字不在同一行（图标成了孤儿）：${res}`] : [];
  } });

// ② 紧凑档每条一行
await step('紧凑档 → 每条一行（长名字也不许把行撑高）', async () => {
  // 视图与「第二行印什么」都是**全机器的偏好**（改它会重画整页），
  // 所以先记住现在这两样，量完放回去。
  const before = await run(`JSON.stringify({v:document.body.dataset.profileView||'groups',
    a:document.body.dataset.addrLevel||'host'})`);
  const b = JSON.parse(before);
  // **第二行必须是「主机名」这一档**（2026-10-21 改判）：「完整地址」是**用户显式选的**
  // 那一档，而那一档**按设计**就该在紧凑档里也印出第二行——`prefs.addrLevel.hint`
  // 明写「选了完整地址之后，密集视图与窄屏磁贴也显示第二行」。
  // 拿它当缺陷 = 把用户的显式选择读成 bug（2026-10-21 第一次写这条时正是这么错的：
  // 读到 90px 的行就红了，而那是**横扫自己**把第二行改成完整地址造成的）。
  await run(`fetch('/prefs',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'view=dense&addrLevel=host'})`);
  await sleep(900);
  await goHome();
  const res = await run(`(()=>{
    // 只量**能点的那些卡**：「点不动的那一行」（.plain）自带一行地址，
    // 它高一点是设计（那一行的正文就是地址）。
    const cs=[...document.querySelectorAll('li[data-item]')].map(li=>{
      const a=li.querySelector('.card'); if(!a) return null;
      return {h:Math.round(a.getBoundingClientRect().height),
        sec:li.closest('section')?.dataset.gid||'?',
        t:(a.querySelector('b')?.textContent||'').trim().slice(0,10)};}).filter(Boolean);
    const top=cs.slice().sort((x,y)=>y.h-x.h).slice(0,3);
    return JSON.stringify({最高:Math.max(...cs.map(c=>c.h)), n:cs.length, 最高的几张:top,
      视图:document.body.dataset.profileView||'?',
      // **指针那一格也进读数**：这一档的形状只在精确指针上成立，而浏览器报什么
      // 是它自己说的（harness.mjs 的 connect() 已经先把过这一关）。
      指针:(matchMedia('(hover: hover) and (pointer: fine)').matches?'精确':'非精确')});
  })()`);
  await run(`fetch('/prefs',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'view=${b.v}&addrLevel=${b.a}'})`);
  await sleep(700);
  return res;
}, { noInvariant: true, wait: 400,
  outcome: (inv, d, res) => {
    const m = /"最高":(\d+)/.exec(String(res));
    if (!m) return ['读数没拿到：' + res];
    // 40px 是这一档的行高；60 留一格余量（44px 的地板 + 内衬）。
    return Number(m[1]) > 60 ? [`紧凑档里有 ${m[1]}px 高的条目——这一档的契约是「每条一行」：${res}`] : [];
  } });
// ③ 横幅的关闭键不许折行
await step('横幅 → 关闭键不折行（一个字一行是坏的）', async () => {
  await send('Emulation.setDeviceMetricsOverride', { width: 1400, height: 1000, deviceScaleFactor: 1, mobile: false });
  await goHome();
  // 长按进多选：那条提示横幅就是这时候出来的（app.js 的 app.multi.hint）。
  await ev(`const li=document.querySelector('li[data-item]');
    if(!li) return 'NO-LI'; li.dispatchEvent(new PointerEvent('pointerdown',{bubbles:true,button:0})); return 'held'`);
  await sleep(900);
  const res = await run(`(()=>{
    const d=document.querySelector('#lb-banner .dismiss'); if(!d) return JSON.stringify({键:'没有'});
    const b=d.getBoundingClientRect();
    return JSON.stringify({字:d.textContent.trim(), 宽:Math.round(b.width), 高:Math.round(b.height)});
  })()`);
  await ev(`document.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true}))`);
  await sleep(400);
  return res;
}, { noInvariant: true, wait: 400,
  outcome: (inv, d, res) => {
    const m = /"高":(\d+)/.exec(String(res));
    if (!m) return ['读数没拿到（多选那条横幅没出来？）：' + res];
    // 一行是 44px（min-height 那个地板）；折成两行就是 52、三行 72。
    return Number(m[1]) > 46 ? [`关闭键被压成 ${m[1]}px 高——它的字折行了：${res}`] : [];
  } });

// ── 管理页在窄屏下：六列表格翻成卡片，全仓没有闸量过它 ────────────────────────
// **它不用 `step()`**：那个 helper 的前置是"这一步开始时页面还得是档案页"
// （[data-gid] 数为 0 就判成"上一步把页面弄走了"），而管理页本来就没有分组。
// 零 JS 那几步也是自己走一套（见下面那个 noJS）——同一件事，同一种做法。
{
  stats.ran++;
  await send('Emulation.setDeviceMetricsOverride', { width: 390, height: 844, deviceScaleFactor: 2, mobile: true });
  // 管理权要一把真的管理密钥（环境变量里那把）——这一页是账号层面，管理员那把才进得去。
  await run(`fetch('/admin/login',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'key=${ADM}'})`);
  await sleep(500);
  await send('Page.navigate', { url: SRV + '/admin' }); await sleep(1600);
  const res = await run(`(()=>{
    const de=document.documentElement, vw=de.clientWidth;
    const over=[...document.querySelectorAll('body *')].filter(n=>{
      const b=n.getBoundingClientRect(); return b.width>0 && b.height>0 && b.right>vw+1;})
      .slice(0,3).map(n=>n.tagName+'.'+String(n.className).slice(0,16));
    const tbl=document.querySelector('.adm');
    const thead=tbl?getComputedStyle(tbl.querySelector('thead')).display:'no-table';
    // 同一**横行**里两格左右交叠 = 卡片式排版撞了
    let overlap=null;
    for(const tr of document.querySelectorAll('.adm tbody tr')){
      const cells=[...tr.children].map(c=>({k:c.getAttribute('data-k')||c.className, b:c.getBoundingClientRect()}));
      for(let i=0;i<cells.length;i++) for(let j=i+1;j<cells.length;j++){
        const A=cells[i].b,B=cells[j].b;
        const sameRow=Math.abs(A.top-B.top)<Math.min(A.height,B.height)/2;
        if(sameRow && A.left < B.right-1 && B.left < A.right-1) overlap=[cells[i].k,cells[j].k];
      }
    }
    return JSON.stringify({vw, scrollW:de.scrollWidth, thead, over, overlap});
  })()`);
  const problems = [];
  // **键序要与上面 JSON.stringify 的对象字面量一致**（vw 在前、scrollW 在后）：
  // 2026-10-21 第一次写成了 "scrollW":…,"vw":…，于是正则永远不匹配，
  // 每一次都报「读数没拿到」——**一条量不出东西的断言比没有断言更坏**。
  const m = /"vw":(\d+),"scrollW":(\d+)/.exec(String(res));
  if (!m) problems.push('读数没拿到：' + res);
  else {
    if (Number(m[2]) > Number(m[1]) + 1) problems.push(`横向溢出：内容 ${m[2]}px，视口 ${m[1]}px`);
    if (!/"thead":"none"/.test(String(res))) problems.push('窄屏下表头没收起来（列名会与每格重复）');
    if (/"overlap":\[/.test(String(res))) problems.push('同一行里两格左右交叠：' + res);
    // **空数组不算**：「"over":[]」是"没有元素越界"，而 `\[\S` 会把 `]` 当成内容——
    // 那样每一次都误报（2026-10-21 第二次写这条时踩的）。
    // 要判的是**方括号里确实有东西**：`\[[^\]]`。
    if (/"over":\[[^\]]/.test(String(res))) problems.push('有元素超出视口右缘：' + res);
  }
  if (problems.length) { stats.fails++; say('FAIL 管理页 · 窄屏 390 → 表格翻成卡片，不横滚不重叠'); for (const p of problems) say('     · ' + p); }
  else say('PASS 管理页 · 窄屏 390 → 表格翻成卡片，不横滚不重叠');
  await send('Emulation.setDeviceMetricsOverride', { width: 1500, height: 1300, deviceScaleFactor: 1, mobile: false });
  await goHome();
}

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
