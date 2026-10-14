// sw.js —— P1-⑥ 的 Service Worker。
//
// **它读不到 cookie**（fetch 事件里 request.headers 不含 Cookie），所以它自己
// 选不出档案。服务端因此在每个 HTML 响应上带 `X-LB-Profile: <id|picker|-admin>`
// （§7.1），SW 拿那个头决定**存进哪个桶**。三个值的处置各不相同，而它们的差别
// 就是本文件最重要的一段逻辑：
//
//   html-<id>     某个人的档案页 —— 存。它是离线时唯一真正有用的东西。
//   html-picker   档案选择页 —— 存。它本来就只列档案名字（§8.0：陌生人打开
//                 首页看到的就是那些名字），所以离线时能回退到它不比在线时多
//                 泄露什么；反过来，"连当前档案都不知道"的机器上，它是唯一
//                 还能把人送进某个档案的入口。
//   html--admin   管理页 —— **不存**。它的哨兵名是 `-admin`（model.ValidSlug
//                 拒收的形状，render.AdminSentinel 的注释解释了为什么），
//                 而它含全家庭的档案名 + 条目数 + rev + 快照数：那比选择页敏感
//                 一档，缓存它等于把全家概况留在共用设备上。
//
// HTML 走 **network-first，1.5 秒超时回退缓存**（§7.1）：局域网里网络几乎总通，
// 所以"永远最新"是对的；NAS 挂了才吃缓存。这条专门对付"SW 把含数据的 HTML 缓存
// 住，于是看到旧书签"——那种 bug 是本设计里最贵的一类。
//
// **不静默 skipWaiting**（§7.1）：检测到新版本时不抢占，等页面自己决定。

const VERSION = 'lb-sw-2';
// 代次从 1 跳到 2 的原因（2026-10-15）：上面那条「面板 fetch 不进资产缓存」是
// 这一次才补的，而**已经被钉住的那一份还在 lb-sw-1 里**。按 §7.1 的规矩，sw.js
// 变了就得换代次——activate 那一支只清 assets-<别的代次>，于是旧桶连同那份
// 陈旧的面板 HTML 一起被删掉。不换代次的话，那份脏数据会一直躺在设备上，
// 而将来谁把判断顺序调回去，它会立刻变成同一个 bug。
const ASSET_CACHE = 'assets-' + VERSION;
// 超时：够 LAN 里一次往返，不够等到用户以为它死了。
const HTML_TIMEOUT_MS = 1500;

// ── IndexedDB：当前档案 ─────────────────────────────────────────────────────
// 为什么放这里而不是 localStorage：IndexedDB 是这一族里唯一能被页面与 SW
// **可靠地**同时读写的存储；localStorage 在 SW 上下文里虽有，但它是同步 API，
// 每次读写都阻塞，而这份数据每个 HTML 响应都要动一次。
const DB = 'lb-offline', STORE = 'kv';

function idb() {
  return new Promise((res, rej) => {
    const r = indexedDB.open(DB, 1);
    r.onupgradeneeded = () => r.result.createObjectStore(STORE);
    r.onsuccess = () => res(r.result);
    r.onerror = () => rej(r.error);
  });
}

async function kvGet(k) {
  try {
    const db = await idb();
    return await new Promise((res, rej) => {
      const t = db.transaction(STORE, 'readonly').objectStore(STORE).get(k);
      t.onsuccess = () => res(t.result);
      t.onerror = () => rej(t.error);
    });
  } catch { return undefined; } // 拿不到就当没有：回退到 html-picker
}

async function kvSet(k, v) {
  try {
    const db = await idb();
    await new Promise((res, rej) => {
      const t = db.transaction(STORE, 'readwrite').objectStore(STORE).put(v, k);
      t.onsuccess = res;
      t.onerror = () => rej(t.error);
    });
  } catch { /* 存不进去不影响在线功能，只是离线回退退粗了 */ }
}

// ── install ────────────────────────────────────────────────────────────────
// 预缓存清单**拿不到**：指纹路径是服务端的常量，而 install 那一刻只有 URL，
// 没有页面。所以清单由页面装好之后 postMessage 送来（见下面的 message）。
// 这里只等自己装完，不 skipWaiting。
self.addEventListener('install', (e) => { e.waitUntil(Promise.resolve()); });

async function precache(urls) {
  if (!urls || !urls.length) return;
  const c = await caches.open(ASSET_CACHE);
  // 逐个 add 而不是 addAll：addAll 里任何一条失败就整体 reject，于是"某张图标
  // 404"会让整份预缓存作废——而预缓存失败的后果是整个离线能力没有，不是少一张图。
  await Promise.all(urls.map((u) => c.add(u).catch(() => {})));
}

// ── activate ──────────────────────────────────────────────────────────────
// **只清 assets- 的旧版本，不清 html- 的桶**：那些桶是用户正在离线看着的东西，
// "删掉一个离线副本"比"多留一个"更让人意外。而 html- 的桶名里**本来就没有版本号**
// （`html-<profileId>`），所以"跨版本残留"这件事对它们根本不成立。
//
// 一条**已知边界**（spec §15 记着）：某个档案被删掉之后，它的 `html-<id>` 桶会留在
// 设备上。清它需要"这台机器现在还有哪些档案"这份清单，而这份清单只有选择页拿得到
// ——选择页是零 JavaScript 的（§8.1），给它挂脚本只为发一条清单不划算；档案页自己
// 则只知道它是谁。所以这个清理现在没有干净的落点。
//
// **这里如实写明，而不是承诺一个不存在的函数**：本文件早先写过一句"由页面启动时
// 按 id 清（见 app.js 的 pruneStaleBuckets）"，而那样的函数在 app.js 里并不存在。
// 一条注释承诺了一件没做的事，比不写更坏——后来的人会照着它去找一个死胡同。
self.addEventListener('activate', (e) => {
  e.waitUntil((async () => {
    for (const k of await caches.keys()) {
      if (k.startsWith('assets-') && k !== ASSET_CACHE) await caches.delete(k);
    }
    // 刻意不调 skipWaiting：§7.1 要求"检测到 rev 变化不静默更新，顶栏出提示"，
    // 而静默更新的开关就在这一行。
  })());
});

// ── 页面来的消息 ───────────────────────────────────────────────────────────
self.addEventListener('message', (e) => {
  const d = e.data || {};
  if (d && d.type === 'precache') {
    e.waitUntil(precache(d.urls));
    return;
  }
  // 切档/退出：改写"当前档案"并清掉上一个档案的 HTML 桶（§7.1 末条）。
  // **两个动作必须同批做**：只改记录会留下旧档案的缓存（下次离线回退打错人），
  // 只清缓存会让离线回退找不到任何东西。
  if (d && d.type === 'switch' && d.from) {
    e.waitUntil((async () => {
      if (d.to) await kvSet('currentProfile', d.to);
      await caches.delete('html-' + d.from);
    })());
  }
});

// ── fetch ─────────────────────────────────────────────────────────────────
self.addEventListener('fetch', (e) => {
  const req = e.request;
  if (req.method !== 'GET') return;
  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return; // 外面的交给浏览器

  // **接口一律不缓存**（不 respondWith，交给浏览器直连）。cache-first 对
  // /api/v1/data 的含义是"写进去之后读回来还是旧的那份"，而 app.js 在补存离线
  // 队列之前正是靠这一条取最新 rev：读到旧值必然撞 409，那个 409 说的不是
  // "别人改过了"而是"你自己过期了"，队列里的条目会被当冲突逐条丢掉。
  // 实测过这一桩：磁盘上 rev 已经是 3，页面里 fetch('/api/v1/data') 仍回 rev 1。
  if (url.pathname.startsWith('/api/')) return;

  // 导航请求 = HTML。判据是 mode 而不是 URL 形状：首页只有 "/"，而它是档案页、
  // 选择页、引导页、坏档页共用的一条路径——靠 URL 分不出它是哪一张。
  if (req.mode === 'navigate') {
    e.respondWith(htmlFirst(req));
    return;
  }

  // **面板那一路 fetch 不是导航**（2026-10-15 修）。app.js 的 prefsOpen 注入设置
  // 面板时用的是 `fetch('/prefs', {Accept: 'text/html'})`——同源 fetch，mode 不是
  // navigate，于是从前落到下面那条 cacheFirst。而 ASSET_CACHE 那一族是**指纹
  // 资源**的缓存（immutable），存进去的东西按设计永远不过期，于是面板**第一次
  // 取到的那一份被永久钉住**。
  //
  // 症状（实测）：改完一个颜色再打开设置，屏上还是第一次打开时的颜色——而
  // `GET /api/v1/data` 与盘上都是新值。于是"设置改了没生效"与"按了没反应"
  // 在同一件事上同时成立，而它们指向的都不是原因。
  //
  // **判据是 Accept 而不是 URL 形状**：与上面那条「mode 而不是 URL 形状」同一条
  // 纪律——`/prefs` 只是眼下唯一一处，而将来任何一段"取一段 HTML 注入页面"
  // （导入报告、坏档说明）都该走同一条路。
  //
  // **不接管**（而不是给它一条 network-first）：网络不通时 fetch 会抛，
  // prefsOpen 的 catch 已经写着 `location.href = '/prefs'` —— 那是一次**导航**，
  // 于是自动落到上面那条 htmlFirst，仍然有离线回退。两条路各管一段。
  if ((req.headers.get('Accept') || '').includes('text/html')) return;

  e.respondWith(cacheFirst(req));
});

async function htmlFirst(req) {
  try {
    const res = await withTimeout(fetch(req), HTML_TIMEOUT_MS);
    if (res && res.ok) await storeHTML(req, res.clone());
    return res;
  } catch {
    // 网络不通**或**超时：回退缓存。这不是"错误"，是离线态——所以不报错，
    // 只把缓存里那份交出去（离线时页面顶端会显示"离线·只读"，§8.7）。
    for (const key of [await kvGet('currentProfile'), 'picker']) {
      if (!key) continue;
      const hit = await (await caches.open('html-' + key)).match(req);
      if (hit) return hit;
    }
    // 连当前档案都没有（本设备从没在线成功过）→ 选择页，这正是 spec §7.1
    // 点名的那一档。
    throw new Error('offline and nothing cached');
  }
}

async function storeHTML(req, res) {
  const id = res.headers.get('X-LB-Profile') || '';
  // 哨兵 -admin 一律不存（见文件头）。空头也不存：不知道是哪一份的页面放进
  // 哪个桶，就是下一次离线回退打错人的那条路。
  if (!id || id === '-admin') return;
  await kvSet('currentProfile', id);
  await (await caches.open('html-' + id)).put(req, res);
}

async function cacheFirst(req) {
  const c = await caches.open(ASSET_CACHE);
  const hit = await c.match(req);
  if (hit) return hit;
  const res = await fetch(req);
  if (res && res.ok) c.put(req, res.clone());
  return res;
}

function withTimeout(p, ms) {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('timeout')), ms);
    p.then((v) => { clearTimeout(timer); resolve(v); },
           (e) => { clearTimeout(timer); reject(e); });
  });
}