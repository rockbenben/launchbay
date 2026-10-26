// bg.js —— P2 浏览器扩展的后台（service worker）。
//
// 两件事：右键收藏一条、批量收藏当前窗口的所有标签页（spec §14 的 P2 形状）。
//
// **令牌按档案配，存在 chrome.storage.local**，不进 cookie、不进 sync：
// chrome.storage.sync 会跟着浏览器账号上云，而那个令牌是这台机器的写权限
// （§6.3 那条"令牌只发给 AI、扩展与 CLI"，扩展是其中之一，但**云同步过的令牌
// 等于把 NAS 的写权限放进 Google 的账号里**）。
//
// 扩展**不读 cookie、不开标签页到 launchbay**：它直接调 REST（credScoped，令牌
// 在 Authorization 头里）。走会话 cookie 的话，扩展与那个浏览器档案绑死，
// 而这台机器上的档案是多个人的。
//
// 它开的 host_permissions 只到 http/https：chrome:// 与 file:// 这类地址
// 右键收藏得到，但服务端会原样存下来并在界面上标「需手动打开」（§9 特例一）——
// 扩展不做 URL 白名单，替用户偷偷删书签不是它该做的事。
//
// 这里**一句中文都不写**：每一句人都要读到的话都住在 `_locales/*/messages.json`
// （internal/i18n/extcatalog_test.go 钉着两份语言一一对应，并钉着"服务端登记的
// 每一个错误码在这儿都有交代"）。服务端答的那些 detail / reason 是给脚本读的
// 协议话，不进通知——它们进 console，那里有排查的人与 AI。

import { t, httpErr, httpErrKey, kindOf, serverNote, noteTarget } from './i18n.js';

const CFG_KEY = 'lb-config';
const DEFAULTS = { base: '', tokens: {}, defaultProfile: '' };

async function cfg() {
  const got = await chrome.storage.local.get(CFG_KEY);
  return { ...DEFAULTS, ...(got[CFG_KEY] || {}) };
}

function currentProfile(c) {
  const p = c.defaultProfile;
  return p && c.tokens[p] ? p : Object.keys(c.tokens)[0] || '';
}

// send 打这台机器。**每一处失败都要变成人话**：扩展弹一下就没了，报错写在
// console 里的话，点了没反应的人根本不会去看那里。`body` 不给就是 GET。
// 哪一类失败说哪一句的判据在 i18n.js 的 KIND 表——三处通路共用那一张表。
async function send(c, path, body, profile) {
  const opts = body === undefined
    ? { method: 'GET' }
    : { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) };
  opts.headers = { Authorization: 'Bearer ' + (c.tokens[profile] || ''), ...(opts.headers || {}) };
  let res;
  try {
    res = await fetch(c.base + path, opts);
  } catch (e) {
    // 地址写错、机器没起、端口没开，拿到的都是 "Failed to fetch" 这一句英文——
    // 而那是这条流水线上**最常被读到的一句**，贴原文等于让人去猜。
    throw Object.assign(new Error(t('netFail', e.message)), { where: noteTarget('netFail') });
  }
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    // 通知里说的是**出路**（按 kind 查表，见 i18n.js），服务端那句 detail/reason
    // 是给脚本对照的协议话，只进 console：排查的人要原话，正要点下去的人要下一步。
    console.warn('lb: ' + path + ' → ' + res.status, kindOf(data), serverNote(data));
    throw Object.assign(new Error(httpErr(res, data, profile)), { where: noteTarget(httpErrKey(res, data)) });
  }
  return data;
}

// readData 是 GET /api/v1/data。**它必须看状态码**：以前这里直接 .json()，
// 于是一次 401 会让下面那句"这个档案一个分组都没有"上了屏——说的是一句假故障，
// 而真正的原因（令牌不对）就在这条请求里被丢掉了。
async function readData(c, profile) {
  return send(c, '/api/v1/data?profile=' + encodeURIComponent(profile), undefined, profile);
}

// ── 反馈：成功走角标，失败走通知 ────────────────────────────────────────────
// 以前所有结果都只进角标，而角标那一小块地方**只放得下三个字符**：
// "这个档案还没有令牌"上屏变成"这个档"，一条 409 变成"HTTP "。
// 也就是说唯一那条反馈渠道刚好装不下最需要被读到的那种内容。
// 分法：成功是"按完了，知道了"——角标两个字够用；失败必须**整句读得完**，
// 所以走系统通知，并且点通知直接打开设置页（报错十有八九是配置问题）。
async function tellOK(text) {
  await chrome.action.setBadgeText({ text });
  await chrome.action.setBadgeBackgroundColor({ color: '#2a63bd' });
  setTimeout(() => chrome.action.setBadgeText({ text: '' }), 4000);
}

// tellNote：一整句读得完的话。`!` 那个角标在这里说的是"什么都没写进去"，
// 不只是"出事了"——"这一窗本来就没有新东西"也归它管。
// `where` 是"点这条通知该落到哪儿"（'page' | 'settings'，判据见 i18n.js 的
// NOTE_TARGET）。不传就当 settings——那是一句说不清出路的失败该有的默认。
let lastNoteWhere = 'settings';

async function tellNote(title, message, profile, where) {
  lastNoteWhere = where === 'page' ? 'page' : 'settings';
  await chrome.action.setBadgeText({ text: '!' });
  await chrome.action.setBadgeBackgroundColor({ color: '#b3261e' });
  setTimeout(() => chrome.action.setBadgeText({ text: '' }), 6000);
  const body = profile ? `${message}\n${t('failProfile', profile)}` : message;
  try {
    await chrome.notifications.notify({
      id: 'lb-fail',
      title,
      message: body,
      iconUrl: 'icons/32.png',
      priority: 2,
    });
  } catch {
    // 系统通知被关掉（专注模式、Windows 的"通知已关闭"）是常态而不是异常：
    // 那时候至少把句子留在控制台，别让"点了没反应"变成三件事叠在一起。
    console.error('launchbay:', title, message);
  }
}

// 点通知 = 走那句话教的那条路（2026-10-16）。从前这里无条件开设置页，于是
// 一句"去页面上在那一条上按编辑"配着一颗把人带去设置页的按钮——**一句话不许
// 同时指两个地方**。设置类失败照旧开设置页，那才是它的出路。
chrome.notifications?.onClicked?.addListener(async (id) => {
  if (id !== 'lb-fail') return;
  if (lastNoteWhere !== 'page') {
    await chrome.runtime.openOptionsPage();
    return;
  }
  const c = await cfg();
  if (!c.base) {
    await chrome.runtime.openOptionsPage();
    return;
  }
  await chrome.tabs.create({ url: c.base });
});

// saveOne 的三种结果：`null` = 存进去了；`{dup:true}` = 这一条本来就在这份里，
// 一个字都没写；`{why}` = 这一条没存进去，why 是能整句读得完的人话。
// "已经在里面了"为什么不归到失败那一档：批量那一窗里，一个早就存过的主页
// 不该把后面十一个标签页一起停下来——而让服务端来回一句"item id 重复"再把它
// 翻译成人话，比在这儿看一眼已经拿到手的数据贵。
async function saveOne(page, profile) {
  const c = await cfg();
  if (!c.base) return { why: t('noBase'), where: noteTarget('noBase') };
  if (!profile || !c.tokens[profile]) return { why: t('noToken'), where: noteTarget('noToken') };
  // 先 resolve 一次：它把标题、描述、建议分组与图标候选都填好，而服务端据此
  // 拒掉/收下每一条。**不要在扩展里自己造分组** —— 那是服务端那份映射的职责，
  // 两处各判一次就会分家。
  let picked = {};
  try {
    const r = await send(c, '/api/v1/resolve', { url: page.url, profile }, profile);
    picked = {
      name: r.title || page.title || page.url,
      desc: r.desc || '',
      icon: (r.iconCandidates && r.iconCandidates[0]) || '',
      // suggestedGroup 以前被**整个丢掉**：这里构造 picked 时没带它，下面
      // `picked.group` 于是恒为 undefined，服务端算好的建议分组一次都没用上，
      // 每条都落进"第一个分组"。修法是把它接上，而不是在扩展里另猜一套。
      //
      // **它是一个对象 {id,title}，不是一个 id 字符串**（2026-10-19 修）：服务端
      // `resolveResponse.SuggestedGroup` 是 `*resolveGroup{ID,Title}`，JSON 出来
      // 是 `{"id":"...","title":"..."}`。从前这里整个对象塞进 `group`，而下面拿
      // 它跟字符串 id 比（`g.id === picked.group`）恒为 false —— 于是**建议分组
      // 一次都没生效过，每条都落进第一个分组**，而提示还写"已修好"。
      // 判据只有一处：下面用 `picked.group.id`。
      group: r.suggestedGroup || '',
    };
  } catch {
    // resolve 抓不到（内网、已失效、要登录）不算失败：标题退回标签页标题，
    // 其余留空，让人在界面上补。
    picked = { name: page.title || page.url, desc: '', icon: '', group: '' };
  }

  const cur = await readData(c, profile);

  const groups = (cur && cur.data && cur.data.groups) || [];
  if (groups.some((g) => (g.items || []).some((it) => it.url === page.url))) return { dup: true };
  // **取 `picked.group.id`**：服务端给的是 `{id,title}` 对象（见上面那段）。
  const wanted = groups.find((g) => g.id === (picked.group && picked.group.id));
  const gid = (wanted && wanted.id) || (groups[0] && groups[0].id) || '';
  if (!gid) return { why: t('noGroup'), where: noteTarget('noGroup') };

  const value = { id: idFor(page.url, picked.name), name: picked.name, url: page.url };
  if (picked.desc) value.desc = picked.desc;
  if (picked.icon) value.icon = picked.icon;

  await send(c, '/api/v1/batch', {
    profile, expectedRev: cur.rev, by: 'extension',
    ops: [{ op: 'add', path: 'groups/' + gid + '/items', value }],
  }, profile);
  return null;
}

// 条目 id 从**地址**派生：名字会被 resolve 填、也会被手改，拿它当 id 意味着
// 改一次名字这条的 id 就变了。形状与 app.js 的 slug() 是同一族（两边各自
// 实现过一次，改的时候记得两处都要看——这是已知代价，不是疏忽的理由）。
//
// **query 要算进去**（2026-10-19 修）：从前只用「主机名首段 + 路径末段」，于是
// `https://www.youtube.com/watch?v=abc123` 与 `...?v=zzz999` 派生出**同一个**
// id（`youtube-watch`）。扩展先按 URL 判重（两个 URL 不同 → 不是重复），再按
// id 写入 —— 两次判据不一致，于是第二个视频**根本没存进去**，服务端答 422
// `duplicate-id`，而扩展把这句话翻成「这一条已经在这份里了，没有存第二遍」，
// **把人支去改另一条**。症状是"存了两次同一个网站，第二次说存过了"，
// 而两条根本不是同一个页面。
//
// 判据：**地址不同就该是不同的一条**，所以 query 与 hash 都要进 id。
// 取法仍是从地址派生（不拿名字），只是把 `?`/`#` 之后那段也算进去。
function idFor(url, name) {
  let base = '';
  try {
    const u = new URL(url);
    // **裸 IP 走哈希**（2026-10-19）：与 app.js 的 `slug(text, true)` 同一句判据。
    //
    // 从前这里一律取 `hostname` 的第一段，于是 `10.0.2.3:6789` 与
    // `10.0.2.3:7080` **全部**变成 `10-…`——加第二个内网服务就撞 id，而撞 id 是
    // 硬错误（duplicate-id），症状是「明明填对了地址却存不进去」。
    //
    // 而更早一版是整串 ascii，得到 `192-168-2-36789` 那种：它把**内网地址与端口
    // 烘进了 id**，而 `GET /api/v1/export` 发的是磁盘原字节——**导出文件发给别人
    // 就等于把这段拓扑一起发出去**。app.js 那边早就是哈希（见它 `slug()` 里
    // `isIPv4` 那一支），这里漏了，所以扩展存的内网书签会带着拓扑流出去。
    //
    // 判据是「四段全是数字」而不是「含数字」——`10.0.2.3.example.com` 那种仍走
    // 可读那条，而它不含任何私密信息。
    //
    // **路径、query、hash 也算进去**（2026-10-26）：从前只哈希主机+端口，于是
    // 同一个 NAS 端口下的 `/photo` 与 `/music` 派生出同一个 id，第二条被答成
    // `duplicate-id`（「这一条已经在这份里了」——假话）。与 app.js 同一句判据。
    const host = u.hostname.replace(/^www\./i, '');
    if (/^\d{1,3}(\.\d{1,3}){3}$/.test(host)) {
      return 'ip-' + (hash36(host + (u.port || '') + u.pathname + (u.search || '') + (u.hash || '')) || 'item');
    }
    // **整条路径，不只末段**（2026-10-20 修）：从前只取末段，于是
    // `github.com/userA/project` 与 `github.com/userB/project` 派生出同一个 id
    // （github-project）——撞 id 是硬错误（duplicate-id），而扩展把它翻成
    // 「这一条已经在这份里了」，那是**假话**（两个页面完全不同）。
    //
    // query 与 hash 同样算进去：路径末段相同的两个页面（两个视频、两篇文章）
    // 靠这段分开（2026-10-19 修）。
    //
    // **这一段与 app.js 的 `slug(url, true)` 是同一条判据**（2026-10-20 两边对
    // 上）：同一个地址在页面与扩展上必须得到**同一个 id**，否则同一条书签会被
    // 存成两条。
    base = host.split('.')[0] + '-' +
           (u.pathname.split('/').filter(Boolean).join('-') || '') +
           (u.search || '') + (u.hash || '');
  } catch { base = ''; }
  base = base.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
  // **超长要加哈希后缀，上限是 32**（2026-10-20 修）：从前这里是 40，而服务端
  // 收的是 32（model.ValidSlug：`^[a-z0-9][a-z0-9-]{0,31}$`）——于是凡是派生 id
  // 超过 32 的地址，**扩展一条书签都存不进去**（整批 422），而地址本身一点问题
  // 都没有。app.js 那边 2026-10-16 就修好了（它的 `ascii()` 用 SLUG_MAX=32），
  // 这里跟上——**两边是同一族判据，改的时候记得两处都要看**。
  if (base.length > 32) {
    const h = hash36(base);
    base = base.slice(0, 32 - h.length - 1) + '-' + h;
  }
  // 兜底那一条也要去掉首尾的连字符：slug 的首字符必须落在 [a-z0-9]。
  return base || String(name || url).toLowerCase().replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '').slice(0, 32) || 'item';
}

// hash36 是 app.js 那份的同款（FNV-1a 的 32 位变体，转 36 进制）。
function hash36(text) {
  let h = 2166136261;
  for (let i = 0; i < text.length; i++) h = (h ^ text.charCodeAt(i)) * 16777619 >>> 0;
  return h.toString(36);
}

// ── 菜单 ────────────────────────────────────────────────────────────────────
// 文案**只有一份**：以前 onInstalled 与 refreshMenus 各写一遍同样的两条，
// 改一处就会漂移（而菜单文案是这个扩展里唯一天天被看的东西）。
// 动词也统一成「存」：批量那条从前写「收藏这个窗口的所有标签页」，
// 与单条的「存到 launchbay」是同一个动作的两套词。
const MENUS = [
  { id: 'lb-save', title: t('menuSave'), contexts: ['page', 'link'] },
  { id: 'lb-save-window', title: t('menuSaveWindow'), contexts: ['page'] },
];

async function refreshMenus() {
  await chrome.contextMenus.removeAll().catch(() => {});
  for (const m of MENUS) await chrome.contextMenus.create(m);
}

// onInstalled 只在**首次安装**时跑一次，扩展更新后菜单不会重建——那会让用户
// 更新一版之后右键菜单就没了，而且没有任何报错。所以三个时机都刷一遍，
// 而它们跑的是同一份 MENUS。
chrome.runtime.onInstalled.addListener(refreshMenus);
chrome.runtime.onStartup.addListener(refreshMenus);
refreshMenus();

chrome.contextMenus.onClicked.addListener(async (info, tab) => {
  const c = await cfg();
  const profile = currentProfile(c);
  const fail = (message, where) => tellNote(t('failTitle'), message, profile, where);
  if (info.menuItemId === 'lb-save') {
    const page = info.linkUrl ? { url: info.linkUrl, title: info.linkUrl } : { url: tab.url, title: tab.title };
    try {
      const r = await saveOne(page, profile);
      if (r) throw Object.assign(new Error(r.why || t('alreadySaved')), { where: r.where });
      await tellOK(t('badgeSaved'));
    } catch (e) {
      await fail(e.message, e.where);
    }
    return;
  }
  if (info.menuItemId === 'lb-save-window') {
    // 批量：**一条一条存**，且第一条失败就把后面全停下来。
    // 全塞进一批的话，一条坏的就让这一批全不落——而用户看到的是"收藏了 12 个，
    // 回来发现一个都没存上"，比停在第 7 条难查得多。
    // 「本来就在这份里」的那几条**不算失败、跳过接着存**（见 saveOne 的三态）。
    const pages = (await chrome.tabs.query({ currentWindow: true })).filter((p) => p.url && /^https?:/.test(p.url));
    let ok = 0, dup = 0, firstErr = null, firstErrWhere = null;
    for (const p of pages) {
      try {
        const r = await saveOne({ url: p.url, title: p.title }, profile);
        if (!r) { ok++; continue; }
        if (r.dup) { dup++; continue; }
        if (!firstErr) { firstErr = r.why; firstErrWhere = r.where; }
        break;
      } catch (e) { if (!firstErr) { firstErr = e.message; firstErrWhere = e.where; } break; }
    }
    const left = pages.length - ok - dup;
    if (!firstErr) {
      // 全是重复的那种"什么都没发生"必须说出来：角标亮一个 0 会被读成坏了，
      // 而它其实是最正常的一种结果（这一窗刚存过）。
      // 角标只有那么宽：三位以上收在 99+，与主站那一族一致。
      if (ok) { await tellOK(t('badgeSavedN', ok > 99 ? '99+' : ok)); return; }
      await tellNote(t('failTitle'), t('nothingNew', pages.length), profile, 'page');
      return;
    }
    // "停在第 7 条"这句话必须连"后面还有 5 条没存"一起说：只报已存数会让人
    // 以为这一窗就是 7 个标签页。已经存过的那几条也要点一句，否则那 5 条里
    // 有 2 条是"本来就有的"，剩下的数目对不上。
    const lines = [t('stoppedAt', ok, firstErr), t('windowTotals', pages.length, left)];
    if (dup) lines.push(t('skippedDup', dup));
    await fail(lines.join('\n'), firstErrWhere);
  }
});
