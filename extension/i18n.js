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

// ── 失败怎么说：按**错误码（kind）**，不按状态码 ────────────────────────────
//
// 状态码答不了"该做什么"。422 同时是"这一条已经存过了"（别存第二遍，去改它）与
// "那一组已经不在了"（刷新再看一次）——两句出路相反，而按状态码选句子时它们只能
// 共用一句。实测批量收藏撞上一个重复网址，通知里写的是"东西没了，刷新一下"，
// 人照着刷新，问题一个字都没少。
//
// 服务端现在每个 JSON 错误都带 kind（AGENTS.md「错误码」），这一侧从码出句子。
// **状态码那一张表只留给版本差**：扩展可能比那台机器旧，遇到不认识的码时
// 退回按状态码说一句大致对的话，也比把服务端的协议原话贴进通知好。
//
// 表里的 value 是目录里的 key；闸在 internal/i18n/extcatalog_test.go：
// 服务端登记的每一个 kind 都要在这儿有句子，或者在测试那份"收不到"的清单里
// 写着为什么收不到。
const KIND = {
  'credential-missing': 'noToken',
  'credential-unknown': 'err401',
  'credential-scope': 'err403',
  'credential-admin-only': 'errAdminOnly',
  'profile-not-found': 'err404',
  'profile-broken': 'errBroken',
  'profile-disabled': 'errDisabled',
  'duplicate-id': 'errDuplicate',
  'missing-target': 'errGoneTarget',
  'icon-unknown': 'errIconName',
  'url-incomplete': 'errBadUrl',
  'rev-conflict': 'errConflict',
  'snapshot-failed': 'errNoSnap',
  'ops-invalid': 'errOpsBatch',
  'invalid-request': 'errOpsBatch',
  // 值本身不收（名字过长、地址缺主机名那一类）。扩展这一侧组装得了请求体，
  // 但**值是从网页与标签页标题来的**，所以这一码真收得到——那句"得在页面上改"
  // 就是它的出路。
  'field-invalid': 'errOpsBatch',
  'import-no-bookmarks': 'errNoBookmarks',
  'import-nothing-accepted': 'errNothingAccepted',
  'import-plan-stale': 'errPlanGone',
};

const STATUS = {
  401: 'err401',
  403: 'err403',
  404: 'err404',
  409: 'errConflict',
  410: 'errPlanGone',
  422: 'errOpsBatch',
  507: 'errNoSnap',
};

// kindOf：这一发该按哪一码说话。batch 与 import 的外壳 kind 说的是**这一族**
// （ops-invalid = 整批未写入），具体哪一条、为什么在 errors[0] —— 所以先读明细。
// 与主站 app.js 的 msgKeyFor 同一口径。
export function kindOf(body) {
  const first = Array.isArray(body && body.errors) && body.errors[0];
  return (first && first.kind) || (body && body.kind) || '';
}

// serverNote：服务端那句协议原文（detail / errors[].reason）。它**不进通知**——
// 那一句是给脚本对照的中文黑话（AGENTS.md：`/api/v1/*` 的 detail 固定中文），
// 而扩展唯一的展示面是一条人正在读的消息。它进 console：排查的人要原话。
export function serverNote(body) {
  const first = Array.isArray(body && body.errors) && body.errors[0];
  return (first && (first.reason || first.field)) || (body && body.detail) || '';
}

// httpErr：把一次失败变成人能照做的一句话。**这一张表是全扩展唯一的一份**——
// bg.js 的右键收藏、options 页的导入两次请求走的是同一条流水线，同一个错误在
// 两处说两种话，人就照着最后一句查。
//
// `body` 是解析过的响应（认不出来就传 `{}`）；`profile` 给那几句需要点名档案的话。
export function httpErr(res, body, profile) {
  const key = httpErrKey(res, body);
  return key === 'errServer' ? t('errServer', res.status) : t(key, profile);
}

// httpErrKey：那句人话是**按哪个 key 说的**。调用方要它，是因为"点这条通知该
// 落到哪儿"跟着的是出路而不是那句话的译文——拿译文去比字符串，等于把判据建在
// 两份语言包的字面上（换一句措辞就悄悄失效）。
export function httpErrKey(res, body) {
  return KIND[kindOf(body)] || STATUS[res.status] || 'errServer';
}

// NOTE_TARGET：通知点下去落在哪儿，**按出路分**。
//
// 从前 bg.js 的 onClicked 无条件 openOptionsPage()，而 errDuplicate /
// errGoneTarget / errOpsBatch / errIconName 那几句教的是"去页面上在那一条上
// 按编辑"——人照着做，点一下通知却落到设置页。一句说得出出路的失败，
// 它自己的可点部分不许把人说走。
//
// 'settings' = 这台机器还没配对（地址、令牌、档案本身不在了）；
// 'page'     = 配对是对的，要做的事在导航页上（改那一条、建那个分组、刷新再看）。
// 这张表由 internal/i18n/extnotify_test.go 逐 key 钉住：KIND 与 STATUS 里
// 每一个值都要在这儿有一格，漏一个就是那条出路又变回"点了不知道去哪"。
export const NOTE_TARGET = {
  noBase: 'settings',
  noToken: 'settings',
  noProfileToken: 'settings',
  needToken: 'settings',
  err401: 'settings',
  err403: 'settings',
  err404: 'settings',
  errAdminOnly: 'settings',
  errBroken: 'settings',
  errDisabled: 'settings',
  errServer: 'settings',
  netFail: 'settings',
  errNoBookmarks: 'settings',
  errPlanGone: 'settings',
  noGroup: 'page',
  errDuplicate: 'page',
  errGoneTarget: 'page',
  errIconName: 'page',
  errBadUrl: 'page',
  errConflict: 'page',
  errNoSnap: 'page',
  errOpsBatch: 'page',
  errNothingAccepted: 'page',
};

export function noteTarget(key) {
  return NOTE_TARGET[key] || 'settings';
}
