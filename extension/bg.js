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

// api 打这台机器。**每一处失败都要变成人话**：扩展弹一下就没了，报错写在
// console 里的话，点了没反应的人根本不会去看那里。
async function api(c, path, body, profile) {
  const res = await fetch(c.base + path, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': 'Bearer ' + (c.tokens[profile] || ''),
    },
    body: JSON.stringify(body),
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    // batch 的 422 说的是"ops 校验未过，整批未写入"，那是协议话；真正的原因
    // 在 errors[0].reason。与 app.js 那侧同一条口径。
    const first = Array.isArray(data.errors) && data.errors[0];
    const why = (first && (first.reason || first.field)) || data.detail || `HTTP ${res.status}`;
    throw new Error(why);
  }
  return data;
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

async function tellFail(message, profile) {
  // 角标仍然亮一下，但只放一个符号——它是"出事了"的信号，不是消息本体。
  await chrome.action.setBadgeText({ text: '!' });
  await chrome.action.setBadgeBackgroundColor({ color: '#b3261e' });
  setTimeout(() => chrome.action.setBadgeText({ text: '' }), 6000);
  const body = profile ? `${message}\n（当前要存的档案：${profile}）` : message;
  try {
    await chrome.notifications.notify({
      id: 'lb-fail',
      title: '没存进 launchbay',
      message: body,
      iconUrl: 'icons/32.png',
      priority: 2,
    });
  } catch {
    // 系统通知被关掉（专注模式、Windows 的"通知已关闭"）是常态而不是异常：
    // 那时候至少把句子留在控制台，别让"点了没反应"变成三件事叠在一起。
    console.error('launchbay 扩展：', message);
  }
}

chrome.notifications?.onClicked?.addListener((id) => {
  if (id === 'lb-fail') chrome.runtime.openOptionsPage();
});

async function saveOne(page, profile) {
  const c = await cfg();
  if (!c.base) return '还没配这台机器的地址（扩展设置页第一格）';
  if (!profile || !c.tokens[profile]) return '这个档案还没有令牌';
  // 先 resolve 一次：它把标题、描述、建议分组与图标候选都填好，而服务端据此
  // 拒掉/收下每一条。**不要在扩展里自己造分组** —— 那是服务端那份映射的职责，
  // 两处各判一次就会分家。
  let picked = {};
  try {
    const r = await api(c, '/api/v1/resolve', { url: page.url, profile }, profile);
    picked = {
      name: r.title || page.title || page.url,
      desc: r.desc || '',
      icon: (r.iconCandidates && r.iconCandidates[0]) || '',
      // suggestedGroup 以前被**整个丢掉**：这里构造 picked 时没带它，下面
      // `picked.group` 于是恒为 undefined，服务端算好的建议分组一次都没用上，
      // 每条都落进"第一个分组"。修法是把它接上，而不是在扩展里另猜一套。
      group: r.suggestedGroup || '',
    };
  } catch {
    // resolve 抓不到（内网、已失效、要登录）不算失败：标题退回标签页标题，
    // 其余留空，让人在界面上补。
    picked = { name: page.title || page.url, desc: '', icon: '', group: '' };
  }

  const cur = await (await fetch(c.base + '/api/v1/data?profile=' + encodeURIComponent(profile),
    { headers: { Authorization: 'Bearer ' + c.tokens[profile] } })).json();

  const groups = (cur && cur.data && cur.data.groups) || [];
  const wanted = groups.find((g) => g.id === picked.group);
  const gid = (wanted && wanted.id) || (groups[0] && groups[0].id) || '';
  if (!gid) return '这个档案一个分组都没有，先在页面上建一个分组再收藏';

  const value = { id: idFor(page.url, picked.name), name: picked.name, url: page.url };
  if (picked.desc) value.desc = picked.desc;
  if (picked.icon) value.icon = picked.icon;

  await api(c, '/api/v1/batch', {
    profile, expectedRev: cur.rev, by: 'extension',
    ops: [{ op: 'add', path: 'groups/' + gid + '/items', value }],
  }, profile);
  return null;
}

// 条目 id 从**地址**派生：名字会被 resolve 填、也会被手改，拿它当 id 意味着
// 改一次名字这条的 id 就变了。形状与 app.js 的 slug() 是同一族（两边各自
// 实现过一次，改的时候记得两处都要看——这是已知代价，不是疏忽的理由）。
function idFor(url, name) {
  let base = '';
  try {
    const u = new URL(url);
    base = u.hostname.replace(/^www\./i, '').split('.')[0] + '-' +
           (u.pathname.split('/').filter(Boolean).pop() || '');
  } catch { base = ''; }
  base = base.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 40);
  return base || String(name || url).toLowerCase().replace(/[^a-z0-9]+/g, '-').slice(0, 40) || 'item';
}

// ── 菜单 ────────────────────────────────────────────────────────────────────
// 文案**只有一份**：以前 onInstalled 与 refreshMenus 各写一遍同样的两条，
// 改一处就会漂移（而菜单文案是这个扩展里唯一天天被看的东西）。
// 动词也统一成「存」：批量那条从前写「收藏这个窗口的所有标签页」，
// 与单条的「存到 launchbay」是同一个动作的两套词。
const MENUS = [
  { id: 'lb-save', title: '存到 launchbay', contexts: ['page', 'link'] },
  { id: 'lb-save-window', title: '把这一窗的标签页全存进去', contexts: ['page'] },
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
  if (info.menuItemId === 'lb-save') {
    const page = info.linkUrl ? { url: info.linkUrl, title: info.linkUrl } : { url: tab.url, title: tab.title };
    try {
      const err = await saveOne(page, profile);
      if (err) throw new Error(err);
      await tellOK('已存');
    } catch (e) {
      await tellFail(e.message, profile);
    }
    return;
  }
  if (info.menuItemId === 'lb-save-window') {
    // 批量：**一条一条存**，且第一条失败就把后面全停下来。
    // 全塞进一批的话，一条坏的就让这一批全不落——而用户看到的是"收藏了 12 个，
    // 回来发现一个都没存上"，比停在第 7 条难查得多。
    const tabs = (await chrome.tabs.query({ currentWindow: true })).filter((t) => t.url && /^https?:/.test(t.url));
    let ok = 0, firstErr = null;
    for (const t of tabs) {
      try {
        const err = await saveOne({ url: t.url, title: t.title }, profile);
        if (err) { firstErr = firstErr || err; break; }
        ok++;
      } catch (e) { firstErr = firstErr || e.message; break; }
    }
    const left = tabs.length - ok;
    if (!firstErr) {
      await tellOK('已存' + (ok > 99 ? '99+' : ok));
      return;
    }
    // "停在第 7 条"这句话必须连"后面还有 5 条没存"一起说：只报已存数会让人
    // 以为这一窗就是 7 个标签页。
    await tellFail(
      `存到第 ${ok} 条时停了：${firstErr}\n这一窗共 ${tabs.length} 个网页，还有 ${left} 个没存。`,
      profile);
  }
});
