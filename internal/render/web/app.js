// app.js —— P1-⑤ 的第一段真字节。
//
// 只有一件事：**顶栏「＋ 添加」与它的浮层**（spec §8.1「本次要解决的具体抱怨」
// 与 §8.5 的形状）。刻意只做一件事：第一个真字节越小，"关掉 JS 还能不能用"这件事
// 越容易用肉眼验收，而它正是 §7.2 那句判据的落点。
//
// 零依赖、零框架、ES module。理由与后端同一条：depguard 守着整个仓库零外部依赖，
// 而一个前端框架会让"构建期零 node"（§10）当场失效。
//
// **按钮由本文件创建，不由模板渲染**——所以关掉 JS 时顶栏不会出现一个点了没反应的
// 东西。§8.1 的原话是"点了没反应的东西不进界面"，这比用 `<noscript>` 藏起来更
// 干脆：没有 JS 就没有这个功能，而书签仍全部可见可点（§7.2）。
//
// 所有写入都经 `POST /api/v1/batch`（§6.1 的唯一写通路）。**不直接碰 data/**：
// 快照、历史、rev 递增都在那条通路内部发生，绕过它等于造出第二份真相。



// BEGIN APP_I18N — 这一块**由 internal/i18n/messages/*.json 生成，不是手抄的第二份**。
// 判据只有一份：语言包是源，这里是它 app.* 前缀的切片。
// appi18n_test.go 逐条比对两者并扫残余的中文硬编码，两边不同步就红。
// 加一条 app 文案的动作：只在语言包里加，然后跑 go test ./internal/render。
const APP_I18N = {
  "zh": {
    "app.add": "＋ 添加",
    "app.addTitle": "添加书签（按 n）",
    "app.sheetTitle": "添加书签",
    "app.field.url": "地址",
    "app.field.title": "名字",
    "app.field.desc": "描述",
    "app.field.group": "分组",
    "app.field.icon": "图标",
    "app.ph.title": "网站标题，可改",
    "app.ph.desc": "一句话说明，可不填",
    "app.ph.group": "新分组叫什么",
    "app.icon.none": "不选图标（用名字第一个字）",
    "app.icon.auto": "抓这个网站的图标",
    "app.icon.currentSuffix": "（现在的）",
    "app.icon.fetch": "去看这一站的图标",
    "app.icon.lib": "图标库…",
    "app.icon.libTitle": "从图标库里挑一个",
    "app.icon.libPh": "搜名字，比如 docker、github 或 tabler:box",
    "app.icon.libHint": "图标库是随程序一起装上的那一份：品牌图标（simple-icons）与概念图标（tabler）。",
    "app.icon.libStart": "在上面搜一个名字，图标就出现在这里。",
    "app.icon.libNone": "图标库里没有叫「%s」的",
    "app.icon.libMore": "共 %s 个，先看前 %s 个",
    "app.icon.libFail": "图标库没查到（这一格连不上，稍后再试一次）",
    "app.save": "保存",
    "app.saveMore": "保存并继续",
    "app.close": "关闭",
    "app.reading": "正在读取那个网页…",
    "app.readFail": "没读到东西，标题可以自己填",
    "app.readOk": "已读到标题、说明和图标",
    "app.readEmpty": "这个网页没给出可读的信息，标题自己填",
    "app.readErr": "读不出来：%s",
    "app.err.group": "先选一个分组。",
    "app.err.groupName": "先给这个分组起个名字。",
    "app.err.url": "先填地址。",
    "app.offline.queued": "离线了，这条先记下，网络通了自动补上。",
    "app.offline.chip": "离线记了 %d 条",
    "app.offline.aria": "看离线记下的条目",
    "app.offline.drop": "不要这条",
    "app.offline.list": "下面这些是离线时记下的，网络通了会自动补进去。",
    "app.offline.blocked": "现在没网，只能看。添加会先记下来，网络通了自动补进去。",
    "app.offline.done": "离线记下的 %d 条已经补进去",
    "app.offline.dropped": "这 %d 条和别人改的撞上了，没自动合并，请重新加一次：%s",
    "app.offline.left": "还有 %d 条没存进去：%s",
    "app.saving": "正在保存…",
    "app.saved": "已保存。",
    "app.conflict": "这一份已经被改过（可能另一边也动了）。刷新后再来一次。",
    "app.cmdk.placeholder": "搜书签，或做点什么",
    "app.cmdk.more": "在整页里搜「%s」· 一共 %d 条命中",
    "app.cmdk.aria": "命令面板",
    "app.cmdk.clipboard": "把剪贴板里的网址存进来",
    "app.cmdk.history": "看这一份的改动历史",
    "app.cmdk.kind": "动作",
    "app.cmdk.none": "没有对得上「%s」的书签，也没有对得上的动作。",
    "app.cmdk.empty": "这里还没有可列的东西。",
    "app.clip.denied": "这个浏览器不给读剪贴板，手动贴一下地址。",
    "app.clip.noturl": "剪贴板里那段不像一个网址，手动贴一下。",
    "app.clip.noread": "读不到剪贴板（系统没给这个权限），手动贴一下地址。",
    "app.copy.aria": "复制「%s」的链接",
    "app.copy.done": "已复制",
    "app.copy.fail": "复制不了，已经帮你把地址选中了，按 ⌘ 或 Ctrl + C",
    "app.edit.aria": "改「%s」",
    "app.edit.fallbackName": "这一条",
    "app.layout.tiles": "磁贴",
  "app.group.new": "＋ 新分组…",
  "app.group.shape": "这一组用",
    "app.layout.rows": "单列",
    "app.layout.feature": "大卡",
    "app.layout.featureTitle": "挪到页面顶部独占一块：卡片更大、名字下面是主机名。这一组不再出现在正文的分组序列里",
    "app.layout.aria": "把「%s」这一组从%s改成%s",
    "app.multi.aria": "批量操作",
    "app.multi.move": "移动到（%d）",
    "app.multi.tags": "打标签（%d）",
    "app.multi.delete": "删掉（%d）",
    "app.multi.none": "全不选",
    "app.multi.exit": "退出",
    "app.multi.hint": "已经在多选里：点卡片选上或取消，下面那一排是批量动作，按 Esc 退出。",
    "app.move.aria": "移动到哪个分组",
    "app.move.title": "把这 %d 条移动到",
    "app.move.cancel": "取消",
    "app.tags.aria": "打标签",
    "app.tags.title": "给这 %d 条打标签",
    "app.tags.label": "标签",
    "app.tags.ph": "用空格或逗号分开写",
    "app.tags.empty": "标签是空的，先写一个。",
    "app.keys.aria": "快捷键",
    "app.keys.title": "快捷键",
    "app.keys.k": "打开命令面板",
    "app.keys.enter": "在面板里把选中的那条开在新标签页",
    "app.keys.slash": "聚焦搜索框",
    "app.keys.n": "打开添加浮层",
    "app.keys.q": "打开快捷键清单",
    "app.keys.long": "进多选：一次移动或打标签好几条",
    "app.keys.arrows": "在面板里选择",
    "app.keys.esc": "关掉当前浮起来的那一层",
    "app.keys.escAll": "Esc 关掉这一页",
    "app.sw.dismiss": "知道了",
    "app.sw.aria": "关掉这条提醒",
    "app.sw.updated": "书签页面已更新。点刷新用上新的。",
    "app.generic.cancel": "取消",
    "app.generic.save": "保存",
    "app.ops.item": "这一条",
    "app.ops.move": "移动",
    "app.ops.layout": "这一组的排法",
  "app.ops.group": "这一下",
  "app.ops.tabs": "点开书签",
  "app.ops.prefs": "这一格设置",
  "app.ops.bg": "背景图",
    "app.ops.bulkMove": "批量移动",
    "app.ops.bulkTags": "批量打标签",
    "app.ops.delete": "删除",
    "app.ops.bulkDelete": "批量删除",
    "app.edit.where": "这一条在「%s」里",
    "app.delete.item": "删掉这条",
    "app.delete.confirm": "确定删掉「%s」？再按一次就删",
    "app.delete.bulk": "删掉这 %d 条？",
    "app.delete.confirmMany": "删掉这 %d 条",
    "app.delete.more": "…另外还有 %d 条没列出来",
    "app.delete.aria": "批量删除",
    "app.delete.undoHint": "删掉之后还能用顶栏的「回退」找回这一版",
    "app.applyOps.fail": "%s没存上：%s",
    "app.opsConflict": "这一份已经被改过，刷新之后再试。",
    "app.opsWrongProfile": "这一页改的不是你正在用的那一份。点右上角的档案名，重新进一次。",
    "app.opsStale": "那一组或那一条已经不在了——可能别人刚改过。刷新之后再看一次。",
    "app.opsDuplicate": "这一条已经在这份里了（同一条书签不能存两次）。要改它，就在那一条上按 ✎。",
    "app.opsIcon": "那个图标名这台机器查不到。换一枚，或者留空走首字母色块。",
    "app.resolveBadUrl": "这个地址还不完整——要有协议和主机名（例如 http://10.0.0.5:5000/）。",
    "app.resolveDown": "那个网页没应答（那台机器可能没开）。照样存下来也行，回头补标题。",
    "app.opsNoProfile": "这台机器上没有这一份档案了——多半是刚被删掉。点右上角的档案名重新选一份。",
    "app.opsBroken": "这一份的数据文件读不动，刚才的改动没有写进去。这不是重试一次能解决的——要在这台机器上修好那个文件。",
    "app.opsRejected": "这一批没有写进去：这台机器不收刚才那些内容。（原因在浏览器控制台里。）",
    "app.opsNoSnap": "这一版存不下，改动已经取消，书签一个字都没变。",
    "app.opsServerError": "这台机器刚才没能完成（HTTP %s）。稍后再试一次。",
    "app.opsGone": "这一页的登录已经过期。点右上角的档案名，重新进一次就好。",
    "app.sep": "、",
    "app.keys.mod": "⌘ 或 Ctrl",
    "app.keys.longpress": "长按卡片"
  },
  "en": {
    "app.add": "+ Add",
    "app.addTitle": "Add a bookmark (press n)",
    "app.sheetTitle": "Add bookmark",
    "app.field.url": "Address",
    "app.field.title": "Name",
    "app.field.desc": "Description",
    "app.field.group": "Group",
    "app.field.icon": "Icon",
    "app.ph.title": "Page title, editable",
    "app.ph.desc": "One-line note, optional",
    "app.ph.group": "Name for the new group",
    "app.icon.none": "No icon (use the first letter)",
    "app.icon.auto": "Fetch this site's icon",
    "app.icon.currentSuffix": " (current)",
    "app.icon.fetch": "Look up this site's icons",
    "app.icon.lib": "Icon library…",
    "app.icon.libTitle": "Pick one from the icon library",
    "app.icon.libPh": "Search by name — docker, github, or tabler:box",
    "app.icon.libHint": "The library ships with the program: brand marks (simple-icons) and concept icons (tabler).",
    "app.icon.libStart": "Search for a name above and the icons show up here.",
    "app.icon.libNone": "Nothing in the library is called %s",
    "app.icon.libMore": "%s in all, showing the first %s",
    "app.icon.libFail": "The icon library could not be reached (try again in a moment)",
    "app.save": "Save",
    "app.saveMore": "Save and continue",
    "app.close": "Close",
    "app.reading": "Reading that page…",
    "app.readFail": "Nothing was read; you can fill the title in yourself",
    "app.readOk": "Read the title, description and icon",
    "app.readEmpty": "That page gave nothing readable, so fill the title in yourself",
    "app.readErr": "Could not read it: %s",
    "app.err.group": "Pick a group first.",
    "app.err.groupName": "Give the new group a name first.",
    "app.err.url": "Fill in an address first.",
    "app.offline.queued": "You are offline. This one is queued and will go through when the connection is back.",
    "app.offline.chip": "%d queued offline",
    "app.offline.aria": "Show bookmarks queued while offline",
    "app.offline.drop": "Drop this one",
    "app.offline.list": "These were saved while offline and will go through once you are back online.",
    "app.offline.blocked": "No network, so this is read-only for now. Additions are queued and sent automatically when you are back.",
    "app.offline.done": "%d queued bookmarks have been sent",
    "app.offline.dropped": "%d bookmarks clashed with someone else's changes and were not merged automatically. Please add them again: %s",
    "app.offline.left": "%d bookmarks still not saved: %s",
    "app.saving": "Saving…",
    "app.saved": "Saved.",
    "app.conflict": "This profile changed underneath you (something else may have edited it). Refresh and try again.",
    "app.cmdk.placeholder": "Search bookmarks, or do something",
    "app.cmdk.more": "Search the whole page for “%s” · %d matches in total",
    "app.cmdk.aria": "Command palette",
    "app.cmdk.clipboard": "Save the address on the clipboard",
    "app.cmdk.history": "See this profile’s change history",
    "app.cmdk.kind": "action",
    "app.cmdk.none": "Nothing matches “%s” - no bookmark, and no action either.",
    "app.cmdk.empty": "There is nothing to list yet.",
    "app.clip.denied": "This browser will not let the page read the clipboard. Paste the address yourself.",
    "app.clip.noturl": "What is on the clipboard does not look like an address. Paste it yourself.",
    "app.clip.noread": "Could not read the clipboard (the system did not grant permission). Paste the address yourself.",
    "app.copy.aria": "Copy the link for %s",
    "app.copy.done": "Copied",
    "app.copy.fail": "Could not copy it — the address is selected now, press ⌘ or Ctrl + C",
    "app.edit.aria": "Edit “%s”",
    "app.edit.fallbackName": "this bookmark",
    "app.layout.tiles": "Tiles",
  "app.group.new": "+ New group…",
  "app.group.shape": "Arrange it as",
    "app.layout.rows": "Single column",
    "app.layout.feature": "Feature",
    "app.layout.featureTitle": "moves to a band of its own at the top: bigger cards with the host under the name. This group no longer appears in the body sections",
    "app.layout.aria": "Switch the group “%s” from %s to %s",
    "app.multi.aria": "Bulk actions",
    "app.multi.move": "Move to (%d)",
    "app.multi.tags": "Tag (%d)",
    "app.multi.delete": "Delete (%d)",
    "app.multi.none": "Clear selection",
    "app.multi.exit": "Exit",
    "app.multi.hint": "In multi-select: click cards to pick or unpick them. The row below holds the bulk actions; press Esc to exit.",
    "app.move.aria": "Which group to move to",
    "app.move.title": "Move these %d bookmarks to",
    "app.move.cancel": "Cancel",
    "app.tags.aria": "Add tags",
    "app.tags.title": "Tag these %d bookmarks",
    "app.tags.label": "Tags",
    "app.tags.ph": "Separate them with spaces or commas",
    "app.tags.empty": "The tags are empty. Write at least one.",
    "app.keys.aria": "Keyboard shortcuts",
    "app.keys.title": "Keyboard shortcuts",
    "app.keys.k": "Open the command palette",
    "app.keys.enter": "In the palette, open the selected bookmark in a new tab",
    "app.keys.slash": "Focus the search box",
    "app.keys.n": "Open the add sheet",
    "app.keys.q": "Open the shortcut list",
    "app.keys.long": "Enter multi-select: move or tag several at once",
    "app.keys.arrows": "Move the selection in the palette",
    "app.keys.esc": "Close the topmost open layer",
    "app.keys.escAll": "Esc closes this panel",
    "app.sw.dismiss": "Got it",
    "app.sw.aria": "Dismiss this notice",
    "app.sw.updated": "The bookmark page has been updated. Refresh to use it.",
    "app.generic.cancel": "Cancel",
    "app.generic.save": "Save",
    "app.ops.item": "That bookmark",
    "app.ops.move": "The move",
    "app.ops.layout": "The group's layout switch",
  "app.ops.group": "That change",
  "app.ops.tabs": "opening a bookmark",
  "app.ops.prefs": "that setting",
  "app.ops.bg": "the background",
    "app.ops.bulkMove": "The bulk move",
    "app.ops.bulkTags": "The bulk tagging",
    "app.ops.delete": "The deletion",
    "app.ops.bulkDelete": "The bulk deletion",
    "app.edit.where": "This bookmark is in “%s”",
    "app.delete.item": "Delete this bookmark",
    "app.delete.confirm": "Delete “%s”? Press again to delete it",
    "app.delete.bulk": "Delete these %d bookmarks?",
    "app.delete.confirmMany": "Delete %d bookmarks",
    "app.delete.more": "…and %d more not listed",
    "app.delete.aria": "Bulk delete",
    "app.delete.undoHint": "You can bring this version back from “Revert” in the top bar",
    "app.applyOps.fail": "%s didn't go through: %s",
    "app.opsConflict": "This profile changed underneath you. Refresh and try again.",
    "app.opsWrongProfile": "This page isn't the profile you're using. Tap the name at the top right and come back in.",
    "app.opsStale": "That group or bookmark is gone — someone else may have changed it. Reload and look again.",
    "app.opsDuplicate": "That one is already in this profile — a bookmark can't be stored twice. To change it, press ✎ on it.",
    "app.opsIcon": "This machine doesn't know that icon name. Pick another, or leave it empty and use the initial-letter tile.",
    "app.resolveBadUrl": "That address isn't complete yet — it needs a scheme and a host name (e.g. http://10.0.0.5:5000/).",
    "app.resolveDown": "That page didn't answer (the machine may be off). You can still save it and fill the title in later.",
    "app.opsNoProfile": "This machine no longer has that profile — it was probably just deleted. Pick another one from the name at the top right.",
    "app.opsBroken": "That profile's data file can't be read, so the change wasn't written. Retrying won't help — the file has to be fixed on this machine.",
    "app.opsRejected": "That batch wasn't written — this machine didn't accept what was in it. (The reason is in the browser console.)",
    "app.opsNoSnap": "This version couldn't be stored, so the change was cancelled — your bookmarks are untouched.",
    "app.opsServerError": "This machine couldn't finish that (HTTP %s). Try again in a moment.",
    "app.opsGone": "This page is no longer signed in. Click your profile name at the top right to sign in again.",
    "app.sep": ", ",
    "app.keys.mod": "⌘ or Ctrl",
    "app.keys.longpress": "Long-press a card"
  }
};
// END APP_I18N


(() => {
  'use strict';

  // ── 文案查表 ────────────────────────────────────────────────────────────────
  // 语言从 <html lang> 读（服务端渲染时已经写对了）：zh-CN → zh，en → en。
  // 读不到就落中文——**与 server 侧 i18n.Default 同一个默认**，两处必须一致。
  const LANG = (document.documentElement.lang || 'zh-CN').slice(0, 2);
  const MSGS = APP_I18N[LANG] || APP_I18N.zh;

  // T 是查表，可带 %s/%d 占位符。
  // **缺 key 不返回空串**：返回 `⟨key⟩`。页面上出现尖括号一眼看出漏翻了，
  // 而空白看着像设计如此——那种 bug 会被当成"这页本来就空"。
  function T(key, ...args) {
    const s = MSGS[key];
    if (s === undefined) return '⟨' + key + '⟩';
    if (!args.length) return s;
    let i = 0;
    return s.replace(/%[sdv]/g, () => (i < args.length ? String(args[i++]) : '⟨缺参数⟩'));
  }

  const body = document.body;
  const profile = body.dataset.profile || '';
  let rev = Number(body.dataset.rev || '0');
  if (!profile) return; // 不是档案首屏（选择页 / 坏档页），这里什么也不注入


  // 服务端每个 JSON 错误都带一格 kind（AGENTS.md「错误码」）。这两张表是**这一侧
  // 唯一的选句子判据**：kind 优先，状态码兜底。
  //
  // 为什么不能只按状态码：同一个 422 在两条通路上说的是两件相反的事——
  // missing-target 该说"刷新看看"，duplicate-id 该说"这条已经在里面了，去改它"。
  // 上一版只按状态码选，于是重复添加同一网址的人看到"东西没了，刷新一下"。
  //
  // 422 的兜底是 app.opsRejected（"没收，原因在 console"）而不是"已经不在了"：
  // 走到那一格说明这台机器给了一个这一版还不认识的码，那句"刷新看看"就成了假话。
  const KIND_KEY = {
    'duplicate-id': 'app.opsDuplicate',
    'missing-target': 'app.opsStale',
    'icon-unknown': 'app.opsIcon',
    'url-incomplete': 'app.resolveBadUrl',
    'target-unreachable': 'app.resolveDown',
    'rev-conflict': 'app.opsConflict',
    'credential-missing': 'app.opsGone',
    'credential-unknown': 'app.opsGone',
    'credential-scope': 'app.opsWrongProfile',
    'profile-not-found': 'app.opsNoProfile',
    'profile-broken': 'app.opsBroken',
    'snapshot-failed': 'app.opsNoSnap',
  };
  const STATUS_KEY = { 401: 'app.opsGone', 403: 'app.opsWrongProfile', 409: 'app.opsConflict', 422: 'app.opsRejected', 507: 'app.opsNoSnap' };

  // isURLProblem 判这一次失败说的是不是**地址本身不行**。
  //
  // 判据只收 `url-incomplete` 与 `field-invalid` 两个 kind，**不按 HTTP 码判**：
  // resolve 的地址类失败是 422，而 422 同时也是"这一批没写进去"的码（batch）。
  // 按码判会把一次保存失败也贴到地址那一格去——那一格没有问题，是别的地方有问题。
  // `target-unreachable`（抓不下来）也**不算**：地址是对的，是那台机器没应答，
  // 它说的是"这一次读取"而不是"你敲错了"。
  const URL_KINDS = new Set(['url-incomplete', 'field-invalid']);
  const isURLProblem = (r) => URL_KINDS.has(r && (r.kind || ''));

  // kindOf：这一发该按哪一码说话。batch 的外壳 kind 说的是**这一族**
  // （ops-invalid = 整批未写入），具体哪一条、为什么在 errors[0] —— 所以先读明细。
  function kindOf(data) {
    const first = Array.isArray(data && data.errors) && data.errors[0];
    return (first && first.kind) || (data && data.kind) || '';
  }
  // msgKeyFor 挑这一发该说的那一句：kind 优先，状态码只在这一版不认识那个码时兜底。
  function msgKeyFor(data, status) {
    return KIND_KEY[kindOf(data)] || STATUS_KEY[status] || 'app.opsServerError';
  }
  // ── 小工具 ────────────────────────────────────────────────────────────────
  const $ = (sel, root = document) => root.querySelector(sel);
  const el = (tag, attrs = {}, kids = []) => {
    const n = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs)) {
      if (k === 'text') n.textContent = v; // **永远走 textContent**：书签名、
      // desc、标题都是用户或第三方给的，innerHTML 会把它们当标记解析。
      else if (k === 'on') for (const [ev, fn] of Object.entries(v)) n.addEventListener(ev, fn);
      // **空串也要写上去**，只跳 null/undefined（2026-10-13 实拍逮到）。
      // 从前是 `v !== ''` ——理由大概是"不写空 class/id，省几个字节"。但对
      // `<option>` 来说少写 `value` 不是省字节而是**换语义**：没有 value 属性的
      // option，其 `.value` 按规范回退到**文字**，于是「不选图标（用名字第一个字）」
      // 那一颗读回来的是这句中文，submit 把它当图标名发出去，服务端回一句
      // 「图标名不在索引里」。
      // 症状特别难认：它是**每一条从浮层存进去的书签都发不出去**，而页面上那一格
      // 看上去完全正常——只有一个真实用户点一次才看得见。
      else if (v != null) n.setAttribute(k, v);
    }
    for (const c of [].concat(kids)) n.append(c);
    return n;
  };
  // say 把一句话写到**某一行状态里**。第三参数是落点：不给就是底部那条
  // （`status`，答"这次读取/保存整体怎么样了"），给了就写到那一格自己下面
  // （答"哪一格不对"）。两种答案同时在场不冲突——它们说的不是一件事。
  //
  // **为什么按参数分流而不是分两个函数**：一个叫 say 一个叫 sayField 的话，
  // 第三个调用点开始就有人挑错，而挑错的后果是"字段错误飘到表单最底端"——
  // 那正是 2026-10-14 实拍到的那个位置（地址是第一格，红字在「保存」下面）。
  const say = (msg, kind = 'info', into = null) => {
    const node = into || status;
    if (!msg) {
      // 清空**两条**：只清落点那一条会让上一句挂在别的行上不走——那句话
      // 描述的是上一轮的字段状态，留着就是对着已经改对的地址继续报错。
      status.textContent = '';
      status.className = 'sheet-status';
      if (into) { node.textContent = ''; node.className = 'sheet-status fieldmsg'; }
      return;
    }
    node.textContent = msg;
    node.className = 'sheet-status fieldmsg ' + kind;
    if (into && into !== node) { status.textContent = ''; status.className = 'sheet-status'; }
  };

  // ── 顶栏按钮 ──────────────────────────────────────────────────────────────
  const header = $('header');
  if (!header) return;

  // 打开浮层这件事有四个入口（按按钮、按 n、bookmarklet 带进来、⌘K 的剪贴板），
  // 光标该落在哪儿只有一个答案。从前只有 `n` 那条会 focus()：鼠标用户点开之后
  // 还要再点一下输入框才能贴地址。
  // aria-modal="true" 是一句**声明**，浏览器不会替你做到。改前实测：浮层开着时
  // 面板外还有 29 个可聚焦元素（面板内只有 9 个），Tab 一路走到背后那张被遮住的
  // 页面上去——焦点跑到看不见的地方，比没有浮层更难找回。
  // inert 把"面板之外"整个摘出 Tab 顺序与读屏；老浏览器不认它也只是退回原样，
  // 功能不受影响（渐进增强）。
  function syncTrap() {
    // 查 DOM 而不是引用闭包里的 help/cmdk：bookmarklet 那条路径会在它们声明之前就
    // 调用 openSheet，而 module 里提前读一个还没初始化的 const 是 TDZ 报错。
    // DOM 里的顺序天然就是叠放顺序：最后面出现的那一层是最上层。
    // 判据本身在 topLayer() 里——「哪些算一层」只有一份，syncTrap 与「点外面关」
    // 共用它（两份会漂，而漂的症状各自都不响）。
    const top = topLayer();
    for (const el of document.body.children) {
      if (!top || el === top) el.removeAttribute('inert');
      else el.setAttribute('inert', '');
    }
  }

  // closeTopLayer 关掉**最上面**那一层，返回它是不是关掉了什么。
  //
  // **它为什么必须是"最上面"而不是"全部"**：在图标库那一层里再点开添加面板，
  // 一次 Esc 应当只关图标库；连浮层和面板一起关掉是一次按键毁掉两个状态。
  //
  // Esc 与"点外面"共用这一份（2026-10-14）：从前 Esc 那一支是内联在 keydown 里的，
  // 而"点外面"根本不存在——只有 Esc 与 × 能关，于是唤出 ⌘K 之后点了页面别处，
  // 面板纹丝不动，而背后整页还被 inert 摘掉了可聚焦性（点不动、Tab 也出不去）。
  // 两处各写一份顺序的判据，早晚会漂，所以只留这一份。
  function closeTopLayer() {
    // 多选排第一：进多选时那句提示明写「按 Esc 退出」，而从前这一支里
    // 根本没有 exitMulti——说了没做。编辑面板比它更靠上层，先关它。
    // 判据读 openEdit 而不是查 DOM：这一层如今挂在 body 上，
    // `editing.closest('li')` 那套"从面板找回它那一行"的写法已经不成立。
    if (openEdit) { closeEdit(); return true; }
    // 多选**不在这条"关掉"的路上**：它是页面的一部分（顶栏一条多选带），
    // 点外面去不叫"关掉弹层"，而 Esc 要管它——所以这里 return false，
    // 让"点外面"那半句继续往下的层找，而不是把整条多选带撤掉。
    if (multi.on) { return false; }
    // 台面浮层要排在下面那个通用 `.sheet` 分支**之前**：它就是一个 `.sheet`，
    // 而那一支是 `dlg.remove()`——那是为"用完即弃"的挑组/标签/确认框写的
    // （它们每次重新画）。台面面板的标记是**fetch 一次、反复复用**的，
    // remove 掉等于第二次打开要重新拉一次，而且 DOM 里那个空壳被摘走了。
    if (prefsSheet && !prefsSheet.hidden) { prefsClose(); return true; }
    // 图标库那一层要排在下面那个通用 `.sheet` 分支**之前**：它就是 `.sheet`，
    // 而那一支是 `dlg.remove()`——摘掉 DOM 却不同步 syncTrap，页面会整片留在
    // inert 里（面板没了，一个字都点不动）。它的收尾还要把焦点还给打开它的那颗键。
    if (iconPick.panel) { closeIconPicker(); return true; }
    // 那些"用完即弃"的小层（挑组 / 挑标签 / 确认）：DOM 里现找，摘掉。
    const dlg = [...document.querySelectorAll('.sheet')].find((x) => x !== sheet && x !== prefsSheet);
    if (dlg) { dlg.remove(); syncTrap(); return true; }
    for (const n of [help, cmdk, sheet]) {
      if (!n.hidden) { n.hidden = true; syncTrap(); return true; }
    }
    const banner = $('#lb-banner');
    if (banner) { banner.remove(); return true; }
    return false;
  }

  // 点层外 = 关掉最上面那一层（2026-10-14）。
  //
  // **为什么用 pointerdown 而不是 click**：click 要等 mouseup 落在同一个目标上，
  // 而人在"点外面"时经常是按下、拖一点、松开——那一下 click 压根不发生，
  // 于是"甩一下关掉"这种最自然的动作反而关不掉。pointerdown 立即响应。
  //
  // **为什么判据是 `contains` 而不是坐标**：这一层盖着整屏，层外就是它自己那张
  // 遮罩；用坐标要去量 rect，而层内元素被重排时 rect 会变——`contains` 问的是
  // "这一下点在谁身上"，DOM 结构说了算，不受布局影响。
  //
  // **为什么不用捕获阶段**：冒泡阶段就够了。inert 让层外的元素收不到事件，
  // 也就是说这一下的 target **永远落在最上面那一层之内或之上**——判据成立。
  document.addEventListener('pointerdown', (e) => {
    if (!topLayer()) return;
    const top = topLayer();
    if (top.contains(e.target)) return;
    closeTopLayer();
  });

  // topLayer 是"现在最上面那一层"，与 syncTrap 用的是**同一份 DOM 判据**。
  // 两个函数各自写一遍"哪些算层"是这一族最容易漂的地方：syncTrap 少认一层，
  // 症状是那一层开着时背后整页仍可聚焦；这里少认一层，症状是点了外面关不掉它。
  function topLayer() {
    return [...document.querySelectorAll('#lb-sheet, .cmdk, .sheet')].filter((n) => !n.hidden).pop() || null;
  }

  function openSheet() {
    sheet.hidden = false;
    syncTrap();
    say('');
    fUrl.focus();
  }

  const openBtn = el('button', {
    type: 'button', id: 'lb-add', class: 'add primary', text: T('app.add'), title: T('app.addTitle'),
    on: { click: openSheet },
  });
  // 插在搜索表单**之后**而不是 header 末尾：DOM 顺序就是视觉顺序，这样「＋ 添加」
  // 跟着标题与搜索框排同一行，不再独占一行拉成一条横带（1440 上实测 980×44）。
  const searchForm = $('header .search');
  header.insertBefore(openBtn, searchForm ? searchForm.nextSibling : header.firstChild);

  // ── 浮层 ──────────────────────────────────────────────────────────────────
  const fUrl = el('input', { type: 'url', id: 'lb-url', class: 'fld', placeholder: 'https://', required: 'required', autocomplete: 'off' });
  const fTitle = el('input', { type: 'text', id: 'lb-title', class: 'fld', placeholder: T('app.ph.title'), autocomplete: 'off' });
  const fDesc = el('input', { type: 'text', id: 'lb-desc', class: 'fld', placeholder: T('app.ph.desc'), autocomplete: 'off' });
  const fGroup = el('select', { id: 'lb-group', class: 'fld' });
  const fNewGroup = el('input', { type: 'text', id: 'lb-newgroup', class: 'fld', placeholder: T('app.ph.group'), autocomplete: 'off' });
  // 「这一组用磁贴还是单列」——**只在建新分组时出现**。往已有分组里加一条时它
  // 没有作用，而"点了没反应的东西不进界面"（§8.1）。
  //
  // 它之所以在**这一格**而不是台面面板里，是时序：面板改的是已经建好的分组，
  // 而这一次是在同一次提交里建组 + 放条目——顺路问一句，比让人存完再去面板翻一遍
  // 便宜得多。新建出来的分组照样能在面板里随时改。
  const fGroupShape = el('div', { class: 'sheet-keys' }, [
    el('button', { type: 'button', class: 'fld', 'aria-pressed': 'true', text: T('app.layout.tiles'), on: { click: () => pickShape('tiles') } }),
    el('button', { type: 'button', class: 'fld', 'aria-pressed': 'false', text: T('app.layout.rows'), on: { click: () => pickShape('list') } }),
  ]);
  const fGroupShapeWrap = el('label', {}, [
    el('span', { text: T('app.group.shape') }),
    fGroupShape,
  ]);
  // NEW_GROUP 是那颗哨兵选项的值。**它不能是空串**：空串在上一个版本里表示
  // 「（不分组）」，而那一条是走不通的路——submit 判到空值就报错，于是界面摆着
  // 一条按下去必然失败的选项。
  const NEW_GROUP = '__new__';
  let shape = 'tiles';
  function pickShape(next) {
    shape = next;
    for (const b of fGroupShape.children) {
      b.setAttribute('aria-pressed', String(b === fGroupShape.children[next === 'tiles' ? 0 : 1]));
    }
  }
  // 建新分组的那几格跟着下拉的当前值显隐：选了「＋ 新分组…」才问名字，
  // 没选就不占那一屏——一个永远在、永远不动的输入框是界面上最吵的一种东西。
  function syncGroupFields() {
    const creating = fGroup.value === NEW_GROUP;
    fNewGroup.hidden = !creating;
    fGroupShapeWrap.hidden = !creating;
  }
  fGroup.addEventListener('change', syncGroupFields);
  const fIcon = el('select', { id: 'lb-icon', class: 'fld' });

  const status = el('p', { class: 'sheet-status', role: 'status', 'aria-live': 'polite' });
  // 「地址」自己的那一行错误（2026-10-14）。**为什么它不能只靠底部那一条**：
  // 实拍——地址是表单第一格，而消息挂在最后（在「保存」下面），中间隔着名字、
  // 描述、分组、排法、图标五格；而且 resolve 是 blur 触发的，那时焦点已经跳到
  // 「名字」，所以人正看着的那一格下面什么都没有，红字在最远的一端。
  //
  // 判据是**归位**而不是搬走：错误属于哪一格就贴在那一格下面。底部那条留给
  // 「已读到标题」这类**整张表单**的进度与结果（它答的是"这次读取怎么样了"，
  // 不是"哪一格不对"）。两句可能同时在场，而它们说的不是一件事。
  const urlErr = el('p', { class: 'sheet-status bad fieldmsg', role: 'alert' });
  // 一敲就撤：地址改了之后上一句就过期了，而留着它等于让人对着已经改对的地址
  // 继续被告知它不对。focus 也算——回到这一格就是要改它。
  const clearUrlErr = () => { urlErr.textContent = ''; };
  fUrl.addEventListener('input', clearUrlErr);
  fUrl.addEventListener('focus', clearUrlErr);
  const saveBtn = el('button', { type: 'submit', class: 'fld', text: T('app.save') });
  const moreBtn = el('button', { type: 'button', class: 'fld more', text: T('app.saveMore'), on: { click: () => submit(true) } });

  const form = el('form', {
    on: { submit: (e) => { e.preventDefault(); submit(false); } },
  }, [
    el('div', { class: 'fld-group' }, [el('label', { text: T('app.field.url') }, [fUrl]), urlErr]),
    // 字段名与就地编辑统一成「名称」（数据里就是 name）。这里从前叫「标题」、
    // 就地编辑叫「名字」——同一个东西在两层里两个名字。
    el('label', { text: T('app.field.title') }, [fTitle]),
    el('label', { text: T('app.field.desc') }, [fDesc]),
    // **这一格从前没有标签**（2026-10-14 实拍修掉）：`el('label', {}, [...])` 里
    // 一个字都没有，于是下拉框在屏幕上是一个没有名字的控件，读屏里是
    // 「组合框，应用」——听不出它管的是哪一维。旁边四格（地址/名字/描述/图标）
    // 全都有标签，缺的就是这一格，看着像忘了写而不是有意的。
    // `app.field.group` 这个 key **早就躺在四份词典里**（zh/en × 服务端/前端），
    // 而全仓再没有第二处引用它——所以这不是新增文案，是把一条早就写好、
    // 一直没人接的判据接回去。（写这一行注释时先 grep 过：它此前只被声明。）
    el('label', { text: T('app.field.group') }, [fGroup, fNewGroup, fGroupShapeWrap]),
    el('div', { class: 'icrow' }, [
      el('label', { text: T('app.field.icon') }, [fIcon]),
      iconLibraryButton(fIcon),
    ]),
    el('div', { class: 'sheet-actions' }, [saveBtn, moreBtn]),
    status,
  ]);

  const sheet = el('div', {
    id: 'lb-sheet', class: 'sheet', hidden: 'hidden', role: 'dialog',
    'aria-modal': 'true', 'aria-labelledby': 'lb-sheet-h',
  }, [
    el('h2', { id: 'lb-sheet-h', text: T('app.sheetTitle') }),
    form,
    el('button', {
      type: 'button', class: 'fld close', text: '×', 'aria-label': T('app.close'),
      // 关掉之后焦点回到打开它的那枚按钮：浮层是从那儿浮起来的，键盘用户
      // 按 Esc 之后该站在原地，而不是掉回页面顶部。
      on: { click: () => { sheet.hidden = true; syncTrap(); openBtn.focus(); } },
    }),
  ]);
  body.append(sheet);

  // ── 台面面板 ──────────────────────────────────────────────────────────────
  // 顶栏那颗「台面」是 `<a href="/prefs">`。JS 关着时它就是一次普通跳转——**降级
  // 路径必须自己成立**，所以它得是一个真的链接而不是一颗按钮。JS 开着时拦下来
  // 弹成浮层。
  //
  // 内容是**服务端现渲染**的，不是这里拼的：面板里每一行都是一张普通的
  // `form POST /prefs`，抽屉外壳是 JS 画的，改设置不是。fetch 回来的是同一份
  // define（`GET /prefs` 那张独立页渲染的也是它），于是浮层与整页永远一致。
  //
  // 为什么 fetch 一次而不是把标记内嵌进档案页：面板约 2 KB，而 §7.3 那条操作线
  // 管的是**每一页**的首屏——不打开面板的绝大多数人（只是来看书签）不该背这 2 KB。
  // 代价是打开时多一次同源请求；它是本地请求，面板自己不写状态，**失败就退回一次
  // 普通跳转**（一次网络抖动不该把人堵在"浮层开不出来"上）。
  const prefsSheet = $('#lb-prefs');
  let prefsLoaded = false;

  // 背景那一格的文件名回显（2026-10-14）。原生 `<input type=file>` 被 CSS 收起来
  // 之后（见 base.css .prefs-bg-file：它是浏览器自己画的按钮，color-scheme 管不到，
  // 暗色页上一块浅灰像别的产品），"选中了哪个文件"就没有别的地方能显示了。
  //
  // **只在这一条路上做**：档案页的面板是 JS 注入的，所以能绑；`GET /prefs` 那张
  // 零 JS 的独立页没人绑，于是它看不见文件名（表单照样能提交，提交后台面重画）。
  // 绑定用 `on: {change: …}` 与这一份脚本里其余事件同一处置，且**只绑一次**——
  // 面板是注入一次就长期在 DOM 里（prefsLoaded 那一闩），重复绑会挂出多个监听。
  function wireBgFileName(root) {
    const input = root.querySelector('.prefs-bg-file input[type=file]');
    const out = root.querySelector('.prefs-bg-picked');
    if (!input || !out || input.dataset.lbWired) return;
    input.dataset.lbWired = '1';
    input.addEventListener('change', () => {
      // **只取最后一段**：完整路径在这台机器上可能是 C:\Users\…\某张图.png，
      // 一行放不下而被省略号吃掉，露出来的恰好是用户最不需要看的那部分。
      out.textContent = input.files && input.files[0] ? input.files[0].name : '';
    });
  }

  async function prefsOpen() {
    if (!prefsSheet) return;
    if (!prefsLoaded) {
      try {
        const res = await fetch('/prefs', { headers: { Accept: 'text/html' } });
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const doc = new DOMParser().parseFromString(await res.text(), 'text/html');
        const panel = doc.querySelector('section.prefs');
        if (!panel) throw new Error('no panel in that page');
        prefsSheet.append(panel, el('button', {
          type: 'button', class: 'fld close', text: '×', 'aria-label': T('app.close'),
          on: { click: prefsClose },
        }));
        wireBgFileName(prefsSheet);
        prefsLoaded = true;
      } catch {
        location.href = '/prefs';
        return;
      }
    }
    prefsSheet.hidden = false;
    syncTrap();
    // 落点跟「＋ 添加」不一样：那里是**输入**面板，焦点必须在第一个输入框上；
    // 这里是**设置**面板，第一格是文本框但大部分人进来是换视图或换主题，
    // 焦点给它只会让读屏从一行字开始念。落在面板自己身上（tabindex=-1 加在
    // 空壳上），读屏先念可及名——也就是那个 h2。
    prefsSheet.focus();
  }

  function prefsClose() {
    if (!prefsSheet || prefsSheet.hidden) return;
    prefsSheet.hidden = true;
    syncTrap();
    $('#lb-prefs-open')?.focus();
  }

  $('#lb-prefs-open')?.addEventListener('click', (e) => {
    e.preventDefault();
    prefsOpen();
  });

  // 面板里**改书签数据**的表单（POST /prefs/group）在这里被拦下，改走
  // /api/v1/batch——**落点与那张表单完全相同**（都是 applyBatch：快照、审计、
  // rev 一道闸不少），只是不必为按一颗色块把整页导航一次。
  //
  // 拦不拦得动由**按钮的形状**判，不由"面板是不是浮层"判：零 JS 时同一张表单
  // 照常提交、303 回 /prefs 整页，这条路不需要存在。于是独立页上的分组表单
  // 不会被这份委托影响——那一页根本不引 app.js。
  if (prefsSheet) {
    prefsSheet.addEventListener('submit', async (e) => {
      const form = e.target.closest('form[action="/prefs/group"]');
      if (!form) return;
      e.preventDefault();
      // 一次只发一条 op：表单里颜色与排法两组键并排，靠"只提交被按下的那个
      // 控件"分开，所以这里读 submitter 就能知道人按的是哪一颗——
      // 按了没有 name 的那颗「用这个颜色」时 submitter 是那颗键，取它旁边的
      // 取色器。
      const btn = e.submitter;
      const data = new FormData(form, btn || undefined);
      const op = groupOpFromForm(form, data);
      if (!op) return;
      const ok = await applyOps([op], T('app.ops.group'));
      if (ok) reloadKeepingPlace();
    });
    // 「点开书签」那一格（POST /prefs/tabs，2026-10-14 补上）。
    //
    // **它从前是原生提交的**，而服务端答 303 回 `/prefs`——于是在浮层里按一下
    // 「新标签页」，整页导航到独立页（实拍复现）。症状读作「设置变成单独页面了」。
    // 而 `/prefs/group` 那条早就在走 applyOps，**落点与这张表单完全相同**
    // （applyPrefsOps → applyBatch，快照与审计一道不少），所以这条只是漏了同一份
    // 委托，不是新通路。
    prefsSheet.addEventListener('submit', async (e) => {
      const form = e.target.closest('form[action="/prefs/tabs"]');
      if (!form) return;
      e.preventDefault();
      const v = e.submitter?.value;
      if (v !== '0' && v !== '1') return; // 只认这两个（服务端同一条纪律）
      await applyOps([{ op: 'set', path: 'openInNewTab', value: v === '1' }], T('app.ops.tabs'));
      // **不整页刷新**：这一格只改一个布尔，而整页重载会把面板整个重建
      // （prefsLoaded 那一闩只在首次 fetch 后才为真，重载后要重新拉一次），
      // 于是「按一下 → 面板消失」与「整页跳走」是同一种丢失。
      // 改完就地按一下那颗键的 aria-pressed：那是这一格唯一的可见状态。
      markPressed(form, v);
    });
    // 改名 / 怎么看 / 分组导航 / 主题那四张（POST /prefs，2026-10-14 补上）。
    //
    // **它们不能走 applyOps**：那四格改的是 `profile.json`（台面偏好），不产生
    // rev、不做快照——而 applyOps 走的是 `/api/v1/batch`（书签文档），那是**另一份
    // 文件**。所以这一族是普通 fetch + 就地改，不假装它是 batch。
    //
    // 为什么要就地改而不是整页重载：这一格按完，屏上要变的**只有那颗键的
    // aria-pressed**（标签是稳的，见 TestPrefs_开关的标签稳定而状态走aria）。
    // 整页重载会把面板整个重建（`prefsLoaded` 那一闩只在首次 fetch 后为真），
    // 于是「按一下 → 面板消失」与「整页跳走」是同一种丢失。
    prefsSheet.addEventListener('submit', async (e) => {
      const form = e.target.closest('form[action="/prefs"]');
      if (!form) return;
      e.preventDefault();
      const btn = e.submitter;
      const data = new FormData(form, btn || undefined);
      const payload = {};
      // **只交被按下的那一格**：命名过的输入框每次提交都跟着走，
      // 而一次提交就是"改一件事"。
      for (const k of ['theme', 'view', 'layout', 'title']) {
        if (btn && btn.name === k) payload[k] = String(data.get(k) ?? '');
      }
      if (!Object.keys(payload).length) return;
      const body = new URLSearchParams(payload);
      try {
        // 手动跟随 303：fetch 默认不跟，而这一族成功回的就是 303。
        // 我们不关心它跳去哪儿（面板留在原地），只关心它有没有成功——
        // fetch 对 3xx 仍然算 ok，所以真正的失败是网络异常与 4xx/5xx。
        const res = await fetch('/prefs', {
          method: 'POST', body, credentials: 'same-origin', redirect: 'follow',
        });
        if (!res.ok) throw new Error('HTTP ' + res.status);
        // **这一族必须重渲染**（2026-10-14 实拍后改判）。先前只给 theme 做了就地更新，
        // 于是 view/layout/title 按下去**屏上一动不动**——症状是"设置改了没生效"，
        // 而服务端明明答了 303、数据也写进去了。
        //
        // 判据是**这一格的效果画在哪里**，不是"能不能不重载"：
        //   - `data-theme` 是 `<html>` 上的一个属性，换掉它整页立刻变色 → **能就地**；
        //   - view / layout / title 决定的是**整页怎么画**（分几栏、侧栏在不在、
        //     标题叫什么），没有一个属性可以单独换 → **只能重渲染**。
        // 「留在浮层」在这里赢不了「立刻见效」：屏上没变化时，人不会去想"要不要刷新"，
        // 他会以为没保存成功，然后按第二遍——第二遍同样没变化。
        //
        // 所以：**theme 就地，其余重载**。重载后回到当前 URL（档案页或 /prefs），
        // 落在**新**的那一档视图上，这正是"按一下就生效"。
        if (btn && btn.name === 'theme' && payload.theme) {
          document.documentElement.dataset.theme =
            payload.theme === 'auto' ? '' : payload.theme;
          if (btn.value != null) markPressed(form, btn.value);
          return;
        }
        location.reload();
      } catch (err) {
        showBanner(T('app.applyOps.fail', T('app.ops.prefs'), String(err.message || err)));
      }
    });
    // 背景那一节（POST /prefs/background，2026-10-14 补上）。
    //
    // 它**必须 multipart**（文件走不了 urlencoded），所以不能像上一族那样把
    // FormData 转成 URLSearchParams——直接交 FormData，让浏览器自己带那一份
    // multipart boundary。这一族含文件，fetch 的体比 URLSearchParams 大一号，
    // 而它一年也就按几次。
    prefsSheet.addEventListener('submit', async (e) => {
      const form = e.target.closest('form[action="/prefs/background"]');
      if (!form) return;
      e.preventDefault();
      const data = new FormData(form, e.submitter || undefined);
      const file = form.querySelector('input[type=file]')?.files?.[0];
      // 空提交没有意义：没有文件、没按 remove、也没按 hidden。
      if (!file && !data.has('remove') && !data.has('hidden')) return;
      try {
        const res = await fetch('/prefs/background', {
          method: 'POST', body: data, credentials: 'same-origin', redirect: 'follow',
        });
        if (!res.ok) throw new Error('HTTP ' + res.status);
        // 背景的三个动作各自要面板**重画**（预览图、按钮组、那句"正在用它"都会变），
        // 而这一格画不出自己——所以这一族是唯一必须整页重载的：
        // 留在原地的话，屏上那一节会是**按下去之前的样子**，看着像没生效。
        location.reload();
      } catch (err) {
        showBanner(T('app.applyOps.fail', T('app.ops.bg'), String(err.message || err)));
      }
    });
  }

  // markPressed 就地把「哪一颗按下了」改过来——**不重载、不关面板**。
  //
  // 为什么它够：这一格只有两颗键，状态就写在自己的 aria-pressed 上
  // （TestPrefs_开关的标签稳定而状态走aria 钉的就是这一条）。
  // 服务端答 200/303 都说明写成了，而 applyOps 失败时会自己在屏上说话。
  function markPressed(form, value) {
    for (const b of form.querySelectorAll('button[aria-pressed]')) {
      b.setAttribute('aria-pressed', String(b.value === value));
    }
  }

  // groupOpFromForm 把「哪一个控件被按下了」翻成一条 op。
  //
  // 它必须**认名字而不是认顺序**：色板、排法与取色器在同一张表单里，而命名过的
  // 输入框每一次提交都跟着走。判据与 `httpapi/prefs.go` 那一处是同一份——
  // `apply_custom` 在场 = 取色器说话，否则 `accent` 说话；两个都不在 = 只改排法。
  function groupOpFromForm(form, data) {
    const gid = form.querySelector('input[name=group]')?.value;
    if (!gid) return null;
    const layout = data.get('layout');
    if (typeof layout === 'string' && layout) {
      return { op: 'set', path: `groups/${gid}/layout`, value: layout };
    }
    const accent = data.has('apply_custom') ? data.get('custom') : data.get('accent');
    if (typeof accent === 'string' && (accent === '' || /^#[0-9a-fA-F]{3}([0-9a-fA-F]{3})?$/.test(accent))) {
      return { op: 'set', path: `groups/${gid}/accent`, value: accent };
    }
    return null;
  }

  // 分组下拉的选项来自**服务端已经渲染在页面上的分组**。不重新拉一次 /api/v1/data：
  // 一次浮层开合不该为了拿一份没变的数据多发一个请求，而它还会带来"两处分组列表"
// 迟早不同步的那一类 bug。
  for (const h2 of document.querySelectorAll('main h2[data-group]')) {
    fGroup.append(el('option', { value: h2.dataset.group, text: h2.textContent.trim() }));
  }
  // 末尾那颗「＋ 新分组…」是**建组唯一的入口**。从前新分组只在"一个分组都还没有"
  // 的时候问（fNewGroup.hidden = fGroup.options.length > 0），于是有了书签之后
  // 界面上再也建不了新分组——而 ops 一直认 `add path=groups`。这是同一个洞的
  // 另一面：不是"选了不生效"，是"这一条根本不在界面上"。
  //
  // 哨兵值**不能是空串**：空串在上一个版本里表示「（不分组）」，而那一条是走不通
  // 的路（submit 判到空值就报错），界面摆着一条按下去必然失败的选项。
  fGroup.append(el('option', { value: NEW_GROUP, text: T('app.group.new') }));
  // 零选项的档案（刚建好、一条书签都没有）下拉里只剩这一颗：读作"新分组"，
  // 选它才问名字。那一档从前是**整个下拉藏起来、直接问名字**——两条路说的是同一
  // 件事，现在合成一条，少一个只有零分组时才走到的分支。
  if (fGroup.options.length === 1) fGroup.value = NEW_GROUP;
  syncGroupFields();

  // 图标下拉在 resolve 之前是**空的**（零选项的下拉框）：人看不到能选什么，
  // 也选不到"抓这个网站的图标"。两个语义值常驻，resolve 回来的候选再插进去。
  const iconLabel = (v) => v.replace(/^[a-z-]+:/, '');
  function fillIconChoices(sel, candidates, current) {
    sel.innerHTML = '';
    sel.append(
      el('option', { value: '', text: T('app.icon.none') }),
      el('option', { value: 'auto', text: T('app.icon.auto') }),
    );
    const list = candidates || [];
    // 盘上那个名字如果不在候选里，也**必须**还能被选着：否则打开模态什么都没改，
    // 一按保存就把它的图标清掉了——那一格说"没设图标"，而人从没碰过它。
    if (current && current !== 'auto' && !list.includes(current)) {
      sel.append(el('option', {
        value: current, selected: 'selected',
        text: iconLabel(current) + T('app.icon.currentSuffix'),
      }));
    }
    for (const c of list) sel.append(el('option', { value: c, text: iconLabel(c) }));
    if (current) sel.value = current;
  }
  function iconChoices(candidates) { fillIconChoices(fIcon, candidates, ''); }
  iconChoices([]);

  // ── 图标库：挑一枚图标（2026-10-14）────────────────────────────────────
  //
  // 内嵌索引有六千多个名字（品牌层 simple-icons + 概念层 tabler），而两个浮层原本
  // 只给三样：不选图标 / 自动按网址抓 / resolve 给的至多 5 条候选。库里那六千个
  // 于是**在界面上一个出口都没有**——ops 认 icon 字段、CLI 能查名字，而"给一条书签
  // 换个图标"只能去打接口。
  //
  // **图库不进 app.js、也不进首屏**：按搜索词向 `/api/v1/icons/tiles` 要一小格
  // **服务端画好的 HTML**（§7.3 那条操作线管的就是"只看首屏的每一页不该背一份图库"）。
  // 那一格用的是页面画卡片图标时同一个 svgMarkup，于是挑的时候看见的形状与存下去
  // 之后卡片上那一枚逐像素相同。
  const iconTilesLimit = 24;
  const iconPick = { sel: null, panel: null, input: null, grid: null, note: null, timer: 0, seq: 0, opener: null };

  // 「图标库…」那颗键。**它不进 <label> 里**：label 包着的控件是那个 <select>，
  // 而点 label 里的任何一点都会连带激活它——于是点「图标库…」会顺带把那颗下拉
  // 弹开，两件事一起发生，人看到的是一个说不清的界面。
  function iconLibraryButton(target) {
    return el('button', {
      type: 'button', class: 'fld', text: T('app.icon.lib'),
      on: { click: () => openIconPicker(target) },
    });
  }

  function openIconPicker(target) {
    closeIconPicker();
    const input = el('input', { type: 'search', class: 'fld', autocomplete: 'off',
      placeholder: T('app.icon.libPh'), 'aria-label': T('app.icon.libPh') });
    const grid = el('div', { class: 'icpick-grid' });
    // 空态说「先搜个名字」，而不是讲图库是怎么来的：面板打开时网格是空的，
    // 而那时候人要知道的**只有一件事**——往上面打字。libHint 那句来源说明
    // 另有位置常驻（网格下面），它是背景知识，不是此刻缺的那条指令。
    const note = el('p', { class: 'sheet-status', role: 'status', 'aria-live': 'polite', text: T('app.icon.libStart') });
    const panel = el('div', {
      class: 'sheet icpick', role: 'dialog', 'aria-modal': 'true', 'aria-label': T('app.icon.libTitle'),
    }, [
      el('h2', { text: T('app.icon.libTitle') }),
      input, grid, note,
      el('p', { class: 'hint icpick-hint', text: T('app.icon.libHint') }),
      el('button', { type: 'button', class: 'fld close', text: '×', 'aria-label': T('app.close'),
        on: { click: closeIconPicker } }),
    ]);
    body.append(panel); // 最后 append 的一层就是最上层：syncTrap 按 DOM 顺序判
    Object.assign(iconPick, { sel: target, panel, input, grid, note, opener: document.activeElement });
    syncTrap();
    input.focus();
    input.addEventListener('input', () => {
      clearTimeout(iconPick.timer);
      iconPick.timer = setTimeout(runIconSearch, 180);
    });
    input.addEventListener('keydown', (e) => {
      // 回车取第一格：搜出来的常常只有一个想要的名字，而手已经在回车上了
      if (e.key === 'Enter') {
        e.preventDefault();
        grid.querySelector('.icpick-tile')?.click();
      } else if (e.key === 'ArrowDown') {
        e.preventDefault();
        grid.querySelector('.icpick-tile')?.focus();
      }
    });
    grid.addEventListener('keydown', (e) => {
      if (e.key === 'ArrowUp' && document.activeElement === grid.querySelector('.icpick-tile')) {
        e.preventDefault();
        input.focus();
      }
    });
    grid.addEventListener('click', (e) => {
      const tile = e.target.closest('.icpick-tile');
      if (tile) pickIcon(tile.dataset.icon);
    });
    runIconSearch();
  }

  function closeIconPicker() {
    clearTimeout(iconPick.timer);
    const opener = iconPick.opener;
    if (iconPick.panel) {
      iconPick.panel.remove();
      syncTrap(); // 不调这一句，整页会留在 inert 里——面板没了，页面却还不能点
      opener?.focus();
    }
    // seq 归零同样重要：关掉之后还在飞的答复回来时不该去写已经不在的那一格
    Object.assign(iconPick, { sel: null, panel: null, input: null, grid: null, note: null, opener: null, seq: 0 });
  }

  async function runIconSearch() {
    const { input, grid, note } = iconPick;
    if (!input) return;
    const q = input.value.trim();
    const seq = ++iconPick.seq;
    const show = (msg, bad) => {
      note.textContent = msg;
      note.className = 'sheet-status' + (bad ? ' bad' : '');
    };
    if (!q) {
      grid.replaceChildren();
      show(T('app.icon.libStart'));
      return;
    }
    let markup = '';
    let total = 0;
    try {
      const res = await fetch('/api/v1/icons/tiles?q=' + encodeURIComponent(q) + '&limit=' + iconTilesLimit,
        { credentials: 'same-origin' });
      if (!res.ok) throw new Error('HTTP ' + res.status);
      total = Number(res.headers.get('X-LB-Icons-Total') || 0);
      markup = await res.text();
    } catch {
      if (iconPick.seq === seq) show(T('app.icon.libFail'), true);
      return;
    }
    // **迟到的答复丢掉**：先敲的词可能后到，不丢就会盖掉最后敲的那个，
    // 而这一格显示的是"你正在搜的那个词的结果"——盖错了等于答非所问。
    if (iconPick.seq !== seq) return;
    // 插入走 DOMParser 而不是 innerHTML：那一段是本服务自己画的（路径数据来自内嵌
    // 索引、属性值在 svgMarkup 里逐个转义），但它终究跨了一次 fetch，而 parse 出来
    // 再 replaceChildren 顺带把 script 与内联事件一起丢掉——"它是我们自己画的"是
    // 注释，parse 是机制。
    const tiles = [...new DOMParser().parseFromString(markup, 'text/html').body.children];
    grid.replaceChildren(...tiles);
    if (!tiles.length) show(T('app.icon.libNone', q));
    else if (total > tiles.length) show(T('app.icon.libMore', total, tiles.length));
    else show('');
  }

  function pickIcon(name) {
    const sel = iconPick.sel;
    if (!sel || !name) return;
    // 图库给的名字多半不在 resolve 的候选里（候选至多 5 条），所以要自己补一颗
    if (![...sel.options].some((o) => o.value === name)) {
      sel.append(el('option', { value: name, text: iconLabel(name) }));
    }
    sel.value = name;
    closeIconPicker();
  }

  // ── bookmarklet 带进来的一条（§6.3）────────────────────────────────────
  // 页面已经渲染完并带着 data-add（服务端透传，见 PageRequest）。这里只**预填**
  // 并把浮层打开，**不自动保存**：bookmarklet 从来不是"点一下就自动存"——
  // 它把你送到这里，你要看一眼标题对不对再按保存（§6.3 的形状）。
  //
  // 放在浮层与分组下拉**之后**：这段要碰 sheet/fUrl/fTitle，而它们是 const，
  // 写在前面就是 TDZ 报错——而 module 脚本的 TDZ 报错出现在**整个 IIFE 执行到
  // 那一行**时，看起来像"add 按钮没生成"，与真正的病因隔了半个文件。
  const pending = body.dataset.add || '';
  if (pending) {
    fUrl.value = pending;
    fTitle.value = body.dataset.addTitle || '';
    openSheet();
    resolveNow();
  }

  // ── resolve：填标题/描述/建议分组/候选图标 ───────────────────────────────
  // 失焦就跑一次（不是每次按键）：粘贴 URL 的动作天然发生在 blur 之前，而每敲一个
  // 字符打一次请求会把 resolve 的冷却与限流打满。
  fUrl.addEventListener('blur', resolveNow);

  async function resolveNow() {
    const url = fUrl.value.trim();
    if (!url) return;
    // 读之前先说"正在读…"——那是**这一次读取的状态**，归底部那条。
    say(T('app.reading'), 'busy');
    try {
      const r = await api('/api/v1/resolve', { url, profile });
      if (r.detail || r.code) {
        // 屏上说的仍是**这一类**的话：那一格的文本是给脚本对照的协议话，
        // 英文界面里冒出一句中文、还不说这是哪一步，等于让人自己猜。
        const key = msgKeyFor(r, r.code);
        console.warn('lb: resolve answered with an error body', r.code, r.kind || '', r.detail || '');
        const msg = T(KIND_KEY[r.kind] || (key === 'app.opsServerError' ? 'app.readFail' : key));
        // **地址类的错归地址那一格**：它是在说"你刚敲的那个地址不行"，
        // 而 resolve 又是 blur 触发的——那时焦点已经离开这一格，红字挂在
        // 表单最底端（隔着五格）等于没有。别的（读取失败之类）留在底部，
        // 因为它们说的是"这一次读取"而不是"哪一格不对"。
        say(msg, 'bad', isURLProblem(r) ? urlErr : null);
        return;
      }
      say(''); // 读成了：把上一轮任何一行状态都清掉（两行一起清，见 say 的注释）
      if (!fTitle.value) fTitle.value = r.title || '';
      if (!fDesc.value) fDesc.value = r.desc || '';
      if (r.suggestedGroup && fGroup.value !== NEW_GROUP) {
        for (const o of fGroup.options) {
          if (o.value === r.suggestedGroup) { fGroup.value = r.suggestedGroup; break; }
        }
      }
      iconChoices(r.iconCandidates);
      say(r.title ? T('app.readOk') : T('app.readEmpty'), r.title ? 'ok' : 'warn');
    } catch (e) {
      // **写失败一律显式报错，不静默吞**（§8.7）：一条"正在读取…"永远停在那儿，
      // 用户会以为它在加载。
      //
      // 带 status 的那一发，`e.message` 已经是 api() 按错误码挑出来的那句话，
      // 再套一层"读不出来："就成了"读不出来：这个地址还不完整——要有协议和主机名"
      // ——两句叠一句，实测英文站上就是 "Could not read it: That address isn't
      // complete yet"。没有 status 的才是真·意外（网络断了、脚本自己抛的），那时
      // 前面那句"读不出来："带着上下文才有用。
      say(e && e.status ? e.message : T('app.readErr', e.message), 'bad',
        e && e.status && isURLProblem(e) ? urlErr : null);
    }
  }

  // ── 写入 ──────────────────────────────────────────────────────────────────
  // id 是**必填的**（ops 校验：寻址全靠 id，不用下标，§4.4），且必须是 ASCII slug。
  //
  // 条目 id 从**地址**派生而不是从书签名派生：名字会被 resolve 自动填、也可能被
  // 手改，拿它当 id 意味着改一次名字这条的 id 就变了——而别人存过的链接会指空。
  // 域名是这条书签**不会变**的那一半。
  //
  // 分组 id 从分组名派生（新建分组那一条路），名字是中文时剥完剩下空串，
  // 于是补一个由同一串文字算出来的稳定后缀：**同一个名字永远得到同一个 id**，
  // 撞车由服务端回 422 说清，不在这里造一个"聪明的"去重。
  function ascii(text) {
    return String(text).toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 40);
  }
  function hash36(text) {
    let h = 2166136261;
    for (let i = 0; i < text.length; i++) h = (h ^ text.charCodeAt(i)) * 16777619 >>> 0;
    return h.toString(36);
  }
  // 条目的 id（寻址键）：**从地址派生**——名字会被 resolve 自动填、也会被人改，
  // 拿它当 id 意味着改一次名字这条的 id 就变了，而别人存过的链接会指空
  //（域名是地址里不会变的那一半）。
  //
  // **裸 IP 的主机名不走可读那条路**（2026-10-14 实拍改判）。原来一律取
  // `hostname` 的第一段，于是 `192.168.2.3:6789` / `:7080` / `:23333` **全部**
  // 变成 `192`——加第二个内网服务就撞 id，而 ops 撞 id 是硬错误（duplicate-id），
  // 症状是「明明填对了地址却存不进去」。而更早一版是整串 ascii，得到
  // `192-168-2-36789` 那种：它把**内网地址与端口烘进了 id**，而
  // `GET /api/v1/export` 发的是磁盘原字节——导出文件发给别人就等于把这段拓扑
  // 一起发出去。两种都坏，坏法不同。
  //
  // 现在：**主机是裸 IP 就用「主机+端口」的哈希**，既不泄漏拓扑、也天然区分
  // 端口。带域名的照旧走可读那条（`docs.google.com` → `docs`），而它不含任何
  // 私密信息。
  function slug(text, fromURL) {
    if (fromURL) {
      let u = null;
      try { u = new URL(text); } catch { /* 落到下面那一支 */ }
      if (!u) return ascii(text) || hash36(String(text)) || 'item';
      const host = u.hostname.replace(/^www\./i, '');
      // **四段全是数字** = 裸 IP（IPv4）。判据是这个而不是「含数字」——
      // `192.168.2.3.example.com` 那种仍走可读那条，而它不含私密信息。
      const isIPv4 = /^\d{1,3}(\.\d{1,3}){3}$/.test(host);
      if (isIPv4) return 'ip-' + (hash36(host + (u.port || '')) || 'item');
      const base = host.split('.')[0] + '-' +
        (u.pathname.split('/').filter(Boolean).pop() || '');
      return ascii(base) || ascii(text) || hash36(String(text)) || 'item';
    }
    return ascii(text) || 'g-' + hash36(String(text));
  }

  // 分组这一格：有分组可挑就挑一个；一份分组都没有（刚建好的档案）就用
  // 「新分组叫什么」那一格，把建组与放条目放进**同一批** ops——batch 是全成
  // 或全败的，不会出现"组建了条目没进去"。
  function groupOpsAndTarget() {
    if (fGroup.value !== NEW_GROUP) {
      const gid = fGroup.value;
      if (!gid) return { err: T('app.err.group') };
      return { gid, ops: [] };
    }
    const title = fNewGroup.value.trim();
    if (!title) return { err: T('app.err.groupName') };
    const gid = slug(title);
    // 一次 batch 里先建组再放条目——两件事本来就属于同一个意图（"把这条收进
    // 一个新分组"）。layout 带上人在浮层里挑的那一档，省得存完再去面板翻一遍。
    return { gid, ops: [{ op: 'add', path: 'groups', value: { id: gid, title, layout: shape, items: [] } }] };
  }

  async function submit(keepOpen) {
    const url = fUrl.value.trim();
    // 空地址同样归那一格（与 resolve 的地址类失败同一处）：它说的是这一格，
    // 而这一格下面就是它自己。人按了「保存」得到一句飘在五格之外的话，
    // 得自己判断那句是在说哪一格——而这一格是唯一可能空的那一格。
    if (!url) { say(T('app.err.url'), 'bad', urlErr); fUrl.focus(); return; }
    const name = fTitle.value.trim() || url;
    const value = { id: slug(url, true), name, url };
    if (fDesc.value.trim()) value.desc = fDesc.value.trim();
    if (fIcon.value) value.icon = fIcon.value;
    const g = groupOpsAndTarget();
    if (g.err) { say(g.err, 'bad'); return; }

    // 离线时**不报错也不假装成功**：进待办队列，等网络回来补上（§8.7）。
    // 这一支的差别是"什么时候写进去"，不是"写不写"——静默吞掉才是错的。
    if (!navigator.onLine) {
      enqueue({ gid: g.gid, ops: g.ops, value });
      say(T('app.offline.queued'), 'warn');
      if (keepOpen) { for (const f of [fUrl, fTitle, fDesc]) f.value = ''; } else { sheet.hidden = true; }
      paintQueue();
      return;
    }

    saveBtn.disabled = moreBtn.disabled = true;
    say(T('app.saving'), 'busy');
    try {
      const r = await api('/api/v1/batch', {
        profile, expectedRev: rev, by: 'web',
        ops: [...g.ops, { op: 'add', path: 'groups/' + g.gid + '/items', value }],
      });
      rev = r.rev; // 同一页再存第二条要拿新的 rev，否则下一条必然 409
      say(T('app.saved'), 'ok');
      // 刚建出来的那一组**就地进下拉并选中**：连着往同一个新分组里加第二条时，
      // 若下拉还停在「＋ 新分组…」且名字框留着上一次的内容，下一次就会拿同一个
      // group id 再 `add` 一遍——ops 拒掉（重复 id），而屏幕上那句话说的是
      // "保存失败"，人不知道自己刚才已经把第一条存进去了。
      //
      // 列表是服务端渲染的，所以这一条只是**这一层浮层**的视图；重载之后下拉
      // 会从页面上重新长一遍，那时它已经在列表里了。
      if (g.ops.length && !fGroup.querySelector('option[value="' + g.gid + '"]')) {
        const opt = el('option', { value: g.gid, text: fNewGroup.value.trim() });
        fGroup.insertBefore(opt, fGroup.lastElementChild);
        fGroup.value = g.gid;
        syncGroupFields();
      }
      if (keepOpen) {
        for (const f of [fUrl, fTitle, fDesc]) f.value = '';
        iconChoices([]);
        fUrl.focus();
      } else {
        sheet.hidden = true;
        syncTrap();
        reloadKeepingPlace(); // 列表是服务端渲染的：不重载就看不到刚存的那条
      }
    } catch (e) {
      // 409 是"数据变了"，不是"再试一次就好"：让人自己看清了刷新。
      say(e.status === 409 ? T('app.conflict') : e.message, 'bad');
    } finally {
      saveBtn.disabled = moreBtn.disabled = false;
    }
  }

  // ── ⑥ 离线待办队列（§8.7）─────────────────────────────────────────────────
  // 三条纪律都在代码里：
  //   - **不自动合并**：撞了 409 就把那条**丢掉并逐条说清**，让人重加一次比猜着
  //     合并便宜；
  //   - **不无限重试**：恢复在线只重放一遍，失败就不再自动来第二次；
  //   - **写失败一律显式报错**，不静默吞。
  const QKEY = 'lb-queue-' + profile;
  const readQueue = () => {
    try { return JSON.parse(localStorage.getItem(QKEY) || '[]'); } catch { return []; }
  };
  const writeQueue = (q) => {
    try { localStorage.setItem(QKEY, JSON.stringify(q)); } catch { /* 存不下就算了，别静默 */ }
  };

  function enqueue(entry) {
    const q = readQueue();
    q.push({ at: Date.now(), ...entry });
    writeQueue(q);
    paintQueue();
  }

  // 「待办 N 条」从前是一枚 `<span>`：点不开、看不到是哪几条、也删不掉。
  // 一个只报数量的读数等于让人猜自己存了什么。现在它是按钮，点开是清单，
  // 每条能单独撤掉。
  function paintQueue() {
    const n = readQueue().length;
    let chip = $('#lb-q');
    if (n && !chip) {
      chip = el('button', { type: 'button', class: 'badge', id: 'lb-q',
        'aria-label': T('app.offline.aria'), on: { click: showQueue } });
      header.append(chip);
    }
    if (chip) chip.textContent = T('app.offline.chip', n);
    if (!n && chip) chip.remove();
    if (!n && $('#lb-qlist')) $('#lb-qlist').remove();
    paintOnline();
  }

  function showQueue() {
    const old = $('#lb-qlist');
    if (old) { old.remove(); return; }
    const q = readQueue();
    if (!q.length) return;
    const list = el('ul', { class: 'qlist', id: 'lb-qlist' }, q.map((entry, i) => el('li', {}, [
      el('b', { text: entry.value.name }),
      el('code', { text: entry.value.url }),
      el('button', { type: 'button', class: 'fld', text: T('app.offline.drop'), on: { click: () => {
        const left = readQueue();
        left.splice(i, 1);
        writeQueue(left);
        paintQueue();
      } } }),
    ])));
    header.after(el('div', { class: 'notice', role: 'status' }, [
      el('p', { text: T('app.offline.list') }), list,
    ]));
  }

  function paintOnline() {
    const off = !navigator.onLine;
    const badge = $('#lb-off');
    if (off && !badge) {
      header.prepend(el('p', { class: 'notice', id: 'lb-off', role: 'status',
        text: T('app.offline.blocked') }));
    } else if (!off && badge) {
      badge.remove();
    }
  }

  let replaying = false;
  async function replay() {
    if (replaying) return;
    const q = readQueue();
    if (!q.length) { paintQueue(); return; }
    replaying = true;
    let done = 0;
    const dropped = [];
    // **重放前先取最新 rev**（§8.7）：拿队列存下来的那个 rev 一定会撞 409，
    // 而那个 409 说的不是"数据变了"而是"你自己过期了"。
    try {
      const cur = await (await fetch('/api/v1/data?profile=' + encodeURIComponent(profile),
        { credentials: 'same-origin' })).json();
      rev = cur.rev;
    } catch { /* 取不到就按队列里的 rev 试，撞了会逐条丢弃 */ }

    const left = [];
    for (const entry of q) {
      try {
        const r = await api('/api/v1/batch', {
          profile, expectedRev: rev, by: 'web',
          ops: [...(entry.ops || []), { op: 'add', path: 'groups/' + entry.gid + '/items', value: entry.value }],
        });
        rev = r.rev;
        done++;
      } catch (e) {
        // **409 的那条丢掉并点名**：补一次之后还有 409 说明别人真的改过了，
        // 再自动重试下去只是把同一件事重复三遍。让人自己看着办。
        if (e.status === 409) dropped.push(entry.value.name + '（' + entry.value.url + '）');
        else left.push(entry);
      }
    }
    writeQueue(left);
    paintQueue();
    replaying = false;

    if (done || dropped.length) {
      const parts = [];
      if (done) parts.push(T('app.offline.done', done));
      if (dropped.length) parts.push(T('app.offline.dropped', dropped.length, dropped.join(T('app.sep'))));
      showBanner(parts.join('；'));
    }
    if (left.length) {
      showBanner(T('app.offline.left', left.length, left.map((e) => e.value.url).join(T('app.sep'))));
    }
    if (done) reloadKeepingPlace();
  }

  paintOnline();
  paintQueue();
  addEventListener('online', () => { paintOnline(); replay(); });
  addEventListener('offline', paintOnline);

  // ── api：同一份错误形状，不让每个调用点各写一遍 ──────────────────────────
  async function api(path, body) {
    const res = await fetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      credentials: 'same-origin', // 会话 cookie 就是凭证（credScoped）
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) {
      // **按状态码换成带出路的句子，屏上不出现服务端那格协议话。**
      //
      // 服务端那一格（detail 与 errors[].reason）是给脚本对照的，固定中文
      // （AGENTS.md「翻的与不翻的」）。从前只有 401 换过句子，其余原样贴进横幅：
      // 实测 422 那一份出来是「这一下没存上：没有分组 "nope"」——英文界面里一句
      // 中文，末尾还挂着一个人在界面上根本看不见的 id；它既不说这是哪一步失败了，
      // 也不说下一步该按什么。
      //
      // 401 在这一条通路上只有一种意思（会话没了：cookie 被清，或页面是 Service
      // Worker 从缓存里给的），所以它说"重新进一次"。403 说的是这一页与这一份
      // 对不上号。422 在界面上的成因只有一个：那一组或那一条已经不在了（别人改过、
      // 或这一页开着很久）。507 是快照写不下去，而那一句必须说清**什么都没改**，
      // 否则人会以为改坏了。其余（500/503）不是这一页能解决的，说"稍后再试"。
      //
      // 原文不进屏，但也不许丢：进 console——排查的人与 AI 打的是 devtools 与容器
      // 日志，那里有完整的 field/reason。注释里不复述那几字：整份 app.js 原样下发，
      // 字面串会喂饱"客户端不许抄服务端协议话"那条闸。
      const first = Array.isArray(data.errors) && data.errors[0];
      // **kind 优先，状态码兜底**。服务端每个错误都带 kind（AGENTS.md「错误码」），
      // 而同一个状态码在两条通路上说的是两件相反的事：422 既可能是"那一组已经不
      // 在了"（该说：刷新看看），也可能是"这一条已经存过了"（该说：去改它，别再
      // 存第二遍）。上一版只按状态码选，于是重复添加同一网址的人看到的是"东西没
      // 了，刷新一下"——实测抓到的就是这一条。
      // errors[0].kind 先读：batch 的外壳那一格说的是**这一族**（ops-invalid = 整批
      // 未写入），具体哪一条、为什么在明细里。
      const msgKey = msgKeyFor(data, res.status);
      const why = msgKey === 'app.opsServerError' ? T(msgKey, String(res.status)) : T(msgKey);
      // 标签不再写 "write request"：这一条通路也服务 resolve，那是**读**。
      console.warn('lb: request rejected', res.status, kindOf(data),
        (first && (first.reason || first.field)) || data.detail || '');
      const e = new Error(why);
      e.status = res.status;
      // **kind 一并带走**：屏上判断"这句话说的是不是地址本身不行"靠它，
      // 而不是靠状态码——422 同时是 batch 的"那一组已经不在了"，按码判就会
      // 把一次保存失败贴到地址那一格去。抛出时丢掉它，调用方就只能猜。
      e.kind = kindOf(data) || (first && first.kind) || '';
      throw e;
    }
    return data;
  }

  // 服务端渲染的列表改完必须重载才看得见，但"重载"在 200 条的台子上等于把人
  // 正在看的那一屏扔回顶部。存一个滚动位置，回来时接着放。
  const SCROLLKEY = 'lb-scroll-' + profile;
  function reloadKeepingPlace() {
    try { sessionStorage.setItem(SCROLLKEY, String(scrollY)); } catch { /* 存不下就退回最上面 */ }
    location.reload();
  }
  const back = (() => { try { return Number(sessionStorage.getItem(SCROLLKEY) || '0'); } catch { return 0; } })();
  if (back) {
    sessionStorage.removeItem(SCROLLKEY);
    addEventListener('load', () => scrollTo(0, back), { once: true });
  }

  // ── ⌘/Ctrl K 命令面板（§8.6）───────────────────────────────────────────
  // **叠加能力而不是默认视图**：它是覆盖在上层的一个框，页面本身照常可点、
  // 可用浏览器找词。做成"另一个页面"的话，⌘K 就把搜索框和阅读位置全丢了。
  const cmdkInput = el('input', { type: 'text', id: 'lb-k', class: 'fld', placeholder: T('app.cmdk.placeholder'), autocomplete: 'off' });
  const cmdkList = el('ul', { class: 'cmdk-list', role: 'listbox' });
  const cmdk = el('div', { class: 'cmdk', hidden: 'hidden', role: 'dialog', 'aria-modal': 'true', 'aria-label': T('app.cmdk.aria') }, [
    cmdkInput, cmdkList,
  ]);
  body.append(cmdk);

  let cmdkHits = [], cmdkSel = 0;
  function cmdkOpen() {
    cmdk.hidden = false;
    syncTrap();
    cmdkInput.value = '';
    cmdkRender();
    cmdkInput.focus();
  }
  function cmdkClose() { cmdk.hidden = true; syncTrap(); }

  function cmdkRender() {
    const q = cmdkInput.value.trim().toLowerCase();
    // 快捷动作与书签命中**混在同一个列表**：面板是"做点什么"，不是"搜书签"。
    //
    // 名单按**这台浏览器能不能真做到**给。读剪贴板只在 https 或 localhost 下可用，
    // 而家里用的是 http://内网地址——那一档上这条永远只会回一句"不给读"，
    // 列出来就是一个按下去必然失败的入口（§8.1：点了没反应的东西不进界面）。
    //
    // 从前这里还有第二条「看这个档案的改动历史」，它做的事是导航到
    // /api/v1/history——把人甩进一屏 JSON。那是接口不是界面，先撤掉。
    // 2026-10-13 这一格回来了，去的是 /history：那一屏现在有自己的脸
    // （审计线一直在写，缺的一直是屏）。它不进 `navigator.clipboard` 那一档判断，
    // 因为这一页不需要浏览器给任何权限。
    const actions = [];
    actions.push({ kind: 'action', label: T('app.cmdk.history'), run: () => { location.href = '/history'; } });
    if (navigator.clipboard?.readText) {
      actions.push({ kind: 'action', label: T('app.cmdk.clipboard'), run: addClipboard });
    }
    const hits = [];
    // 从前这里数到 8 就 `break`——一千条那一份实测搜「第」命中 1000 条，
    // 面板却只列 8 行且**什么都不说**，于是"没有更多了"与"我只给你看 8 条"
    // 长得一模一样。现在扫完再截，并把剩下的数目变成列表里最后一条真动作：
    // 选中它并按回车，把这个词交给页面上那个搜索框（整页搜是服务端渲染那条路）。
    let total = 0;
    for (const li of document.querySelectorAll('li[data-item]')) {
      const a = li.querySelector('.card');
      const name = (li.querySelector('b')?.textContent || '').toLowerCase();
      const url = (a?.getAttribute('href') || li.querySelector('code')?.textContent || '').toLowerCase();
      // 判据与 `filter.go` 的 matchItem 是**同一组字段**：名字、地址、说明、每一个
      // 别名、每一个标签，再加"分组标题命中就把整组带出来"。从前这里只比名字与地址，
      // 于是同一个词在同一个产品里有两个答案——实测（同一份数据、两条通路各跑 11 个词）：
      // 「NAS」整页 8 条 / 面板 2 条，「影音」（那是组名）3 条 / 0 条，「pt」「备份」
      // 「对象存储」都是服务端有、面板回一句"没有叫「X」的书签"。
      // 值从 li 上的 data-desc / data-alias / data-tags 读，多个值以换行连接。
      const gtitle = (li.closest('section')?.querySelector('.sec-leaf')?.textContent || '').toLowerCase();
      const extra = [li.dataset.desc || '', ...(li.dataset.alias || '').split('\n'), ...(li.dataset.tags || '').split('\n')];
      if (!q || name.includes(q) || url.includes(q) || gtitle.includes(q)
        || extra.some((x) => x && x.toLowerCase().includes(q))) {
        total++;
        if (hits.length < 8) {
          hits.push({ kind: 'item', label: li.querySelector('b')?.textContent || '', url: a?.getAttribute('href') || '', el: li });
        }
      }
    }
    cmdkHits = [...actions.filter((x) => !q || x.label.toLowerCase().includes(q)), ...hits];
    if (total > hits.length && q) cmdkHits.push({ kind: 'more', label: T('app.cmdk.more', q, total), run: () => toPageSearch(q) });
    cmdkSel = 0;
    cmdkList.innerHTML = '';
    for (const h of cmdkHits) {
      const row = el('li', {
        role: 'option', 'aria-selected': 'false',
        on: {
          click: () => cmdkRun(h),
          mouseenter: () => { cmdkSel = cmdkHits.indexOf(h); cmdkPaint(); },
        },
      }, h.kind === 'more'
        // 这一行不是书签，不能长得像书签：没有地址、没有类型角标，只有一句动作。
        ? [el('span', { text: h.label })]
        : [el('b', { text: h.label }), h.url ? el('code', { text: h.url }) : el('span', { class: 'tag', text: T('app.cmdk.kind') })]);
      if (h.kind === 'more') row.classList.add('cmdk-more');
      cmdkList.append(row);
    }
    // 零命中从前是**一块空白面板**：没有一句话，人不知道自己搜砸了还是它坏了。
    if (!cmdkHits.length) {
      cmdkList.append(el('li', { class: 'cmdk-empty', role: 'presentation',
        text: q ? T('app.cmdk.none', q) : T('app.cmdk.empty') }));
    }
    cmdkPaint();
  }
  function cmdkPaint() {
    [...cmdkList.children].forEach((row, i) => {
      row.setAttribute('aria-selected', i === cmdkSel ? 'true' : 'false');
      row.classList.toggle('sel', i === cmdkSel);
    });
  }
  function cmdkRun(hit) {
    cmdkClose();
    if (hit.kind === 'item') { location.href = hit.url; return; }
    hit.run();
  }
  // 把面板里这个词交给页面上那个搜索框——它走的是服务端那条整页搜索，
  // 命中的是全部条目，不受这里 8 行的展示上限约束。
  function toPageSearch(q) {
    const s = document.querySelector('input[type=search]');
    if (!s || !s.form) return;
    s.value = q;
    if (s.form.requestSubmit) s.form.requestSubmit(); else s.form.submit();
  }
  function addClipboard() {
    // 这条动作只在 `navigator.clipboard?.readText` 真的存在时才会被列进面板
    // （见 cmdkRender），下面两道判据是**贴进来不像网址**与**系统拒了权限**。
    openSheet();
    if (!navigator.clipboard?.readText) {
      say(T('app.clip.denied'), 'warn');
      fUrl.focus();
      return;
    }
    navigator.clipboard.readText().then((t) => {
      const u = (t || '').trim();
      if (!/^https?:\/\//i.test(u)) {
        say(T('app.clip.noturl'), 'warn');
        fUrl.focus();
        return;
      }
      fUrl.value = u;
      resolveNow();
    }).catch(() => {
      say(T('app.clip.noread'), 'warn');
      fUrl.focus();
    });
  }

  cmdkInput.addEventListener('input', cmdkRender);
  cmdkInput.addEventListener('keydown', (e) => {
    if (e.key === 'ArrowDown') { e.preventDefault(); cmdkSel = (cmdkSel + 1) % (cmdkHits.length || 1); cmdkPaint(); return; }
    if (e.key === 'ArrowUp') { e.preventDefault(); cmdkSel = (cmdkSel - 1 + (cmdkHits.length || 1)) % (cmdkHits.length || 1); cmdkPaint(); return; }
    if (e.key === 'Enter') { e.preventDefault(); if (cmdkHits[cmdkSel]) cmdkRun(cmdkHits[cmdkSel]); return; }
    if (e.key === 'Escape') { e.preventDefault(); cmdkClose(); }
  });

  // ── ⑤ 就地编辑（§8.4）：✎ 把名字/描述/地址变成表单，失焦即写库 ─────────
  //
  // **失焦即写，而不是"点保存"**：多一个按钮就多一次点错的余地，而写错了有
  // revert 可退（§11 的"不误改"那一档）。但写**失败必须显式说**，否则改完刷新
  // 就回去了，而人不会知道。
  // 这一组叫什么。**取 h2 的 title 而不是 textContent**：那一行里还住着布局切换键
  // （「磁贴」/「单列」），按 textContent 会念成「文档与账号单列」——实测到的。
  // title 挂的是完整分组名，截断与悬停都不丢信息（模板那边写死的）。
  function groupName(sec) {
    const h2 = sec?.querySelector('h2');
    if (!h2) return sec?.dataset.gid || '';
    return (h2.getAttribute('title') || h2.querySelector('.sec-leaf')?.textContent || sec.dataset.gid || '').trim();
  }

  function opsFor(li, fields) {
    const gid = li.closest('section')?.dataset.gid;
    const iid = li.dataset.item;
    return Object.entries(fields)
      .filter(([, v]) => v !== null)
      .map(([k, v]) => ({ op: 'set', path: `groups/${gid}/items/${iid}/${k}`, value: v }));
  }

  async function applyOps(ops, what) {
    try {
      const r = await api('/api/v1/batch', { profile, expectedRev: rev, by: 'web', ops });
      rev = r.rev;
      return true;
    } catch (e) {
      // 409 也已经在 api() 里按状态码换过句子了（app.opsConflict），这里不再
      // 分第二次——两处各判一遍同一个状态码，迟早有一处忘了加新一档。
      showBanner(T('app.applyOps.fail', T(what), e.message));
      return false;
    }
  }

  // §8.1「点了没反应的东西不进界面」的第三处处置（另两处是顶栏「＋ 添加」与就地
  // 编辑把手）：**复制键由这一段造，模板不渲染它**。模板那边留一枚
  // `<span class="badge warn ext">`，它有双份身份——关掉 JS 时它仍是那一格唯一一句
  // 能读懂的话（「浏览器不让页面打开这种地址」是真的），有 JS 时就地换成一颗能按的
  // 键。判据只有一处：地址取同一个 `<code>` 的 textContent，模板与脚本都不再抄一份
  // URL（⌘K 搜外部条目时读的是同一处）。
  //
  // 为什么不用 navigator.clipboard 就完事：那是 **secure context** 才有的 API，而这
  // 一页在家里就是 http://192.168.x.x:7600（§11 的前提：只在家里，不上公网）——主力
  // 部署形态下 navigator.clipboard 直接是 undefined。所以退到 execCommand：它不挑
  // 协议，虽然早已 deprecated 但桌面浏览器都还认。两条都不成时退到「把地址选中」，
  // 人按一下 Ctrl+C 也就完事——这一格**永远有一个出路**，不留一颗按了没反应的键。
  async function copyText(text) {
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(text);
        return true;
      }
    } catch { /* 非安全上下文或被拒：走下面那条 */ }
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.setAttribute('readonly', '');
    ta.style.cssText = 'position:fixed;top:0;left:0;opacity:0;pointer-events:none';
    document.body.append(ta);
    ta.select();
    ta.setSelectionRange(0, text.length);
    let ok = false;
    try { ok = document.execCommand('copy'); } catch { ok = false; }
    ta.remove();
    return ok;
  }

  // 图标键。`⧉`（U+29C9 两块相叠的方块）是「复制」在纯文本里最通用的一枚字形，
  // 同一族里的 `✎`（把手）已经在每一张卡上跑了很久——一枚字形能显示的系统，
  // 另一枚基本也能；万一不显示，它 44×44 的空盒子仍在，而旁边的地址一字不少。
  const COPY = '\u29C9';
  for (const badge of document.querySelectorAll('.plain .badge.warn.ext')) {
    const plain = badge.parentElement;
    const code = plain?.querySelector('code');
    const url = (code?.textContent || '').trim();
    const name = plain?.querySelector('b')?.textContent?.trim() || T('app.edit.fallbackName');
    if (!url) continue;
    const tip = badge.getAttribute('title') || '';
    // **拖到书签栏这条路要带得上地址**（用户实测：拖过去的那一格没有 URL）。
    // 提示里那句"拖到收藏夹栏再从那里打开"一直是真出路——Chrome 拦的只是
    // "网页发起的跳转"，从书签栏点不经过那道闸。但这一行是 <span> 而不是
    // <a href>：Go 的模板把 chrome:// 与 javascript: 一视同仁换成 #ZgotmplZ，
    // 绕开它（template.URL）等于给"存进库的脚本在我们同源下变成活链接"开门，
    // 而这一页带着会话 cookie。所以负载自己放：拖走的是地址，不是选中的文字。
    if (plain) {
      plain.setAttribute('draggable', 'true');
      plain.addEventListener('dragstart', (e) => {
        const dt = e.dataTransfer;
        if (!dt) return;
        dt.effectAllowed = 'copy';
        dt.setData('text/uri-list', url);
        dt.setData('text/plain', url);
      });
    }
    let timer = 0;
    // **这一颗与 .pencil 同一张脸**（44×44 的幽灵键，app.css 的 .copybtn），
    // 不穿 .badge：实拍过一版带字的徽标，77×44 的框比同排的书签名还响——一个
    //「今天要点」的键不该比「今天要点的东西」更显眼。base.css 那条「span 是状态、
    // a/button 是动作」也是同一件事：动作键不留警告脸。「这一行点不动」由它没有
    // 链接形态、下面那行是纯文本来表达。
    const btn = el('button', {
      type: 'button', class: 'copybtn', text: COPY,
      'aria-label': T('app.copy.aria', name), title: tip,
      on: {
        click: async (e) => {
          e.preventDefault();
          const said = await copyText(url);
          // 反馈是**换字形**而不是换文字：图标键上没有文字可换，而一颗不吭声的
          // 图标键等于按了没反应。✓ 是「成了」，! 是「没成，接着按 Ctrl+C」。
          btn.textContent = said ? '\u2713' : '!';
          const saidText = said ? T('app.copy.done') : T('app.copy.fail');
          btn.title = saidText;
          btn.setAttribute('aria-label', saidText);
          if (!said) {
            // 两条路都不通：把地址选中，让 Ctrl+C 立刻有效——比留一颗假装成功的键好。
            const r = document.createRange();
            r.selectNodeContents(code);
            const sel = getSelection();
            sel.removeAllRanges();
            sel.addRange(r);
          }
          clearTimeout(timer);
          timer = setTimeout(() => {
            btn.textContent = COPY;
            btn.title = tip;
            btn.setAttribute('aria-label', T('app.copy.aria', name));
          }, 1600);
        },
      },
    });
    badge.replaceWith(btn);
  }

  for (const li of document.querySelectorAll('li[data-item]')) {
    // 把手与拖拽挂在**这一行的主体**上：可点的地址是 .card，点不动的那几行是 .plain。
    // 2026-10-12 之前这里只认 .card，注释写的是"非 http 的不给把手（不给做不到的承诺）"——
    // 那句把两件事混成了一件：**打不开**是浏览器的限制，**改不了**不是。
    // 就地编辑写的是 name/desc/url，与协议无关；把把手摘掉等于让一条 chrome://
    // 或 file:// 的书签在界面上永远改不了名、换不了说明，而浏览器收藏夹导入
    // 恰好会把这类地址成批收进来（AGENTS.md：file: 照收）。
    const card = li.querySelector('.card') || li.querySelector('.plain');
    if (!card) continue; // 两种形态都不是（坏档那一屏根本没有 li）才真的没有把手
    const pencil = el('button', {
      type: 'button', class: 'pencil', text: '✎', 'aria-label': T('app.edit.aria', li.querySelector('b')?.textContent || T('app.edit.fallbackName')),
      on: { click: (e) => { e.preventDefault(); editItem(li); } },
    });
    // 两枚键的落点：可点的那张卡上只有一把手，挂在卡片本体（角上）——
    // 「点不动那一行」上**两枚键一起住进地址那一行**（`.addr`，模板给的容器），
    // 把手就跟着复制键进去。分开放看着"分处两行就不可能相交"，实拍是它们叠在
    // 右下角（把手 `top:6px` 高 44px 落到 y 6–50，而第二行从 y 32 起算，重叠 18px）。
    // 同一行里并排是**结构上**不相交，不是算下来不相交。
    const keys = card.querySelector('.addr') || card;
    keys.append(pencil);
    // draggable 要**在拖之前就位**：从前它在 dragstart 的回调里才设上，而浏览器
    // 是在 pointer 按下那一刻读这个属性的——第一次拖根本不会开始，得人拖过一回
    // （属性已经留下了）第二次才动。
    card.setAttribute('draggable', 'true');
    card.addEventListener('dragstart', (e) => {
      const dt = e.dataTransfer;
      // 排序走**自定义 MIME**，`text/plain` 与 `text/uri-list` 留给真地址。
      // 从前这里把条目 id 塞进 text/plain——于是把一张卡片拖到书签栏，
      // 落下来的是一个叫「group-2-chrome-apps」的坏书签（与"点不开那一行"
      // 同一件事的另一半：拖走的东西该是那条书签的地址，不是我们的内部主键）。
      dt.setData('application/x-lb-item', li.dataset.item);
      const href = card.getAttribute('href') || '';
      if (href) {
        dt.setData('text/uri-list', href);
        dt.setData('text/plain', href);
      }
      dt.effectAllowed = 'copyMove';
    });
    li.addEventListener('dragover', (e) => { e.preventDefault(); e.dataTransfer.dropEffect = 'move'; li.classList.add('drop'); });
    li.addEventListener('dragleave', () => li.classList.remove('drop'));
    li.addEventListener('drop', async (e) => {
      e.preventDefault();
      li.classList.remove('drop');
      // 只认自己那套 MIME：从别处（桌面、别的标签页）拖进来的东西没有它，
      // 那种情况下不该动书签的顺序。
      const moved = e.dataTransfer.getData('application/x-lb-item');
      if (!moved || moved === li.dataset.item) return;
      const gid = li.closest('section')?.dataset.gid;
      const sibs = [...li.closest('section').querySelectorAll('li[data-item]')];
      let index = sibs.indexOf(li);
      // 同组往下挪时，被拖的那一条**还算在列表里**：直接落到目标下标会插到
      // 它后面去（想放第 3 位，结果停在第 4 位）。取走它之后的位置才是答案。
      const from = sibs.findIndex((x) => x.dataset.item === moved);
      if (from >= 0 && from < index) index -= 1;
      // 同组排序靠 to=自己那组 + index；跨组是 §8.4 的批量移动那条路。
      const ok = await applyOps([{ op: 'move', path: `groups/${gid}/items/${moved}`, to: gid, index }], 'app.ops.move');
      if (ok) reloadKeepingPlace();
    });
  }

  // 编辑一条书签：三个字段 + 存 / 取消 / 删掉这条。
  //
  // 注释先前写"改完就写，不摆保存大按钮（失焦即写那条裁定见 §8.4）"——那是**旧的**
  // 交互，代码里早就换成「存 / 取消」两颗按钮（失焦即写在"改一半点到别处"时会
  // 静默落库，而这一格没有第二次机会反悔）。注释替不存在的实现说话，比没注释更坏，
  // 所以这一句改口成它今天的样子。
  //
  // **要有关掉它的出路**：从前这一格只有一个「存」，按 Esc 什么都不做
  // （Esc 只关浮层与面板），再点 ✎ 也被 `if (form) return` 挡掉——
  // 手滑点开的人只能刷新页面才能退出。
  //
  // **它是模态的一层，不是贴在卡片上的一坨**（2026-10-13）。这个 form 从前挂在
  // `li` 里、绝对定位盖住那张卡，于是宽度、出屏、被 `content-visibility` 裁掉
  // 这三件事都要在它自己的规则里各补一刀（`min(max(100%,300px),420px)`、
  // `.flip`、`section:has(.inline-edit)`）。现在它挂到 body 上、与「＋ 添加」
  // 共用 `.sheet` 那一族：居中、宽度上限、背板、`max-height` 滚动、以及
  // `syncTrap` 那圈 inert，都是族里现成的规矩。三条补丁连同它们各自的闸一起删。
  let openEdit = null;

  function closeEdit() {
    if (!openEdit) return;
    const { li, form } = openEdit;
    openEdit = null;
    form.remove();
    syncTrap();
    li.querySelector('.pencil')?.focus();
  }

  function editItem(li) {
    if (openEdit && openEdit.li === li) return;
    if (openEdit) closeEdit();
    const a = li.querySelector('.card');
    const name = li.querySelector('b')?.textContent || '';
    // 地址从**这一行本来显示它的那个元素**上读：链接形态读 href，点不动的那一行
    // 读 <code> 的文本（模板与复制键读的是同一处，不另抄一份 URL）。
    const url = a ? (a.getAttribute('href') || '') : (li.querySelector('code')?.textContent || '').trim();
    const desc = li.querySelector('i')?.textContent || '';
    const sec = li.closest('section');
    const gid = sec?.dataset.gid || '';
    // 分组那一行不是装饰：面板浮起来之后底下整屏都不可点，"我在改哪一组的哪一条"
    // 只剩这里能说，而删除那一步的确认句要引用它。
    const gname = groupName(sec);
    const f = (k, v) => el('input', { type: 'text', class: 'fld', name: k, value: v, autocomplete: 'off' });
    const n = f('name', name), d = f('desc', desc), u = f('url', url);
    // 图标这一格从前只在「＋ 添加」那一层有——于是"给一条已经在库里的书签换个图标"
    // 在界面上没有任何出口：能改内容的那一层没有图标，能选图标的那一层只用于新增。
    // 导入报告因此只能说"导完之后用 POST /api/v1/batch 逐条设 icon"，把家里人指去
    // 打接口（那是 2ddeb12 那一族「能配却没有出口」的又一格）。
    const curIcon = li.dataset.iconName || '';
    const ic = el('select', { id: 'lb-edit-icon', class: 'fld' });
    fillIconChoices(ic, [], curIcon);
    const lookIcon = el('button', {
      type: 'button', class: 'fld', text: T('app.icon.fetch'),
      on: {
        click: async () => {
          lookIcon.disabled = true;
          try {
            const r = await api('/api/v1/resolve', { url: u.value.trim() || url, profile });
            fillIconChoices(ic, r.iconCandidates, ic.value || curIcon);
          } catch (e) {
            showBanner(T('app.readErr', e.message), 'bad');
          } finally {
            lookIcon.disabled = false;
          }
        },
      },
    });
    const undo = el('p', { class: 'edit-undo', text: T('app.delete.undoHint') });
    // 删除是**两步**，两步都在这一格里走完：第一下把那颗键换成"确定删掉？"并露出
    // 那句"还能用回退找回来"，第二下才发 op。为什么不用 confirm()——那是 §8.8
    // 早就判过一次的：样式不可控、移动端常常直接不出现，而它是唯一挡在"手滑删掉
    // 一条书签"前面的东西。超时自己退回第一步，所以"按了一半走开"不会留下武装态。
    let armed = false;
    let armTimer = 0;
    const del = el('button', {
      type: 'button', class: 'fld danger', text: T('app.delete.item'),
      on: {
        click: async () => {
          if (!armed) {
            armed = true;
            del.classList.add('armed');
            del.textContent = T('app.delete.confirm', name);
            undo.hidden = false;
            clearTimeout(armTimer);
            armTimer = setTimeout(() => {
              armed = false;
              del.classList.remove('armed');
              del.textContent = T('app.delete.item');
              undo.hidden = true;
            }, 8000);
            return;
          }
          clearTimeout(armTimer);
          const ops = [{ op: 'remove', path: `groups/${gid}/items/${li.dataset.item}` }];
          if (await applyOps(ops, 'app.ops.delete')) reloadKeepingPlace();
        },
      },
    });
    const form = el('form', {
      class: 'sheet inline-edit', role: 'dialog', 'aria-modal': 'true', 'aria-labelledby': 'lb-edit-h',
      on: {
        submit: async (e) => {
          e.preventDefault();
          // icon 只在**人真的换过**时发：没动过就不许发一条 set 把它清掉
          // （下拉里"不选图标"是空串，与"当前没有图标"同值，靠比较而不是靠非空判断）。
          const ops = opsFor(li, { name: n.value.trim() || name, desc: d.value, url: u.value.trim(),
            icon: ic.value === curIcon ? null : ic.value });
          if (await applyOps(ops, 'app.ops.item')) reloadKeepingPlace();
        },
        // 这一层浮在整屏之上，Esc 该先关它、回到 ✎，而不是把底下的浮层一起关掉。
        // 武装态下的 Esc 先**解除武装**，再按一次才关面板——两步确认不能被 Esc 一次跳过。
        keydown: (e) => {
          if (e.key !== 'Escape') return;
          e.preventDefault();
          e.stopPropagation();
          if (armed) {
            armed = false;
            clearTimeout(armTimer);
            del.classList.remove('armed');
            del.textContent = T('app.delete.item');
            undo.hidden = true;
            return;
          }
          closeEdit();
        },
      },
    }, [el('h2', { id: 'lb-edit-h', text: T('app.edit.aria', name || T('app.edit.fallbackName')) }),
        el('p', { class: 'edit-where', text: T('app.edit.where', gname) }),
        el('label', { text: T('app.field.title') }, [n]), el('label', { text: T('app.field.desc') }, [d]), el('label', { text: T('app.field.url') }, [u]),
        el('label', { text: T('app.field.icon') }, [ic]),
        el('p', { class: 'edit-icon-row' }, [lookIcon, iconLibraryButton(ic)]),
        el('div', { class: 'row' }, [
          el('button', { type: 'submit', class: 'fld', text: T('app.generic.save') }),
          el('button', { type: 'button', class: 'fld', text: T('app.generic.cancel'), on: { click: () => closeEdit() } }),
        ]),
        el('div', { class: 'row' }, [del]),
        undo]);
    undo.hidden = true;
    document.body.append(form);
    openEdit = { li, form };
    // 不调这一句，Tab 会一路走进面板背后那张被遮住的页面上去（`syncTrap` 那一
    // 族的判据是 DOM 里最后出现的那一层，这一份是刚 append 的，正好在最上面）。
    syncTrap();
    n.focus();
  }

  // ── 分组级的布局切换（§8.3：tiles 磁贴 / list 单行） ──────────────────────
  //
  // **为什么需要它**：`layout` 从前只能由机器改（ops 的 `groups/<id>/layout`，
  // ops.go 那边校验只收 tiles/list），页面上一个入口都没有——而"哪一组用磁贴、
  // 哪一组用单列"恰恰是**人**的判断：条目多而杂的用磁贴，少而常用的用单列。
  // 导入时那个默认值（书签栏 tiles、其余 list，见 bookimport/convert.go）只是猜的，
  // 猜错了总得有人能改。
  //
  // 它排在 ⑤ 是与就地编辑同源的理由：一次写要快照 + 审计，而改布局不值得为它
  // 单开一条零 JS 通路。零 JS 用户照样看得见分组、点得开条目，只是改不了这一项。
  //
  // 按钮上印的是**当前这一档**，不是"点我会变成什么"——与主题按钮那三态同一个
  // 道理：这一组名字有多长、按钮有多窄，"切换成…"那句话根本放不下。
  for (const sec of document.querySelectorAll('main section[data-gid]')) {
    const h2 = sec.querySelector('h2[data-group]');
    if (!h2) continue;
    const cur = sec.dataset.layout === 'tiles' ? 'tiles' : 'list';
    const other = cur === 'tiles' ? 'list' : 'tiles';
    const name = h2.textContent.trim();
    // 键上从前印的是**当前形态名**（「磁贴」/「单列」），于是分组标题那行读成
    // 「● 应用 磁贴」——一个名词挂在另一个名词后面，既不是句子也不是按钮。
    // 现在印字形（▦ 磁贴 / ☰ 单列），完整那句话进 title 与 aria-label：
    // 看得见的人一眼认出这是"换排法"的开关，读屏与语音的人拿到的是整句。
    const word = T(cur === 'tiles' ? 'app.layout.tiles' : 'app.layout.rows');
    const say = T('app.layout.aria', name, word, T(cur === 'tiles' ? 'app.layout.rows' : 'app.layout.tiles'));
    h2.append(el('button', {
      type: 'button', class: 'layoutbtn',
      text: cur === 'tiles' ? '▦' : '☰',
      title: say,
      'aria-label': say,
      on: {
        click: async (e) => {
          e.preventDefault();
          e.stopPropagation();
          const ok = await applyOps([{ op: 'set', path: `groups/${sec.dataset.gid}/layout`, value: other }], 'app.ops.layout');
          if (ok) reloadKeepingPlace();
        },
      },
    }));
  }

  // ── §8.4 长按多选 → 批量移动 / 打标签 ────────────────────────────────────
  // 拖拽管的是"挪一条"，而**批量管的是"这几条一起挪"**：后者才是"整理"这件事
  // 的真实形状（一次调整一个分类下的一堆链接），一条一条挪等于把这件事做十遍。
  //
  // 用**长按**而不是勾选框：勾选框会占掉每张磁贴的位置，而那点位置在 44px
  // 触摸目标之下已经很紧了；长按不占任何常驻空间。
  const multi = { on: false, picked: new Set() };
  const bar = el('div', { class: 'multibar', hidden: 'hidden', role: 'toolbar', 'aria-label': T('app.multi.aria') });
  body.append(bar);

  function paintMulti() {
    const n = multi.picked.size;
    bar.hidden = !multi.on;
    if (!multi.on) return;
    bar.innerHTML = '';
    const mk = (label, fn) => bar.append(el('button', { type: 'button', class: 'fld', text: label, on: { click: fn } }));
    mk(T('app.multi.move', n), pickMoveTarget);
    mk(T('app.multi.tags', n), pickTags);
    mk(T('app.multi.delete', n), pickDelete);
    mk(T('app.multi.none'), () => { multi.picked.clear(); paintPicked(); });
    mk(T('app.multi.exit'), exitMulti);
  }
  function paintPicked() {
    for (const li of document.querySelectorAll('li[data-item]')) {
      li.classList.toggle('sel-mark', multi.picked.has(li.dataset.item));
    }
    paintMulti();
  }
  function enterMulti(first) {
    multi.on = true;
    if (first) multi.picked.add(first);
    body.dataset.multi = 'on';
    paintPicked();
    showBanner(T('app.multi.hint'));
  }
  function exitMulti() {
    multi.on = false;
    multi.picked.clear();
    delete body.dataset.multi;
    paintPicked();
  }
  function togglePick(id) {
    if (multi.picked.has(id)) multi.picked.delete(id); else multi.picked.add(id);
    paintPicked();
  }

  // 长按用 pointer 事件而不是 mouse/touch 两套：家里三种设备都有（Mac 触控板、
  // Windows 笔记本、iPad），而 touchstart + preventDefault 会顺手把页面的滚动
  // 与选中一起吃掉。
  const HOLD_MS = 450;
  let holdTimer = null;
  for (const li of document.querySelectorAll('li[data-item]')) {
    const id = li.dataset.item;
    li.addEventListener('pointerdown', (e) => {
      if (e.button != null && e.button !== 0) return;
      holdTimer = setTimeout(() => { holdTimer = null; enterMulti(id); }, HOLD_MS);
    });
    const cancel = () => { if (holdTimer) { clearTimeout(holdTimer); holdTimer = null; } };
    li.addEventListener('pointerup', cancel);
    li.addEventListener('pointercancel', cancel);
    li.addEventListener('pointermove', cancel);
    li.addEventListener('click', (e) => {
      if (multi.on) { e.preventDefault(); e.stopPropagation(); togglePick(id); }
      // 不在多选里就让它照常打开链接——这里什么都不做正是要的形状。
    });
  }

  const pickedItems = () =>
    [...document.querySelectorAll('li[data-item]')].filter((li) => multi.picked.has(li.dataset.item));

  async function pickMoveTarget() {
    const picked = pickedItems();
    if (!picked.length) return;
    const groups = [...document.querySelectorAll('section[data-gid]')];
    const dlg = el('div', { class: 'sheet', role: 'dialog', 'aria-modal': 'true', 'aria-label': T('app.move.aria') }, [
      el('h2', { text: T('app.move.title', picked.length) }),
      el('div', { class: 'sheet-actions' }, groups.map((g) => el('button', {
        type: 'button', class: 'fld', text: groupName(g),
        on: {
          click: async () => {
            dlg.remove(); // 从前是 dlg.hidden = true：节点留在 DOM 里，第二次进多选会攒出第二个同名 dialog
            const to = g.dataset.gid;
            const ops = picked.map((li) => ({
              op: 'move',
              path: `groups/${li.closest('section').dataset.gid}/items/${li.dataset.item}`,
              to, index: 0,
            }));
            if (await applyOps(ops, 'app.ops.bulkMove')) reloadKeepingPlace();
          },
        },
      }))),
      el('div', { class: 'sheet-actions' }, [el('button', {
        type: 'button', class: 'fld', text: T('app.move.cancel'), on: { click: () => dlg.remove() },
      })]),
    ]);
    body.append(dlg);
  }

  async function pickTags() {
    const picked = pickedItems();
    if (!picked.length) return;
    const input = el('input', { type: 'text', class: 'fld', placeholder: T('app.tags.ph'), autocomplete: 'off' });
    const dlg = el('div', { class: 'sheet', role: 'dialog', 'aria-modal': 'true', 'aria-label': T('app.tags.aria') }, [
      el('h2', { text: T('app.tags.title', picked.length) }),
      el('label', { text: T('app.tags.label') }, [input]),
      el('div', { class: 'sheet-actions' }, [
        el('button', {
          // 按钮与条上那枚同名「打标签」：从前这里是「打上标签」，同一个动作两套词。
          type: 'button', class: 'fld', text: T('app.tags.label'),
          on: {
            click: async () => {
              const list = input.value.split(/[\s,，]+/).map((t) => t.trim()).filter(Boolean);
              if (!list.length) { showBanner(T('app.tags.empty')); return; }
              dlg.remove();
              const ops = picked.map((li) => ({
                op: 'set',
                path: `groups/${li.closest('section').dataset.gid}/items/${li.dataset.item}/tags`,
                value: list,
              }));
              if (await applyOps(ops, 'app.ops.bulkTags')) reloadKeepingPlace();
            },
          },
        }),
        el('button', { type: 'button', class: 'fld', text: T('app.generic.cancel'), on: { click: () => dlg.remove() } }),
      ]),
    ]);
    body.append(dlg);
    input.focus();
  }

  // 批量删除。这一层对话框**就是第二步**（多选条上那颗是按第一步），
  // 所以它必须把"要删的是哪几条"念出来——只报个数字的确认页回答不了
  // "我到底选了哪几条"，那正是 /revert 那一页早就学到的教训。
  async function pickDelete() {
    const picked = pickedItems();
    if (!picked.length) return;
    const names = picked.map((li) => li.querySelector('b')?.textContent?.trim() || li.dataset.item);
    const SHOWN = 8;
    const list = el('ul', { class: 'del-list' }, names.slice(0, SHOWN).map((x) => el('li', { text: x })));
    // 超过一屏才加那行"另外还有 N 条"。**不能给 el() 传 null**：它内部是
    // `n.append(c)`，而 append 会把非节点参数**转成文字**——传 null 就在页面上
    // 落出一行 "null"（选中 ≤8 条是常态，那一行每次都出现）。
    const more = names.length > SHOWN ? [el('p', { class: 'hint', text: T('app.delete.more', names.length - SHOWN) })] : [];
    const dlg = el('div', { class: 'sheet', role: 'dialog', 'aria-modal': 'true', 'aria-label': T('app.delete.aria') }, [
      el('h2', { text: T('app.delete.bulk', names.length) }),
      list, ...more,
      el('p', { class: 'hint', text: T('app.delete.undoHint') }),
      el('div', { class: 'sheet-actions' }, [
        el('button', {
          type: 'button', class: 'fld danger', text: T('app.delete.confirmMany', names.length),
          on: {
            click: async () => {
              dlg.remove();
              const ops = picked.map((li) => ({
                op: 'remove',
                path: `groups/${li.closest('section').dataset.gid}/items/${li.dataset.item}`,
              }));
              if (await applyOps(ops, 'app.ops.bulkDelete')) reloadKeepingPlace();
            },
          },
        }),
        el('button', { type: 'button', class: 'fld', text: T('app.generic.cancel'), on: { click: () => dlg.remove() } }),
      ]),
    ]);
    body.append(dlg);
    // 焦点落在**取消**上，不落在删除上：走到这一步的人可能只是按错了那颗键，
    // 而默认位置决定了他按 Enter 会发生什么。
    dlg.querySelector('.sheet-actions:last-of-type button:last-of-type').focus();
  }

  // ── ⑤ §7.5 的快捷键清单与「?」帮助 ──────────────────────────────────────
  // 帮助面板**自己就是这份清单的唯一出处**：把它写成静态文案的话，加了键就会
  // 有两处要改，而人只会记得改自己刚写的那一处。
  //
  // 修饰键按平台算好再写进清单，**不要**先生成 ⌘ 版再 replace('⌘','Ctrl')：
  // 那样「⌘/Ctrl K」会变成「Ctrl/Ctrl K」（实测 Windows 上整张清单都这么坏）。
  const mac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);
  const MOD = mac ? T('app.keys.mod') : 'Ctrl';
  const KEYS = [
    [MOD + ' K', T('app.keys.k')],
    [MOD + ' ↵', T('app.keys.enter')],
    ['/', T('app.keys.slash')],
    ['n', T('app.keys.n')],
    ['?', T('app.keys.q')],
    [T('app.keys.longpress'), T('app.keys.long')],
    ['↑ ↓', T('app.keys.arrows')],
    ['Esc', T('app.keys.esc')],
  ];
  const help = el('div', {
    class: 'cmdk', hidden: 'hidden', role: 'dialog', 'aria-modal': 'true', 'aria-label': T('app.keys.aria'),
  }, [
    el('h2', { class: 'cmdk-h', text: T('app.keys.title') }),
    el('dl', { class: 'keys' }, KEYS.flatMap(([k, d]) => [el('dt', { text: k }), el('dd', { text: d })])),
    el('p', { class: 'plain', text: T('app.keys.escAll') }),
  ]);
  body.append(help);

  // ── 键盘（spec §7.5 的两轨：家里混用 Mac 与 Windows）──────────────────────
  // 只绑**输入框之外**的单键；输入框里按 n 是要打字的。
  const mod = (e) => (e.metaKey || e.ctrlKey);

  document.addEventListener('keydown', (e) => {
    const typing = /^(INPUT|SELECT|TEXTAREA)$/.test(document.activeElement?.tagName || '');
    if (mod(e) && (e.key === 'k' || e.key === 'K')) {
      e.preventDefault();
      cmdk.hidden ? cmdkOpen() : cmdkClose();
      return;
    }
    // ⌘/Ctrl ↵：在面板里后台标签打开。它与 Enter 的差别只有一个键，
    // 而那一个键省下来的正是"关掉这一页再开新标签"这一步。
    if (mod(e) && e.key === 'Enter' && !cmdk.hidden) {
      e.preventDefault();
      const hit = cmdkHits[cmdkSel];
      if (hit && hit.url) window.open(hit.url, '_blank', 'noopener');
      return;
    }
    if (e.key === 'Escape') {
      e.preventDefault();
      closeTopLayer();
      if (multi.on) exitMulti();
      return;
    }
    if (typing || mod(e) || e.altKey) return;
    if (e.key === '/') { e.preventDefault(); $('input[type=search]')?.focus(); return; }
    if (e.key === 'n' || e.key === 'N') { e.preventDefault(); openSheet(); return; }
    if (e.key === '?') {
      e.preventDefault();
      help.hidden = !help.hidden;
      if (!help.hidden) cmdkClose();
    }
  });

  // ── ⑥ 离线 ────────────────────────────────────────────────────────────────
  // 静态资产只有这一段 JS。**HTML 不预缓存**：它按档案分桶、走 network-first，
  // 是 storeHTML 在运行时按 X-LB-Profile 落的（§7.1）。图标（/icons/*）是内容
  // 哈希 + 运行时 cache-first，同样不需要进清单。
  //
  // appJSPath 从 DOM 里那个 script 标签取，**不用 document.currentScript**——
  // module script 按规范就没有 currentScript（恒为 null），那条写法看起来能跑，
  // 读到的却是空串，而空串会让预缓存静默什么都不做。
  const appJSPath = document.querySelector('script[src^="/app."]')?.getAttribute('src') || '';

  // 顶栏的提醒**只有一个位置**：从前这里判 `$('#lb-update')` 去重，而本函数创建
  // 的节点压根没有那个 id —— 守卫永远不成立，提醒一条条往上堆，退出多选之后
  // 还挂着（实测两条同时在场，且没有任何关闭出口）。
  function showBanner(text) {
    const old = $('#lb-banner');
    if (old) old.remove();
    const p = el('p', { class: 'notice', id: 'lb-banner', role: 'status' }, [
      el('span', { text }),
      el('button', { type: 'button', class: 'dismiss', text: T('app.sw.dismiss'), 'aria-label': T('app.sw.aria'),
        on: { click: () => p.remove() } }),
    ]);
    header.prepend(p);
  }

  // 注册本身**允许失败**：Service Worker 不可用（http 局域网直连、浏览器关了
  // 开关、老浏览器）时，上面所有功能一点不受影响——离线是加法，不是前提。
  if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('/sw.js').then((reg) => {
      // 清单只能从这里送：指纹路径是服务端的常量，install 那一刻 SW 还不知道。
      // **首次注册时 controller 是 null**（这一页还没被 SW 接管），要等 ready
      // 才拿得到——直接 postMessage 给 controller 会静默丢掉，于是"预缓存
      // 好像没生效"而没人知道。
      const talk = () => (reg.active || navigator.serviceWorker.controller)
        ?.postMessage({ type: 'precache', urls: appJSPath ? [appJSPath] : [] });
      talk();
      navigator.serviceWorker.ready.then(talk);

      // **不静默更新**（§7.1）：浏览器把新 SW 装好后发 controllerchange，
      // 我们只在页面上说一声，让用户自己决定什么时候刷新——一个正在被人编辑着
      // 的页面被后台换掉，比晚一步更新难受得多。
      let said = false;
      navigator.serviceWorker.addEventListener('controllerchange', () => {
        if (said) return;
        said = true;
        showBanner(T('app.sw.updated'));
      });
    }).catch(() => { /* 没有 SW：在线功能一点不受影响 */ });

    // 切档时告诉 SW：清掉上一个档案的 HTML 桶。**两个动作原本是同批做的**
    // （改写"当前档案" + 清旧桶），只改记录会留下旧档案的缓存（下次离线回退
    // 打错人），只清缓存会让离线回退找不到任何东西。
    // 2026-10-10 代管态下线之后 `/switch` 只剩 `to=picker` 一条，于是这里把
    // picker **转成空串**发过去：`sw.js` 那支 `if (d.to)` 不成立，只删桶、不改
    // `currentProfile`。留着那个指针无害——它指向的桶刚被删掉，回退查不到就落到
    // `html-picker`；而下一次有人真的进了一个档案页，`storeHTML` 会把指针改回来。
    // 这里挂在 /switch 的表单提交上：那是全站唯一改 lb_profile 的动作，
    // 别处再埋一份就会漏。
    for (const f of document.querySelectorAll('form[action="/switch"]')) {
      f.addEventListener('submit', () => {
        const to = f.querySelector('input[name=to]')?.value || '';
        navigator.serviceWorker?.controller?.postMessage({
          type: 'switch', from: profile, to: to === 'picker' ? '' : to,
        });
      });
    }
  }
})();