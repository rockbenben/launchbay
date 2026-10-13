// i18n.js —— 扩展取文案的唯一入口：`_locales/<浏览器界面语言>/messages.json`。
//
// 为什么是一份而不是三份：bg.js（service worker）与 options 页里的 import.js /
// options.js 说的是同一件事——同一枚令牌、同一个档案、同一次失败——句子就该同
// 一条。四份文件因此都是 **ES module**：manifest 的 background 写了
// `"type": "module"`，options.html 用 `<script type="module">`。
// 这不是风格选择：MV3 的 service worker **没有 `importScripts()`**，
// 而 `chrome.i18n` 只在扩展自己的上下文里查得到——两头都只能用模块语法接上
// （实测：经典脚本里 importScripts 直接把整个 bg.js 抛掉，症状是右键菜单整个不见、
// 通知里全是 ⟨key⟩，而且没有任何可见的报错）。
//
// 认不出 key 时 `getMessage` 回**空串**，而空白的界面看着像设计如此；所以让它
// 长成 ⟨key⟩，与主站 `T` 的缺 key 形状一致——漏翻在页面上要看得见。
export function t(key, ...args) {
  // Chrome 的绑定只收字符串或字符串数组：计数（书签条数、标签页数）直接递进来
  // 会变成 number，那一发不是"少个引号"而是整句拿不到。
  const s = chrome.i18n.getMessage(key, args.map((a) => String(a)));
  return s || '⟨' + key + '⟩';
}

// httpErr：状态码 → 人能照做的一句话。**这一张表是全扩展唯一的一份**——
// bg.js 的右键收藏、options 页的「试一下这台机器」与 import.js 的两次请求走的是
// 同一条流水线，同一个 403 在三处说三种话，人就照着最后一句查。
// 401 = 这枚令牌不被认，403 = 令牌与档案对不上，404 = 没这个档案。
// 422 是唯一的例外：那是这台机器**对这一条本身**说出的话（最常见的两种是
// 「这一条已经在里面了」与「图标名不在索引里」），除了把它原样带出来没有别的
// 信息可给——所以它是唯一进入通知的服务端文本。
export function httpErr(res, reason, profile) {
  switch (res.status) {
    case 401: return t('err401');
    case 403: return t('err403', profile);
    case 404: return t('err404', profile);
    case 409: return t('errConflict');
    case 410: return t('errPlanGone');
    case 422: return reason ? t('errRejected', reason) : t('errServer', res.status);
    case 507: return t('errNoSnap');
    default: return t('errServer', res.status);
  }
}
