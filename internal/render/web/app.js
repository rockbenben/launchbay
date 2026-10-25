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
    "app.field.newGroup": "新分组名",
    "app.field.icon": "图标",
    "app.ph.title": "网站标题，可改",
    "app.ph.desc": "一句话说明，可不填",
    "app.ph.group": "新分组叫什么",
    "app.icon.none": "不选图标（用名字第一个字）",
    "app.icon.auto": "抓这个网站的图标",
    "app.icon.currentSuffix": "（现在的）",
    "app.icon.fetch": "查这一站有哪些图标",
    "app.icon.lib": "图标库…",
    "app.icon.libTitle": "从图标库里挑一个",
    "app.icon.libPh": "搜名字，比如 docker、github、相册",
    "app.icon.libHint": "常见的牌子都有；也可以按东西搜，比如相机、相册、下载。",
    "app.icon.libStart": "在上面搜一个名字，或者往下翻整个图库。",
    "app.drop.dup": "这一份里已经有这一条了，没有再存一遍。",
    "app.added.saved": "存进「%s」了。",
    "app.drop.noGroup": "这一份还没有分组，拖进来的地址没地方放。先建一个分组，再拖一次。",
    "app.icon.libStarter": "这一份已经在用的",
    "app.icon.libCommon": "常用的",
    "app.icon.libAll": "图库里的全部（%s 个）",
    "app.icon.libLoad": "再加载一批（已显示 %s 个）",
    "app.icon.libNone": "图标库里没有叫「%s」的。",
    "app.icon.libMore": "共 %s 个，先看前 %s 个",
    "app.icon.libFail": "图标库没查到（这一格连不上，稍后再试一次）。",
    "app.save": "保存",
    "app.saveMore": "保存并继续",
    "app.close": "关闭",
    "app.reading": "正在读取那个网页…",
    "app.readFail": "没读到东西，标题可以自己填",
    "app.readOk": "已读到标题、说明和图标",
    "app.readEmpty": "这个网页没给出可读的信息，标题自己填",
    "app.readErr": "读不出来：%s",
    "app.netErr": "没能连上这台机器（网络断了，或者它没开机）。等它回来再试一次。",
    "app.netQueued": "这台机器现在连不上，这条先记在待办里；等它回来会自己补，也可以点待办里的「补进去」。",
    "app.backup.imported": "备份已导入。",
    "app.err.group": "先选一个分组。",
    "app.err.groupName": "先给这个分组起个名字。",
    "app.err.url": "先填地址。",
    "app.offline.queued": "离线了，这条先记下，网络通了自动补上。",
    "app.offline.chip": "待办 %d 条",
    "app.offline.aria": "看待办条目",
    "app.offline.drop": "不要这条",
    "app.offline.list": "下面这些是还没补进去的；网络通了会自动补，没补上就点「补进去」再试一次。",
    "app.offline.retry": "补进去",
    "app.offline.blocked": "现在没网，只能看。添加会先记下来，网络通了自动补进去。",
    "app.offline.done": "待办里的 %d 条已经补进去",
    "app.offline.dropped": "这 %d 条和别人改的撞上了，没自动合并，请重新加一次：%s",
    "app.offline.left": "还有 %d 条没存进去：%s",
    "app.saving": "正在保存…",
    "app.saved": "已保存。",
    "app.conflict": "这一份已经被改过了（可能另一边也动了）。刷新之后再试一次。",
    "app.cmdk.placeholder": "搜书签，或做点什么",
    "app.cmdk.more": "在整页里搜「%s」· 一共 %d 条命中",
    "app.cmdk.aria": "命令面板",
    "app.cmdk.clipboard": "把剪贴板里的网址存进来",
    "app.cmdk.history": "看这一份的改动历史",
    "app.cmdk.multi": "多选书签（一次改好几条）",
    "app.cmdk.kind": "动作",
    "app.cmdk.external": "需手动打开",
    "app.cmdk.none": "没有对得上「%s」的书签，也没有对得上的动作。",
    "app.cmdk.empty": "这里还没有可列的东西。",
    "app.clip.denied": "这个浏览器不给读剪贴板，手动贴一下地址。",
    "app.clip.noturl": "剪贴板里那段不像一个网址，手动贴一下。",
    "app.clip.noread": "读不到剪贴板（系统没给这个权限），手动贴一下地址。",
    "app.copy.aria": "复制「%s」的链接",
    "app.copy.done": "已复制",
    "app.copy.fail": "复制不了，已经帮你把地址选中了，按 Ctrl+C 或 ⌘C",
    "app.edit.aria": "改「%s」",
    "app.edit.fallbackName": "这一条",
    "app.layout.tiles": "磁贴",
  "app.group.new": "＋ 新分组…",
  "app.group.count": "%s（%d 条）",
  "app.group.shape": "这一组的排法",
    "app.layout.rows": "单列",
    "app.layout.feature": "大卡",
    "app.layout.featureTitle": "挪到页面顶部那块放大的区域：卡更大、名字下面是主机名。几个分组都设成「大卡」会并进同一块，这一块会一直往下长",
    "app.layout.aria": "把「%s」这一组从%s改成%s",
    "app.multi.aria": "批量操作",
    "app.multi.move": "移动到（%d）",
    "app.multi.tags": "打标签（%d）",
    "app.multi.delete": "删掉（%d）",
    "app.multi.none": "取消全选",
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
    "app.keys.escAll": "Esc 关掉这个面板",
    "app.sw.dismiss": "知道了",
    "app.sw.aria": "关掉这条提醒",
    "app.sw.updated": "书签页面已更新。刷新一下页面就能用上新的。",
    "app.generic.cancel": "取消",
    "app.generic.save": "保存",
    "app.ops.item": "这一条",
    "app.ops.move": "移动",
    "app.ops.layout": "这一组的排法",
  "app.ops.group": "这一组的改动",
  "app.ops.tabs": "点书签的方式",
  "app.ops.prefs": "这一格设置",
  "app.ops.bg": "背景图",
    "app.ops.bulkMove": "批量移动",
    "app.ops.bulkTags": "批量打标签",
    "app.ops.delete": "删除",
    "app.ops.bulkDelete": "批量删除",
    "app.delete.item": "删掉这条",
    "app.delete.confirm": "确定删掉「%s」？再按一次就删",
    "app.delete.bulk": "删掉这 %d 条？",
    "app.delete.confirmMany": "删掉这 %d 条",
    "app.delete.more": "…另外还有 %d 条没列出来",
    "app.delete.aria": "批量删除",
    "app.delete.undoHint": "删掉之后还能用「设置 → 备份与历史」找回这一版",
    "app.applyOps.fail": "%s没存上：%s",
    "app.opsConflict": "这一份已经被改过了。刷新之后再试一次。",
    "app.opsWrongProfile": "这一页改的不是你正在用的那一份。点右上角的档案名，重新进一次。",
    "app.opsStale": "那一组或那一条已经不在了——可能别人刚改过。刷新之后再看一次。",
    "app.opsDuplicate": "这一条已经在这份里了（同一条书签不能存两次）。要改它，就在那一条上按 ✎。",
    "app.opsIcon": "那个图标名这台机器查不到。换一枚，或者留空，用名字头一个字。",
    "app.resolveBadUrl": "这个地址还不完整——要有协议和主机名（例如 http://10.0.0.5:5000/）。",
    "app.resolveDown": "那个网页没应答（那台机器可能没开）。照样存下来也行，回头补标题。",
    "app.opsNoProfile": "这台机器上没有这一份档案了——多半是刚被删掉。点右上角的档案名重新选一份。",
    "app.opsBroken": "这一份的数据文件读不动，刚才的改动没有写进去。这不是重试一次能解决的——要在这台机器上修好那个文件。",
    "app.opsRejected": "这一批没有写进去：这台机器不收刚才那些内容。（原因在浏览器控制台里。）",
    "app.opsNoSnap": "这一版存不下，改动已经取消，书签一个字都没变。",
    "app.opsServerError": "这台机器刚才没能完成这个请求（HTTP %s）。稍后再试一次。",
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
    "app.field.newGroup": "New group name",
    "app.field.icon": "Icon",
    "app.ph.title": "Page title, editable",
    "app.ph.desc": "One-line note, optional",
    "app.ph.group": "Name for the new group",
    "app.icon.none": "No icon (use the first letter)",
    "app.icon.auto": "Fetch this site's icon",
    "app.icon.currentSuffix": " (current)",
    "app.icon.fetch": "See the icons for this site",
    "app.icon.lib": "Icon library…",
    "app.icon.libTitle": "Pick one from the icon library",
    "app.icon.libPh": "Search by name — docker, github, photos",
    "app.icon.libHint": "Most well-known brands are in here. You can also search for the thing itself: camera, photos, downloads.",
    "app.icon.libStart": "Search for a name above, or scroll through the whole library.",
    "app.drop.dup": "This profile already has that one — it was not saved twice.",
    "app.added.saved": "Saved into %s.",
    "app.drop.noGroup": "This profile has no group yet, so the dropped link has nowhere to go. Make a group first, then drop it again.",
    "app.icon.libStarter": "Already used in this profile",
    "app.icon.libCommon": "Common ones",
    "app.icon.libAll": "All of them in the library (%s)",
    "app.icon.libLoad": "Load more (%s shown so far)",
    "app.icon.libNone": "Nothing in the library is called “%s”.",
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
    "app.netErr": "Couldn't reach this machine (the network is down, or it's off). Try again when it's back.",
    "app.netQueued": "This machine can't be reached, so this one is queued. It will be sent when the machine is back, or press “Send now” in the queue.",
    "app.backup.imported": "Backup imported.",
    "app.err.group": "Pick a group first.",
    "app.err.groupName": "Give the new group a name first.",
    "app.err.url": "Fill in an address first.",
    "app.offline.queued": "You are offline. This one is queued and will go through when the connection is back.",
    "app.offline.chip": "Pending: %d",
    "app.offline.aria": "See pending items",
    "app.offline.drop": "Drop this one",
    "app.offline.list": "These are still pending. They will go through when the connection is back; if not, press “Send now” and try again.",
    "app.offline.retry": "Send now",
    "app.offline.blocked": "No network, so this is read-only for now. Additions are queued and sent automatically when you are back.",
    "app.offline.done": "%d pending items went through",
    "app.offline.dropped": "%d bookmarks clashed with someone else's changes and were not merged automatically. Please add them again: %s",
    "app.offline.left": "%d bookmarks still not saved: %s",
    "app.saving": "Saving…",
    "app.saved": "Saved.",
    "app.conflict": "This profile changed underneath you (something else may have edited it). Refresh and try again.",
    "app.cmdk.placeholder": "Search bookmarks, or do something",
    "app.cmdk.more": "Search the whole page for “%s” · %d matches in total",
    "app.cmdk.aria": "Command palette",
    "app.cmdk.clipboard": "Save the address on the clipboard",
    "app.cmdk.history": "See this profile's change history",
    "app.cmdk.multi": "Multi-select bookmarks (change several at once)",
    "app.cmdk.kind": "action",
    "app.cmdk.external": "Open manually",
    "app.cmdk.none": "Nothing matches “%s” — no bookmark, and no action either.",
    "app.cmdk.empty": "There is nothing to list yet.",
    "app.clip.denied": "This browser will not let the page read the clipboard. Paste the address yourself.",
    "app.clip.noturl": "What is on the clipboard does not look like an address. Paste it yourself.",
    "app.clip.noread": "Could not read the clipboard (the system did not grant permission). Paste the address yourself.",
    "app.copy.aria": "Copy the link for %s",
    "app.copy.done": "Copied",
    "app.copy.fail": "Could not copy it — the address is selected now, press Ctrl+C or ⌘C",
    "app.edit.aria": "Edit “%s”",
    "app.edit.fallbackName": "this bookmark",
    "app.layout.tiles": "Tiles",
  "app.group.new": "+ New group…",
  "app.group.count": "%s (%d)",
  "app.group.shape": "Arrange it as",
    "app.layout.rows": "Single column",
    "app.layout.feature": "Big card",
    "app.layout.featureTitle": "moves into the enlarged area at the top: bigger cards with the host under the name. Groups set to this join the same area, which keeps growing",
    "app.layout.aria": "Switch the group “%s” from %s to %s",
    "app.multi.aria": "Bulk actions",
    "app.multi.move": "Move to (%d)",
    "app.multi.tags": "Tag (%d)",
    "app.multi.delete": "Delete (%d)",
    "app.multi.none": "Clear selection",
    "app.multi.exit": "Exit",
    "app.multi.hint": "In multi-select: click cards to select or deselect them. The row below holds the bulk actions; press Esc to exit.",
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
    "app.sw.updated": "The bookmark page has been updated. Refresh the page to use the new one.",
    "app.generic.cancel": "Cancel",
    "app.generic.save": "Save",
    "app.ops.item": "That bookmark",
    "app.ops.move": "The move",
    "app.ops.layout": "The group's layout switch",
  "app.ops.group": "That group's change",
  "app.ops.tabs": "How bookmarks open",
  "app.ops.prefs": "That setting",
  "app.ops.bg": "The background",
    "app.ops.bulkMove": "The bulk move",
    "app.ops.bulkTags": "The bulk tagging",
    "app.ops.delete": "The deletion",
    "app.ops.bulkDelete": "The bulk deletion",
    "app.delete.item": "Delete this bookmark",
    "app.delete.confirm": "Delete “%s”? Press again to delete it",
    "app.delete.bulk": "Delete these %d bookmarks?",
    "app.delete.confirmMany": "Delete %d bookmarks",
    "app.delete.more": "…and %d more not listed",
    "app.delete.aria": "Bulk delete",
    "app.delete.undoHint": "You can bring this version back from Settings → Backup & history",
    "app.applyOps.fail": "%s didn't go through: %s",
    "app.opsConflict": "This profile changed underneath you. Refresh and try again.",
    "app.opsWrongProfile": "This page isn't the profile you're using. Click the profile name at the top right and come back in.",
    "app.opsStale": "That group or bookmark is gone — someone else may have changed it. Refresh and look again.",
    "app.opsDuplicate": "That one is already in this profile — a bookmark can't be stored twice. To change it, press ✎ on it.",
    "app.opsIcon": "This machine doesn't know that icon name. Pick another, or leave it empty and the first character of the title is used.",
    "app.resolveBadUrl": "That address isn't complete yet — it needs a scheme and a host name (e.g. http://10.0.0.5:5000/).",
    "app.resolveDown": "That page didn't answer (the machine may be off). You can still save it and fill the title in later.",
    "app.opsNoProfile": "This machine no longer has that profile — it was probably just deleted. Pick another from the profile name at the top right.",
    "app.opsBroken": "That profile's data file can't be read, so the change wasn't written. Retrying won't help — the file has to be fixed on this machine.",
    "app.opsRejected": "That batch wasn't written — this machine didn't accept what was in it. (The reason is in the browser console.)",
    "app.opsNoSnap": "This version couldn't be stored, so the change was cancelled — your bookmarks are untouched.",
    "app.opsServerError": "This machine could not finish that request (HTTP %s). Try again in a moment.",
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
  // 备份导入成功会在浮层里重载页面；这一格把成功话带过重载（一次性）。
  const RESTORE_OK = 'lb-restored-' + profile;
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
  // **状态行只有一个，而两个异步动作都会往它上面写**（2026-10-24）：点「保存」
  // 时地址那一格的 blur 会先发一轮 resolve，它与写入并发；谁后回来谁的字留下，
  // 于是慢半步的 resolve 会把「这一条已经在这份里了」盖成「已读到标题」——
  // 人看到一句成功、浮层没关，以为保存没生效。序号让**写入的答案**说了算：
  // submit 一进场就 ++，任何比它早出发的 resolve 回来后一个字都不许写。
  let statusSeq = 0;
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
    // **给了落点就要清掉底部那条**（2026-10-24 修）：原先写的是 `into !== node`，
    // 而 `node` 就是 `into`——条件恒假，于是字段错误写上去之后，上一句
    // 「正在读取那个网页…」仍挂在底部，地址已经报错了屏上还在转。
    if (into) { status.textContent = ''; status.className = 'sheet-status'; }
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
    // 浮起来一层的时候**背景不跟着滚**（batch11 S2）。inert 已经让背景点不动、
    // Tab 进不去，但它管不了滚轮：浮层只盖住视口中间那一块，滚轮落在露出来的
    // 背景上滚的是后面那一页——焦点还在层里，看到的却是别处在动。
    // 用 overflow 而不是 position:fixed：后者要把滚动位置存下来再还原，
    // 而 iOS Safari 上那一次还原本身就是一次抖动。
    document.body.classList.toggle('lb-layer-up', !!top);
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
    //
    // **焦点要还回去**（2026-10-20）：这一层是被某个按钮打开的（挑组是从
    // 「移动到」那颗键、标签是从卡片上那枚笔），关掉它以后焦点原本停在被
    // 摘掉的那个节点上——那个节点已经不在文档里了，于是焦点掉回 <body>，
    // 键盘用户被扔到页面最顶端（他刚按的 Tab 从那儿重新开始）。
    // 同文件里台面面板的关闭与添加浮层的 × 都写了 focus()，这一支原先漏了。
    const dlg = [...document.querySelectorAll('.sheet')].find((x) => x !== sheet && x !== prefsSheet);
    if (dlg) {
      const opener = dlg.__opener || document.activeElement;
      dlg.remove();
      syncTrap();
      if (opener && opener.isConnected) opener.focus();
      return true;
    }
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
    // **不加 primary**（2026-10-17 用户改判：这一颗按普通的来）：它是顶栏里的
    // 一个动作，不是"这一页上你唯一该做的事"——搜索、设置、换档案都在它旁边。
    // 加重之后它比顶栏右侧那三颗都响，而这一页真正的主内容是下面那些书签。
    type: 'button', id: 'lb-add', class: 'add', text: T('app.add'), title: T('app.addTitle'),
    on: { click: openSheet },
  });
  // 插在搜索表单**之后**而不是 header 末尾：DOM 顺序就是视觉顺序，这样「＋ 添加」
  // 跟着标题与搜索框排同一行，不再独占一行拉成一条横带（1440 上实测 980×44）。
  const searchForm = $('header .search');
  header.insertBefore(openBtn, searchForm ? searchForm.nextSibling : header.firstChild);

  // ── 浮层 ──────────────────────────────────────────────────────────────────
  // **地址那一格不加 `required`**（2026-10-22 改）：原生表单校验会**抢在产品
  // 自己的校验前面**——浏览器拦下提交，浮层里一个字都没有，出来的是浏览器自己
  // 的气泡，而那句**跟着浏览器界面语言走，不跟页面的语言走**。
  //
  // 实测（2026-10-22）：页面中文（`lang="zh-CN"`，顶栏「我的书签」）+ 浏览器界面
  // 英文（`navigator.language = en-US`）→ 气泡是 "Please fill out this field."，
  // 而产品自己准备好的那句「先填地址。」（中英各一份）**永远到不了**——下面第五
  // 句那句 `say(T('app.err.url'), 'bad', urlErr)` 被 required 挡在了门外。
  //
  // 去掉它之后，submit() 自己那句说了算：它在**页面语言**里，落在**地址那一格
  // 自己下面**，焦点也回到那一格——与这一族其余的字段错同一形状。
  //
  // **可及性不丢**：required 顺带告诉读屏「这一格必填」，所以补一个 aria-required。
  // **type="url" 留着**：移动端它给的是网址键盘，autofill 也认它。
  //
  // **但 type="url" 自己也会拦提交**（2026-10-24 实测）：敲 "abc" 按「保存」，
  // 浏览器原生气泡「请输入网址。」先冒出来——它跟着浏览器界面语言走，而 submit()
  // 根本没跑，产品那句「这个地址还不完整…」也就到不了。所以下面这张表单带
  // `novalidate`：留下 type=url 的键盘与 autofill，把判据交回 submit() 自己。
  const fUrl = el('input', { type: 'url', id: 'lb-url', class: 'fld', placeholder: 'https://', 'aria-required': 'true', autocomplete: 'off' });
  const fTitle = el('input', { type: 'text', id: 'lb-title', class: 'fld', placeholder: T('app.ph.title'), autocomplete: 'off' });
  const fDesc = el('input', { type: 'text', id: 'lb-desc', class: 'fld', placeholder: T('app.ph.desc'), autocomplete: 'off' });
  const fGroup = el('select', { id: 'lb-group', class: 'fld' });
  // 「新分组名」这一格**也带上限**，与设置面板那两个框同一个数
  // （model.MaxGroupTitleRunes，页面把它印在 <body data-max-group-title> 上）。
  //
  // **为什么这一格原本没有**：面板里那两格是服务端渲染的，maxlength 顺手就印上了，
  // 而这一格是脚本造的，忘了这一件事——于是同一个动作在两个入口给出两个答案：
  // 面板里 60 字以上打不进去，浮层里打多少都收。实测：浮层里存进去一个 200 字的
  // 分组名，回到设置面板那一格（maxlength=60，装不下）**只显示得下一部分**，
  // 而那个名字还印在每个节标题、分组导航与导出文件里。
  //
  // **数从 <body> 上读，不在这里写死**：app.js 是静态资产，读不到服务端的 catalog；
  // 页面把那一个数递过来，两处于是永远是同一个（model 那一份是唯一来源）。
  const maxGroupTitle = Number(body.dataset.maxGroupTitle) || undefined;
  const fNewGroup = el('input', { type: 'text', id: 'lb-newgroup', class: 'fld', placeholder: T('app.ph.group'), autocomplete: 'off', maxlength: maxGroupTitle });
  // 「新分组名」这一格的**标签**要能跟着输入框一起藏（2026-10-21 修）——
  // 见 syncGroupFields 那段：只藏输入框的话，屏上会留下一行没有框的标签。
  const fNewGroupLabel = el('label', { text: T('app.field.newGroup') }, [fNewGroup]);
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
  // **这一组是「这一组用哪种排法」**（2026-10-20）：从前它是一个 <label> 包着
  // 两颗 button，而 label 的隐式目标是**第一颗 button**，它自己又没有 for——
  // 于是读屏只念「磁贴 按钮 已按下」，听不出这是关于排法的；点标签文字还会
  // 顺带触发第一颗。台面面板那一处已经是 role=radiogroup（profile.tmpl），
  // 这里照它那一套：div[role=group] + 组名。
  const fGroupShapeWrap = el('div', { role: 'group', 'aria-label': T('app.group.shape') }, [
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
  //
  // **标签与输入框一起藏**（2026-10-21 修）：从前只藏了输入框，标签那一行留在
  // 屏上——于是「＋ 添加」浮层里浮着一行没有框的「新分组名」，而它下面是一个
  // 图标下拉框。看上去像忘了删的半截表单。
  // 藏标签这一件事 CSS 也要配合：`.sheet label:has(…)` 是作者规则，权重压过
  // 浏览器默认的 `[hidden]{display:none}`，于是光写 `hidden` 是空话——
  // 那条规则改成 `:not([hidden])` 才让 hidden 说了算。
  function syncGroupFields() {
    const creating = fGroup.value === NEW_GROUP;
    fNewGroup.hidden = !creating;
    fNewGroupLabel.hidden = !creating;
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
  // 「保存」是这一层唯一会落库的那一颗（「保存并继续」也落，但它说的是"接着再来一条"）——
  // 从前两颗都是 .fld 那张脸，实测并排等宽等色，主操作只能靠读字认（batch11 P8）。
  const saveBtn = el('button', { type: 'submit', class: 'fld primary', text: T('app.save') });
  const moreBtn = el('button', { type: 'button', class: 'fld more', text: T('app.saveMore'), on: { click: () => submit(true) } });

  // **`class: 'sheet-form'` 不是装饰**（2026-10-21 修）：这一层要按**列**排，
  // 字段与字段之间留 10px 缝（见 app.css 那条 `.sheet-form`）。
  // 从前那条规则写的是 `.sheet form`——**它把台面面板里的每一行也算进去了**：
  // 面板每一行也是 `<form class="prefs-row">`，于是它们全被改成列向 flex，
  // 而 `.prefs-field` 的 `flex:1 1 320px` 本来是**宽度**基准（横排时一格至少那么宽），
  // 一翻成竖排就成了**高度**基准——面板每一行于是最少 320px 高，内容只有 ~110px，
  // 一屏里大半是空的（实测：八行 2787px，每行 350px）。
  // **只给这一层自己的表单一个类**，别的 form（面板那些行、上传那一枚）都不碰。
  const form = el('form', {
    class: 'sheet-form',
    novalidate: '',
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
    // **新分组名那一格有自己的 label**（2026-10-20）：它从前跟分组下拉共用一个
    // <label>，于是「新分组名」输入框的可及名也是「分组」——两个控件同名，
    // 读屏分不出哪个是哪个。
    el('label', { text: T('app.field.group') }, [fGroup]),
    fNewGroupLabel,
    fGroupShapeWrap,
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
  // 为什么 fetch 一次而不是把标记内嵌进档案页：面板标记**随分组数变大**（未压缩：
  // 一份只有一组的档案实测约 12 KB，其中分组清单是最大的一块），而 §7.3 那条操作线
  // 管的是**每一页**的首屏——不打开面板的绝大多数人（只是来看书签）不该背这份标记。
  // 代价是打开时多一次同源请求；它是本地请求，面板自己不写状态，**失败就退回一次
  // 普通跳转**（一次网络抖动不该把人堵在"浮层开不出来"上）。
  const prefsSheet = $('#lb-prefs');
  let prefsLoaded = false;

  // 面板里**选文件那一族**的文件名回显（2026-10-14 起是背景那一格，
  // 2026-10-21 提成这一族：导入一份备份也是选文件）。
  // 原生 `<input type=file>` 被 CSS 收起来之后（见 base.css .prefs-file：它是浏览器
  // 自己画的按钮，color-scheme 管不到，暗色页上一块浅灰像别的产品），
  // "选中了哪个文件"就没有别的地方能显示了。
  //
  // **只在这一条路上做**：档案页的面板是 JS 注入的，所以能绑；`GET /prefs` 那张
  // 零 JS 的独立页没人绑，于是它看不见文件名（表单照样能提交，提交后台面重画）。
  // 绑定用 `on: {change: …}` 与这一份脚本里其余事件同一处置，且**只绑一次**——
  // 面板是注入一次就长期在 DOM 里（prefsLoaded 那一闩），重复绑会挂出多个监听。
  //
  // **绑的是这一族里每一个文件选择器**，不是背景那一颗：两个出口各写一遍
  // 就是两处会各自漂的判据（背景那一颗改了形状，导入那一颗还按老样子找）。
  function wireFileNames(root) {
    for (const input of root.querySelectorAll('.prefs-file input[type=file]')) {
      // 文件名那一行是**同一张表单里的兄弟**（不在 label 里面）——label 里只有
      // 那颗 chip 与真正被盖住的 input。
      const out = input.closest('form')?.querySelector('.prefs-file-picked');
      if (!out || input.dataset.lbWired) continue;
      input.dataset.lbWired = '1';
      input.addEventListener('change', () => {
        // **只取最后一段**：完整路径在这台机器上可能是 C:\Users\…\某张图.png，
        // 一行放不下而被省略号吃掉，露出来的恰好是用户最不需要看的那部分。
        out.textContent = input.files && input.files[0] ? input.files[0].name : '';
      });
    }
  }

  async function prefsOpen() {
    if (!prefsSheet) return;
    if (!prefsLoaded) {
      try {
        // **带上记住的那一节**：面板是服务端渲染的，而"停在哪"活在 `?tab=` 上
        // （不带走它，浮层每次重开都在第一类）。没记住就不带，让服务端落回第一颗。
        const res = await fetch('/prefs' + (prefsTab ? '?tab=' + encodeURIComponent(prefsTab) : ''),
          { headers: { Accept: 'text/html' } });
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const doc = new DOMParser().parseFromString(await res.text(), 'text/html');
        const panel = doc.querySelector('section.prefs');
        if (!panel) throw new Error('no panel in that page');
        // **摘掉「回我的页面」那枚出口**（2026-10-21）：面板的最后一行里，左边是
        // 出口、右边是源码入口，两者**同一行**（用户要的"平行"）。而浮层这一面
        // 就浮在**你自己的书签页**上——出口是 ×，再来一枚「回我的页面」是
        // 「点了没反应的东西不进界面」那一族里最不该留的一颗。
        // 摘掉之后那一行只剩源码入口，它靠右（`margin-left:auto`），落点不变。
        panel.querySelector('.prefs-back')?.remove();
        prefsSheet.append(panel, el('button', {
          type: 'button', class: 'fld close', text: '×', 'aria-label': T('app.close'),
          on: { click: prefsClose },
        }));
        wirePrefsPanel(prefsSheet);
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

  // wirePrefsPanel 重连那几根**按内容找元素**的线：选文件那一族的文件名回显、
  // 分组名字的回车提交、tab 那颗的 change 监听。
  //
  // 最后一根记着"停在哪一节"——**不是为了这次**（DOM 自己就保持着），是为了下一次
  // 重载/重开。`change` 只在真的换了那颗时响（点已选中的那颗不响）。
  function wirePrefsPanel(root) {
    wireFileNames(root);
    wireGroupRename(root);
    for (const r of root.querySelectorAll('.prefs-tab')) {
      r.addEventListener('change', () => rememberPrefsTab(r.id));
    }
  }

  // replacePrefsPanel 把浮层里的面板整块换成服务端刚画的那一份，并把它顶上那句
  // 提示一起搬过来。
  //
  // 为什么需要它：「我的 PIN」那一族改完之后**形状变了**（多一格「现在的 PIN」
  // / 少一格），所以不能只改一格的状态。
  //
  // **那句话在面板外面**：服务端把它插在 `<section class="prefs">` 之前
  // （renderPrefsNotice），于是只换面板会把它落下——"错话说过了而屏上没这句话"。
  //
  // **提交委托是挂在浮层壳上的**（不是挂在表单上），所以换掉面板里的内容不会把
  // 那些委托摘掉；要重连的是 wirePrefsPanel 那三根。
  function replacePrefsPanel(panel, notice) {
    const old = prefsSheet.querySelector('section.prefs');
    if (old) old.replaceWith(panel);
    // 面板顶上原来那一句是**上一次**留下的（上一轮错话），先摘掉再放这一次的。
    for (const n of prefsSheet.querySelectorAll('.notice')) n.remove();
    if (notice) panel.before(notice);
    wirePrefsPanel(prefsSheet);
    // 焦点别丢：换掉整块面板之后焦点会掉回 <body>，而人下一步要敲的正是
    // 那一格（改错了要改「现在的」，改对了要接着填「新的」）。
    const first = panel.querySelector('input[type=password]');
    if (first) first.focus();
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
  //
  // **「＋ 新分组」那一张是原生提交的**（2026-10-15）：它没有 `input[name=group]`
  // （新组还没有 id，id 由服务端派生），所以这一格不归 `groupOpFromForm` 管。
  // 少了下面那一行 early-return，它会被 preventDefault 掉然后 groupOpFromForm
  // 返回 null ——症状是"点「建」**按了没反应**"，而零 JS 的独立页照样建得出来，
  // 两边分家。建组是低频动作，多一次整页跳转换"两条路行为一样"值。
  // **`^=` 而不是 `=`（2026-10-15）**：分组那一族的 action 带着 `?tab=<当前
  // 那一节>`（不带着它，保存完就跳回第一类），而 `form[action="/prefs/group"]`
  // 匹配不到带查询串的。`^="/prefs/group"` 只多认这一族，认不到 `/prefs/tabs`
  // （它更长且第三段不同），所以下面那三份委托各认各的。
  if (prefsSheet) {
    prefsSheet.addEventListener('submit', async (e) => {
      const form = e.target.closest('form[action^="/prefs/group"]');
      if (!form) return;
      if (!form.querySelector('input[name=group]')) return; // 「＋ 新分组」：原生提交
      e.preventDefault();
      // 一次只发一条 op：表单里颜色与排法两组键并排，靠"只提交被按下的那个
      // 控件"分开，所以这里读 submitter 就能知道人按的是哪一颗——
      // 按了没有 name 的那颗「用这个颜色」时 submitter 是那颗键，取它旁边的
      // 取色器。
      const btn = e.submitter;
      const data = new FormData(form, btn || undefined);
      const op = groupOpFromForm(form, data);
      if (!op) return;
      const ok = await applyOps([op], 'app.ops.group');
      if (ok) {
        // 重载之后把面板重新拉起来（见 OPEN_KEY 那段注释）。只在这一族记，
        // 因为只有它会"改完还想接着改下一组"。
        try { sessionStorage.setItem(OPEN_KEY, '1'); } catch { /* 存不下就当没记 */ }
        reloadKeepingPlace();
      }
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
      await applyOps([{ op: 'set', path: 'openInNewTab', value: v === '1' }], 'app.ops.tabs');
      // **不整页刷新**：这一格只改一个布尔，而整页重载会把面板整个重建
      // （prefsLoaded 那一闩只在首次 fetch 后才为真，重载后要重新拉一次），
      // 于是「按一下 → 面板消失」与「整页跳走」是同一种丢失。
      // 改完就地按一下那颗键的 aria-pressed：那是这一格唯一的可见状态。
      markPressed(form, v);
    });
    // 背景那一族（POST /prefs/background，2026-10-20 补上）。
    //
    // **它从前是原生提交的**，于是浮层里按一下「暂时不显示」，整页导航到独立
    // 那一页（实拍复现）：浮层没了、滚动位置没了、还落回"这一页"而不是你刚才
    // 所在的那一节。同一屏里其余每一行都是就地更新——这一族是唯一的例外。
    //
    // 文件那一格是 multipart，所以不能像别的族那样交 URLSearchParams：直接
    // 把 FormData 交出去（边界由浏览器带）。
    //
    // 改完**要重载**：背景的预览图是按内容哈希的 URL，改了就得重新取一张。
    // 但要记住"面板还开着"（OPEN_KEY），否则重载后面板整个不见了——那正是
    // 这一族此前那个症状的另一半。
    prefsSheet.addEventListener('submit', async (e) => {
      const form = e.target.closest('form[action="/prefs/background"]');
      if (!form) return;
      e.preventDefault();
      const data = new FormData(form, e.submitter || undefined);
      try {
        await fetch('/prefs/background', { method: 'POST', body: data, credentials: 'same-origin' });
      } catch (err) {
        say(T('app.applyOps.fail', T('app.ops.prefs'), String(err.message || err)));
        return;
      }
      try { sessionStorage.setItem(OPEN_KEY, '1'); } catch { /* 存不下就当没记 */ }
      reloadKeepingPlace();
    });
    // 「我的 PIN」那一格（POST /prefs/pin，2026-10-20 补上）。
    //
    // **它从前是原生提交的**，而服务端答 303 回 `/prefs?tab=pin`——于是在浮层里
    // 按一下「改这道门」，整页导航到独立页（实拍复现）：浮层没了、你刚待的那一
    // 节没了，症状读作「设置变成单独页面了」。**与 2026-10-14 给「点开书签」那
    // 一格修过的是同一件事**（那一族的注释写得很清楚），这一族是漏了同一份委托。
    //
    // **这一族特殊在形状会变**：设上第一个 PIN 之后面板要多画一格「现在的 PIN」，
    // 撤掉之后又要少一格——所以改完**不能只动一格**，得把面板整块换掉。
    //
    // 换的那一块**由服务端画**，而两种答法**形状一样**：成功时是 303 跟到
    // `/prefs?tab=pin`（新形状），失败时是 200 且面板顶上带着那句提示
    // （`renderPrefsNotice` 重画的就是它）。所以这里不必分成功与失败——从答里
    // 取 `section.prefs` 换进去，成功看到的是新形状，失败看到的是新形状加那句话。
    //
    // **体是 urlencoded**：服务端那条落点收的是 `application/x-www-form-urlencoded`
    //（`r.ParseForm`），而 FormData 是 multipart——所以这里转成 URLSearchParams
    // 再交（同 `/prefs` 那一族那样）。
    prefsSheet.addEventListener('submit', async (e) => {
      const form = e.target.closest('form[action^="/prefs/pin"]');
      if (!form) return;
      e.preventDefault();
      const data = new FormData(form, e.submitter || undefined);
      const body = new URLSearchParams();
      for (const [k, v] of data.entries()) body.append(k, String(v));
      let html;
      try {
        // **地址取表单自己的 action**：它带着 `?tab=pin`（这一格只出现在「我的
        // PIN」那一节），而服务端答错时把面板连同那句话一起重画、读的就是它。
        // 写死成 `/prefs/pin` 的话，错话会落回第一颗 tab（屏上是「这一页」那一节
        // 顶上一句「现在的 PIN 不对」，而人刚待的地方没了）。
        //
        // **Accept: text/html 是必须的**：答对了是 303，跟过去的那次 GET 若带
        // `*/*`，Service Worker 会把它按**指纹资源**缓存住（那一族 immutable、
        // 永不过期）——于是面板被永久钉住，"改完看到的还是上一次的"。sw.js 里
        // 那条注释说的正是这个坑（它当年是面板第一次取到的那一份被钉住）。
        const res = await fetch(form.getAttribute('action'), {
          method: 'POST', body, headers: { Accept: 'text/html' },
          credentials: 'same-origin', redirect: 'follow',
        });
        if (!res.ok) throw new Error('HTTP ' + res.status);
        html = await res.text();
      } catch (err) {
        showBanner(T('app.applyOps.fail', T('app.ops.prefs'), String(err.message || err)));
        return;
      }
      const doc = new DOMParser().parseFromString(html, 'text/html');
      const panel = doc.querySelector('section.prefs');
      if (!panel) {
        showBanner(T('app.applyOps.fail', T('app.ops.prefs'), 'no panel'));
        return;
      }
      // 那句话在 `<main>` 里、面板**之前**（renderPrefsNotice 插在那儿），
      // 外面套着一层 `.prefs-notice`（独立页上用来与居中面板对齐）——
      // 取**里面那个 `.notice`**，搬进浮层后仍是面板前的一条提示。
      const notice = doc.querySelector('.prefs-notice .notice');
      replacePrefsPanel(panel, notice);
    });
    // 导入一份备份那一格（POST /import/backup，2026-10-21 补上）。
    //
    // **它从前是原生提交的**，而服务端答 303 回 `/prefs?tab=look&ok=imported`——于是在浮层里
    // 按一下「导入」，整页导航到独立页（症状读作「设置变成单独页面了」）。
    // 与 2026-10-14 给「点开书签」、2026-10-20 给「我的 PIN」修过的是同一件事，
    // 这一族是漏了同一份委托。
    //
    // **体是 multipart**：服务端那条落点收 multipart（文件字段），所以 FormData
    // 原样交出去就行（与背景那一族同一处置）。
    //
    // **答有两种，形状不一样了**（2026-10-24）：成功时 303 跟到
    // `/prefs?tab=look&ok=imported`，面板顶上是 `.notice.ok`（蓝色）——导入的
    // 设置要真正画到后面那一页上，所以这时**重载**，由一次性横幅补一句；失败时
    // 200，面板顶上带普通 `.notice`（橙）——把面板换回来、留在原地。
    prefsSheet.addEventListener('submit', async (e) => {
      // `^=` 而不是 `=`：这张表单的 action 带着 `?tab=out`（它只出现在「导出、
      // 导入与历史」那一节），写死成 `=` 匹配不到它。
      const form = e.target.closest('form[action^="/import/backup"]');
      if (!form) return;
      e.preventDefault();
      const data = new FormData(form, e.submitter || undefined);
      let html;
      try {
        // **Accept: text/html 是必须的**：成功时那一发 303 跟过去的是面板那一页，
        // 它若带 `*/*`，Service Worker 会把它按**指纹资源**缓存住（那一族
        // immutable、永不过期）——于是面板被永久钉住，"导完看到的还是上一次的"。
        const res = await fetch(form.getAttribute('action'), {
          method: 'POST', body: data, headers: { Accept: 'text/html' },
          credentials: 'same-origin', redirect: 'follow',
        });
        if (!res.ok) throw new Error('HTTP ' + res.status);
        html = await res.text();
      } catch (err) {
        showBanner(T('app.applyOps.fail', T('app.ops.prefs'), String(err.message || err)));
        return;
      }
      const doc = new DOMParser().parseFromString(html, 'text/html');
      const panel = doc.querySelector('section.prefs');
      if (!panel) {
        showBanner(T('app.applyOps.fail', T('app.ops.prefs'), 'no panel'));
        return;
      }
      const notice = doc.querySelector('.prefs-notice .notice');
      // **成功必须重载**（2026-10-24）：导入的设置里有 theme/layout/view/页面名/
      // 语言，只换面板不会把它们画到后面的页面上（实拍：面板里「亮色」已按下，
      // 页面仍旧是暗的）。重载后由 app.backup.imported 横幅补一句成功话。
      if (notice && notice.classList.contains('ok')) {
        try { sessionStorage.setItem(RESTORE_OK, '1'); } catch { /* 存不下就不说 */ }
        location.reload();
        return;
      }
      replacePrefsPanel(panel, notice);
    });
    // 页面名字 / 排列方式 / 第二行 / 分组列表的位置 / 主题 / 界面语言那六张（POST /prefs；
    // 2026-10-14 补上前几张，2026-10-19 加上第二行，2026-10-20 加上界面语言）。
    //
    // **它们不能走 applyOps**：那六格改的是 `profile.json`（台面偏好），不产生
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
      // **只交这一张表单要改的那一件事**：字段名**从表单自己读**——那张表单里
      // **唯一**那个名字就是它要改的那一格（四张单选钮表单里是那几颗键的共同
      // 名字，「改名」那一行里是那个输入框）。
      //
      // **为什么不再抄一份名单**（2026-10-19 改判）：抄一份就会漏，而漏掉的那一格
      // 在浮层里**按了等于没按**——preventDefault 已经把原生提交取消了，而 payload
      // 是空的，函数直接 return，一个请求都不发：
      //   - 第二行（addrLevel，2026-10-19 加的）没进那份名单，于是面板里按
      //     「完整地址」什么都不会发生；
      //   - 「改名」那一行**一直**是坏的：它的保存键**没有 name**（它说的是
      //     「保存」，不是"改成哪一档"），于是名单里那个 title 永远匹配不上。
      // 两格都只在浮层里坏：零 JS 的独立页走原生提交，在那里它们是好的——同一个
      // 设置，两条路两种结果，而坏的那条正是人最常用的那条。
      //
      // **唯一性由面板那条测试钉住**（每张 POST /prefs 表单恰好一个名字），
      // 所以这一格不猜：名字不唯一就不发（宁可"按了没反应"，也不猜错一格改掉
      // 另一格）。
      const names = [...new Set([...form.querySelectorAll('[name]')]
        .map((el) => el.name).filter(Boolean))];
      if (names.length !== 1) return;
      const k = names[0];
      const v = data.get(k);
      if (v === null) return;
      const payload = { [k]: String(v) };
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
  // 它必须**认名字而不是认顺序**：色板、排法、取色器与「删掉这一组」在同一张
  // 表单里，而命名过的输入框每一次提交都跟着走。判据与 `httpapi/prefs.go`
  // 那一处是同一份——`remove` 在场 = 只删这一组，`apply_custom` 在场 = 取色器
  // 说话，否则 `accent` 说话；两个都不在 = 只改排法。
  function groupOpFromForm(form, data) {
    const gid = form.querySelector('input[name=group]')?.value;
    if (!gid) return null;
    // 删掉这一组：**第一句就问它**。那张表单里名字输入框与取色器都常驻，
    // 而人按的是那颗红键——先读颜色/排法的话，那两格的值会跟着这一次提交走，
    // 于是"删掉这一组"会顺手把颜色也改了。判据与 `httpapi/prefs.go` 同一份：
    // remove 在场 = 只有删除这一件事。
    if (data.has('remove')) return { op: 'remove', path: `groups/${gid}` };
    const layout = data.get('layout');
    if (typeof layout === 'string' && layout) {
      return { op: 'set', path: `groups/${gid}/layout`, value: layout };
    }
    const accent = data.has('apply_custom') ? data.get('custom') : data.get('accent');
    if (typeof accent === 'string' && (accent === '' || /^#[0-9a-fA-F]{3}([0-9a-fA-F]{3})?$/.test(accent))) {
      return { op: 'set', path: `groups/${gid}/accent`, value: accent };
    }
    const box = form.querySelector('input[name=rename]');
    if (box && box.value !== box.defaultValue) {
      const t = box.value.trim();
      // 空着交给服务端去 422 并点名 rename：这里自己吞掉的话，人只看到
      // "按了没反应"，而那句 422 会说清是名字那一格没填。
      if (t) return { op: 'set', path: `groups/${gid}/title`, value: t };
      return null;
    }
    return null;
  }

  // 分组名那一格只挂两件事：**回车提交、Esc 撤回**。
  //
  // **为什么不在失焦时提交**（2026-10-15 实拍后改判）："点一颗色块"这个动作是
  // mousedown 先让输入框失焦、mouseup 再按那颗键——于是人只点了**一下**，屏幕上
  // 却有两个提交（改名 + 颜色），而它们带的是同一个 expectedRev：并行发出去第二个
  // 必然 409，那句 409 说的是"别人改过了"。要串行化就得给这一族加一条请求队列，
  // 而它要解决的是一个自己造出来的问题。
  //
  // 于是提交时机只有回车——和这一节自己那句「名字改完按回车」一致。没提交的那半句
  // 在按色块之后会随重载消失：**这是明说的**（hint 里写着），不是静默丢弃。
  function wireGroupRename(root) {
    for (const box of root.querySelectorAll('input[name=rename]')) {
      if (box.dataset.lbWired) continue;
      box.dataset.lbWired = '1';
      box.addEventListener('keydown', (e) => {
        if (e.key !== 'Escape') return;
        e.preventDefault();
        box.value = box.defaultValue;
        box.blur();
      });
    }
  }

  // 分组下拉的选项来自**服务端已经渲染在页面上的分组**。不重新拉一次 /api/v1/data：
  // 一次浮层开合不该为了拿一份没变的数据多发一个请求，而它还会带来"两处分组列表"
// 迟早不同步的那一类 bug。
  for (const h2 of document.querySelectorAll('main h2[data-group]')) {
    // **读 groupName 而不是 h2.textContent**（2026-10-15 实拍）：大卡那一档的组标题
    // 后面挂着一枚条目数徽标（".feat-n"），而 textContent 把它一起读进来——
    // 于是下拉里第一项是「常用7」，别的组（正文标题没有徽标）是干净的。
    // groupName 只取标题那一份文字，它是同一族几处（拖拽时的 aria-label、删除确认
    // 句里那一句）已经在用的判据。
    fGroup.append(el('option', { value: h2.dataset.group, text: groupLabel(h2.dataset.group, groupName(h2)) }));
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
  // 内嵌索引有一万多个名字（品牌层 simple-icons + 概念层 tabler），而两个浮层原本
  // 只给三样：不选图标 / 自动按网址抓 / resolve 给的至多 5 条候选。库里那一万个
  // 于是**在界面上一个出口都没有**——ops 认 icon 字段、CLI 能查名字，而"给一条书签
  // 换个图标"只能去打接口。
  //
  // **图库不进 app.js、也不进首屏**：按搜索词向 `/api/v1/icons/tiles` 要一小格
  // **服务端画好的 HTML**（§7.3 那条操作线管的就是"只看首屏的每一页不该背一份图库"）。
  // 那一格用的是页面画卡片图标时同一个 svgMarkup，于是挑的时候看见的形状与存下去
  // 之后卡片上那一枚逐像素相同。
  const iconTilesLimit = 24;
  // 图库翻页的页大小（2026-10-20）。60 是一次三百来 KB 的 markup——局域网上一瞬
  // 就到，而一次倒一万个是十几 MB（面板会卡死、家用机器的内存也吃不住）。
  const iconBrowsePageSize = 60;
  const iconPick = {
    sel: null, panel: null, input: null, grid: null, note: null, timer: 0, seq: 0, opener: null,
    // 浏览视图的三段（2026-10-20）：这一份已经在用的 / 常用的 / 图库里的全部。
    used: [],        // 这一份已经在用的（克隆卡片上那一枚）
    common: [],     // 服务端那份人工挑的常用清单
    browse: [],     // 已经翻出来的图库页（累积）
    libLabel: null, // 图库那一段的标题（与第一页一起出现）
    browseTotal: 0, // 全库条数
    browseOffset: 0,
    loading: false, // 正在翻页（别重复要）
    mode: 'browse', // 'browse'（浏览视图） | 'search'（搜索结果）
  };

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
    // 滚到底就再要一页（浏览视图）。搜索视图下不翻——那一屏是"你搜到的那些"。
    grid.addEventListener('scroll', () => {
      if (iconPick.mode !== 'browse' || !iconPick.grid) return;
      const g = iconPick.grid;
      if (g.scrollTop + g.clientHeight >= g.scrollHeight - 80) loadIconBrowse();
    });
    iconPick.used = starterIconTiles();
    paintIconStarter();
    loadIconCommon();
    loadIconBrowse();
  }

  // 常用那一批（2026-10-20）：**服务端给的一份人工挑的常用清单**
  // （`GET /api/v1/icons/starter`），与服务端画卡片图标走同一个出口。
  //
  // **为什么要有它**：面板从前打开是一张空网格 + 一句「先搜个名字」。起始格
  // （这一份已经在用的那些）救不了它——书签默认 icon=auto（去抓 favicon），
  // 而 auto 与"没设"都被那一格排除掉，于是实测里**这一格几乎永远是空的**，
  // 面板也就永远空着。那是 2026-10-16 那次想治的同一个症状，而它自己写着
  // "没回答点这颗按钮的人真正想问的'给我看几个'"。
  //
  // 换判据不如换一个**一定有货**的来源：库里一万多个名字不会全倒出来（那是
  // 一屏噪声），但一份常用清单永远是有的。
  //
  // 走 fetch 而不是让服务端把这一批内联进页面：图库不进首屏（§7.3 那条操作线），
  // 而这一批只在**有人点开这个浮层**时才需要。
  async function loadIconCommon() {
    try {
      const res = await fetch('/api/v1/icons/starter', { headers: { Accept: 'text/html' } });
      if (!res.ok) throw new Error('HTTP ' + res.status);
      const html = await res.text();
      // 关掉了就别写了：那一层已经从文档里摘掉，往一个不在的格子里画等于
      // 往虚空里画（下面的 paintIconStarter 自己也会因为 grid 为空而退出）。
      if (!iconPick.panel) return;
      // 插入走 DOMParser 而不是 innerHTML：那一段是本服务自己画的（路径数据来自
      // 内嵌索引、属性值在 svgMarkup 里逐个转义），但它终究跨了一次 fetch，
      // 而 parse 出来再取节点顺带把 script 与内联事件一起丢掉——"它是我们自己
      // 画的"是注释，parse 是机制。
      const doc = new DOMParser().parseFromString(html, 'text/html');
      iconPick.common = [...doc.body.querySelectorAll('.icpick-tile')];
      paintIconStarter();
    } catch (err) {
      if (!iconPick.panel) return;
      const { note } = iconPick;
      note.textContent = T('app.icon.libFail');
      note.className = 'sheet-status bad';
    }
  }

  // 起始格：**这一份已经在用的那些图标**（2026-10-16）。
  //
  // 此前打开就是一张空网格 + 一句"先搜个名字"（app.icon.libStart）。那是
  // 2026-10-14 定下的空态，判据本身没错——库有一万多条，空查询倒出来是一屏
  // 噪声——可它没有回答点这颗按钮的人真正想问的"给我看几个"，症状是实拍到的
  // 那一屏：面板开了、搜索框在、**什么也没有**。
  //
  // 为什么是"这一份已用的"而不是"随便搜个默认词"：
  //   · 同一服务再存一条是最常见的动作，那颗图标多半就在这一份里；
  //   · 它不是随便挑的，是用户自己刚用过的那些——没有比这更不任意的一组；
  //   · 零请求：图标名在 `li[data-icon-name]` 上，图形在卡片里已经画好了，
  //     克隆一份即可，于是起始格与卡片上那一枚**逐像素同源**（同一份
  //     svgMarkup 的输出）。走 `/api/v1/icons/tiles` 反而要 N 次请求，
  //     而且那条路的空查询按契约就该回空图（AGENTS.md「空查询回一张空图」）。
  // `auto` 不进这一组：它是"去抓 favicon"，不是一枚可挑的图标，而这一格
  // 挑的就是图标名。一个都没有时（空档案或全用 auto）才回到 libStart 那句。
  function starterIconTiles() {
    const out = [];
    const seen = new Set();
    for (const li of document.querySelectorAll('li[data-icon-name]')) {
      const name = li.dataset.iconName;
      if (!name || name === 'auto' || seen.has(name)) continue;
      const face = li.querySelector('.ic-svg,.ic-img,.ic');
      if (!face) continue;
      seen.add(name);
      out.push(el('button', {
        type: 'button', class: 'fld icpick-tile', 'data-icon': name,
        title: name, 'aria-label': iconLabel(name),
      }, [
        el('span', { class: 'icpick-face' }, [face.cloneNode(true)]),
        el('span', { class: 'icpick-name', text: iconLabel(name) }),
      ]));
      if (out.length >= iconTilesLimit) break;
    }
    return out;
  }

  // 翻图库（2026-10-20）：**整个**图标库，不只是常用那一批。
  //
  // 面板从前只有两批来源——"搜一个词"与"这一份已经在用的"——于是"这台机器里到底
  // 有哪些图标、随便挑一个"这条路是断的。而那一万个图标是**内嵌在二进制里**的
  // （spec §5）：它不是"别人的东西"，是这台机器自带的；人工挑的常用清单只是
  // 其中三十六个，其余九千多个仍然够不着。
  //
  // 一次倒出来是十几 MB 的 markup（面板会卡死、这台家用机器的内存也吃不住），
  // 所以按页要：服务端按 offset 给一页，界面滚到底再要下一页。
  //
  // **顺序是稳定的**（服务端 icons.Index.AllNames）：品牌层在前、通用层在后、
  // 层内按名字排序。翻页的位置就是顺序里的位置，顺序一变就等于翻乱了。
  async function loadIconBrowse() {
    if (iconPick.loading || !iconPick.panel) return;
    if (iconPick.browseTotal > 0 && iconPick.browse.length >= iconPick.browseTotal) return;
    iconPick.loading = true;
    try {
      const res = await fetch('/api/v1/icons/browse?offset=' + iconPick.browseOffset +
        '&limit=' + iconBrowsePageSize, { headers: { Accept: 'text/html' } });
      if (!res.ok) throw new Error('HTTP ' + res.status);
      const total = Number(res.headers.get('X-LB-Icons-Library') || 0);
      const html = await res.text();
      // 关掉了就别写了（那一层已经从文档里摘掉）。
      if (!iconPick.panel) return;
      const doc = new DOMParser().parseFromString(html, 'text/html');
      const tiles = [...doc.body.querySelectorAll('.icpick-tile')];
      iconPick.browseTotal = total;
      iconPick.browseOffset += tiles.length;
      iconPick.browse = iconPick.browse.concat(tiles);
      // 图库那一段的标题：**与第一页一起出现**（见 paintIconStarter 那段注释——
      // 在第一页到达之前那个"共 N 个"是 0，而"0 个"是一句假话）。
      if (!iconPick.libLabel) {
        iconPick.libLabel = el('p', { class: 'icpick-sec' });
      }
      iconPick.libLabel.textContent = T('app.icon.libAll', String(total));
      // 正在搜的时候不在屏上插（那一屏答的是"你搜到哪几个"），
      // 但**照样记进 browse**——清掉搜索词回到浏览视图时它们要还在。
      if (iconPick.mode === 'browse') appendBrowseTiles(tiles);
    } catch (err) {
      if (!iconPick.panel) return;
      const { note } = iconPick;
      note.textContent = T('app.icon.libFail');
      note.className = 'sheet-status bad';
    } finally {
      iconPick.loading = false;
    }
  }

  // 新的一页**插在「再加载一批」那枚键前面**，而不是整张网格重画：重画会把网格的
  // 滚动位置拨回顶部，于是"往下翻"在翻页时被打断——人按了「再加载一批」，看到的
  // 却是自己回到了第一屏。
  function appendBrowseTiles(tiles) {
    const { grid } = iconPick;
    if (!grid || !tiles.length) return;
    const frag = document.createDocumentFragment();
    // 那一段的标题排在它的格子**前面**（第一次插入时才插，之后它已经在屏上了）。
    if (iconPick.libLabel && !iconPick.libLabel.isConnected) frag.append(iconPick.libLabel);
    for (const t of tiles) frag.append(t);
    const more = grid.querySelector('.icpick-more');
    if (more) {
      grid.insertBefore(frag, more);
      const btn = more.querySelector('button');
      if (btn) btn.textContent = T('app.icon.libLoad', String(iconPick.browse.length));
    } else {
      grid.append(frag);
    }
  }

  // 清空搜索框要**回到浏览视图**而不是回到空白：那一下是"我不搜了，看刚才那批"，
  // 而把它变成一张空网格等于逼着人重新敲一遍刚才已经看过的词。
  //
  // 浏览视图是**三段**：这一份已经在用的（克隆卡片上那一枚，与卡片逐像素同源）
  // + 常用的（服务端那份清单）+ **图库里的全部**（可往下翻）。**自己那批在前**
  // ——自己用过的更可能是想要的那个。
  function paintIconStarter() {
    const { grid, note } = iconPick;
    if (!grid) return;
    const parts = [];
    if (iconPick.used.length) {
      parts.push(el('p', { class: 'icpick-sec', text: T('app.icon.libStarter') }));
      parts.push(...iconPick.used);
    }
    if (iconPick.common.length) {
      // 自己去重：自己用过的那个图标**也可能就在常用那批里**（github 就是），
      // 两格一模一样地并排摆着，人会以为界面上多了一枚。
      // 图库那一段**不去重**：它承诺的是"图库里的全部"，跳掉几条就破了这句话。
      const seen = new Set(iconPick.used.map((t) => t.dataset.icon));
      const rest = iconPick.common.filter((t) => !seen.has(t.dataset.icon));
      parts.push(el('p', { class: 'icpick-sec', text: T('app.icon.libCommon') }));
      parts.push(...rest);
    }
    // 图库那一段的标题**与它的第一页一起出现**：标题上写着"共 N 个"，而第一页
    // 还没到时那个 N 是 0——"0 个"是一句假话，而这一格恰恰是**不许说假话**
    // 的那一族（它答的是"这台机器里有多少图标"）。所以那一段在第一页到达
    // 之前根本不出现（appendBrowseTiles 里把它插在第一格之前）。
    if (iconPick.libLabel) parts.push(iconPick.libLabel);
    parts.push(...iconPick.browse);
    // 「再加载一批」那枚键**在网格里面**（不是网格外面）：网格是要滚的，
    // 外面那行永远在视口里，于是"还有没有"这件事看不见了。
    if (!iconPick.browseTotal || iconPick.browse.length < iconPick.browseTotal) {
      parts.push(el('div', { class: 'icpick-more' }, [
        el('button', {
          type: 'button', class: 'fld', text: T('app.icon.libLoad', String(iconPick.browse.length)),
          on: { click: () => loadIconBrowse() },
        }),
      ]));
    }
    grid.replaceChildren(...parts);
    // 浏览视图下状态行说的是"**怎么找到**"：搜一个名字，或者往下翻。
    // 搜索视图下它说的是搜到了几个（runIconSearch 自己写）。
    if (iconPick.mode === 'browse') {
      note.textContent = T('app.icon.libStart');
      note.className = 'sheet-status';
    }
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
    Object.assign(iconPick, {
      sel: null, panel: null, input: null, grid: null, note: null, opener: null, seq: 0,
      used: [], common: [], browse: [], libLabel: null, browseTotal: 0, browseOffset: 0, loading: false, mode: 'browse',
    });
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
      // 清空搜索 = 回到**浏览视图**（那三段 + 已翻出来的那几页）。
      // 浏览视图是"这一份用过的 + 常用的 + 整个图库"，不是一张空网格。
      iconPick.mode = 'browse';
      paintIconStarter();
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
    // 搜索结果**整张网格换掉**（浏览那一档暂时收起来）：这一屏答的是"你搜到哪几个"，
    // 混着浏览的三段会让人以为那些也是搜出来的。
    iconPick.mode = 'search';
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

  // **浏览器能解析成 URL 才值得发请求**（2026-10-24）：去掉原生校验之后，
  // 敲 "abc" 会先跑一次 resolve、按保存时再跑一次 batch，两次都必然失败。
  // 这里只拦"一眼不是地址"的形状；主机名、scheme、slug 的正式判据仍在服务端
  // 那一处（`model.CheckURL`），不在这里抄第二份。
  //
  // **必须放在下面那个 bookmarklet 预填之前**：`?add=` 那一支会在模块初始化时
  // 调用 `resolveNow()`，而它第一件事就是用这一格。写在 resolveNow 旁边时，
  // 那一发会撞上 TDZ（`Cannot access 'parseableAddress' before initialization`）：
  // 浮层照开、`t=` 带来的标题还在，但那一次读取整个不发生（说明/图标/建议分组
  // 都读不到，状态行空着），错误只在 console 里。
  const parseableAddress = (s) => {
    try { new URL(s); return true; } catch { return false; }
  };

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

  // ── 把一条链接拖进来（书签栏 / 地址栏 / 另一个页面上的链接）───────────────
  // **直接存进落点那一组，不问**（2026-10-16 改判）：动作已经完整——人是把一个地址
  // 拖到某一组上，落点就是"存进哪一组"，再弹一张表请人确认等于把做完的事拆成两段。
  // 它与书签栏按钮**故意不是同一条路**（按钮仍是预填 + 要人按一次保存）。
  //
  // 判据与出处（浏览器三种引擎给三种键、捕获阶段、站内拖动要挡、只收 http(s)）
  // 写在 AGENTS.md 那一行与 openapi.json 里，钉在 dropurl_test.go——这里只留
  // 代码本身必须看得见的那两条：**两种键**（Firefox 只给私有的 x-moz-url，只认
  // uri-list 的话在 Firefox 上整条不成立）与**挡掉站内拖动**（自家 MIME 用前缀判，
  // 写全名会多出第三处字面量，而 appdrag_test 数的正是那两处）。
  const dragIsURL = (dt) => {
    if (!dt || !dt.types) return false;
    const types = [...dt.types];
    if (types.some((t) => t.startsWith('application/x-lb'))) return false;
    return types.includes('text/uri-list') || types.includes('text/x-moz-url');
  };

  // 只收 **http/https 且整段就是一个地址**：从地址栏选一段文字拖进来时负载常常是
  // 「https://x.com 搜索词」，多带一个词就不该当成地址存进去；而 javascript:、
  // data: 这类地址服务端本来就拒（导入报告里那条判据），前端不重复发明。
  const urlFromDrag = (dt) => {
    // uri-list 是**行**格式：可能有多行、可能有 # 开头的注释行（Firefox 的
    // x-moz-url 则是「地址」与「标题」两行）。取第一条合格的就停。
    const raw = dt.getData('text/uri-list') || dt.getData('text/x-moz-url') || '';
    for (const line of raw.split(/\r?\n/)) {
      const s = line.trim();
      if (s && !s.startsWith('#') && /^https?:\/\/\S+$/i.test(s)) return s;
    }
    const plain = (dt.getData('text/plain') || '').trim();
    return /^https?:\/\/\S+$/i.test(plain) ? plain : '';
  };

  // **这两条要挂在捕获阶段**（第三参数 true）。卡片的 dragover/drop 为了站内排序
  // 调了 `stopPropagation()`，挂在 document 上的**冒泡**监听于是永远收不到拖到
  // 卡片上的那一次——症状是"拖进来没反应"，而排序那条路径全程在跑（落点高亮、
  // drop 被 srcOf 判空后静默 return）。捕获里先认下外来链接并按停，站内拖动放行。
  document.addEventListener('dragover', (e) => {
    if (!dragIsURL(e.dataTransfer)) return;
    e.preventDefault();
    e.stopPropagation();
    e.dataTransfer.dropEffect = 'copy';
  }, true);
  document.addEventListener('drop', (e) => {
    if (!dragIsURL(e.dataTransfer)) return;
    const url = urlFromDrag(e.dataTransfer);
    if (!url) return;
    e.preventDefault();
    e.stopPropagation();
    // 落点**在事件里取**：拖到某张卡/某一行上就是那一组，拖到组标题上就是标题
    // 那一组。取完之后才进异步——事件对象一 await 就过期了。
    // 松手的位置也一起带进去：新卡片要从那里滑过来（flip 的旧位置取的就是它）。
    saveDropped(url, dropGroupGid(e.target), { x: e.clientX, y: e.clientY });
  }, true);

  // 拖进来的网址该落进哪一组。**与站内排序的 `dropTargetOf` 同一判据**（行 → 它
  // 那一组；组标题 → 标题那一组），但这里要的是 **gid** 而不是一个可以 mark 的节点。
  //
  // 从前写的是 `closest('section[data-gid]')`（2026-10-16 修）：大卡带的组容器是
  // `ul.feat-row[data-gid]`，它上面那个 `section.feat` 没有 data-gid，于是**拖到大卡
  // 上会一路找到 null**，再由 `sec || 第一组` 兜底——**存进了完全不相干的那一组**，
  // 而屏上给的是"存进了你指的那一组"的横幅。那是这一族里最坏的一种：写进去了、
  // 报成功了、位置是错的。
  const dropGroupGid = (node) => {
    if (!node || !node.closest) return null;
    const li = node.closest('li[data-item]');
    if (li) return groupOf(li)?.dataset.gid || null;
    const head = node.closest('h2[data-group]');
    return head ? head.dataset.group : null;
  };

  // 卡片的**名字**：先要 resolve 读到的标题，读不到就用**网站的名字**（主机名）。
  //
  // 为什么不是那一整条地址（2026-10-16 实拍）：列表卡片在**没有说明时压根不印第二行**
  // （模板的 `{{if .Desc}}`），所以标题一空，那一格就只剩地址——而地址是 40 来个
  // 字符、名字是每天要扫的那一格。一屏里十几张卡只剩地址时，这一页没法扫。
  // 主机名是"这个网站叫什么"，恰好是此刻能拿到的最贴切的名字；完整地址并没有丢，
  // 它在 href 上（点得开）、在编辑那一格里（看得见）。
  const itemName = (title, url) => {
    const t = String(title || '').trim();
    if (t) return t;
    try { return new URL(url).host || url; } catch { return url; }
  };

  // 存进去的那一段。**只剩失败才说话**（§8.7：写失败一律显式报错）。
  // 标题仍然走 resolve：没有它，存进去的名字就是一串地址，而这一页全部的价值
  // 就是"看得懂的名字"；读不到**不是**不存的理由，退回到用网站的名字。
  async function saveDropped(url, dropGid, dropAt) {
    // 同一个地址拖两次不该多出一条。比的是屏上 href 的**原样**：不做规范化比较——
    // 把带不带尾斜杠当成两条不同的书签，比多存一条更难解释。
    // 按**任何带 href 的元素**比（从前是 `a.card`）：大卡带那张是 `a.feat-card`，
    // 于是拖一个大卡里已有的地址进来会被当成新的一条再存一次。
    if (document.querySelector('a[href="' + url.replace(/"/g, '%22') + '"]')) {
      showBanner(T('app.drop.dup'), 'bad');
      return;
    }
    // 拖到空白处就退到页面上第一组：这一页的卡片总是属于某一组。
    const targetGid = dropGid || groupsOnPage()[0]?.dataset.gid;
    if (!targetGid) { showBanner(T('app.drop.noGroup'), 'bad'); return; }
    let meta = null;
    try {
      const r = await api('/api/v1/resolve', { url, profile });
      if (!r.code) meta = r; // 读不到就用地址当名字，不因此丢掉这一条
    } catch { /* 同上：读取失败不等于不能存 */ }
    const name = itemName(meta && meta.title, url);
    const value = { id: slug(url, true), name, url };
    if (meta && meta.desc) value.desc = meta.desc;
    // 分组优先取 resolve 建议的那一组（它只从**这一份现有分组**里挑，绝不凭空
    // 造一个），否则就用拖到的那一组。
    const gid = (meta && meta.suggestedGroup && meta.suggestedGroup.id) || targetGid;
    const ops = [{ op: 'add', path: 'groups/' + gid + '/items', value }];
    // 离线时进待办队列而不是报错：与添加浮层那一条同一个去处（§8.7）。
    if (!navigator.onLine) {
      enqueue({ gid, ops: [], value });
      paintQueue();
      showBanner(T('app.offline.queued'));
      return;
    }
    // applyOps 失败时**自己**已经弹过一句（app.applyOps.fail，带上"改的是什么"），
    // 这里不重复说第二遍——两处各判一次同一个失败，迟早有一处忘了带信息。
    if (!await applyOps(ops, 'app.ops.item')) return;
    // 不重载了：**抓一次页面、只把刚存进去的那一行的片段取出来接上**，于是新条目
    // 从你松手的位置滑进它自己的格子（见接卡片那段）。接不上（抓不到、找不到那一行）
    // 就退回重载——宁可旧样子，不能屏上没有而盘上有。
    await spliceCardIn(gid, value.id, url, dropAt);
  }

  // 把服务端刚画好的那一行接进当前这一组，并让它从 `from`（拖放时指针的位置）滑过来。
  //
  // **为什么不自己在脚本里画一张卡**：卡片的标记由服务端模板画（图标那段尤其——
  // 图标库是内嵌产物，svgMarkup 是全案唯一把图标拼成 markup 的出口）。脚本再画
  // 一份就是"同一份 define 在两处落地"，而那种漂移项目里已经吃过一次亏。
  // **为什么抓得到**：列表是服务端渲染的，`fetch('/')` 回来那份就是答案。
  // **为什么只接一行**：整节替换会把这一节里所有行上挂好的监听器（把手/复制/拖动）
  // 一起丢掉——那正是 bindItem 那个函数存在的理由：接进来的新节点补挂一遍。
  //
  // **在目标组那一个 `<ul>` 里找**（判据见 `listOf`），且**先按 id、再按地址**
  // （2026-10-16 复查）：
  //   · 限定在目标组：页面上同一组只印一次，但**两组印成同一个形状**时不分就会
  //     接错地方——而接错的那一份屏上多出一条、盘上只有一条（与 moveInPlace
  //     撞过的同一个形状）。大卡带的容器是 ul 自己，写死 section 就找不到它。
  //   · 先按 id：那是**我们发过去的那一个**（`slug(url, true)`，服务端原样落盘），
  //     它罩得住三种卡片形态（可点的卡、不可点的外部地址、大卡）。
  //   · 地址作兜底：id 万一对不上时还有一条路——只认 `a.card` 会在大卡带（那里是
  //     `a.feat-card`）与外部地址那一支同时失效，所以按**任何带 href 的元素**找。
  async function spliceCardIn(gid, item, url, from) {
    const fallback = (why) => { console.warn('lb: card not spliced, falling back to reload:', why || '?', gid, url); reloadKeepingPlace({ gid }); };
    let node = null;
    try {
      // **必须显式带 Accept: text/html**（2026-10-16 实拍踩过）：不带时浏览器发的是
      // 默认的 `*/*`，而 sw.js 那一族只放行「Accept 里带 text/html」的 HTML 请求
      // （就是给 `/prefs` 那一路留的规矩）。于是这一发落到资产缓存那条 cache-first 上
      // ——**第一次**执行时被存进去，之后每次都读到那一份，而指纹资产按设计永不过期。
      // 症状极安静：新卡片接不上、`spliceCardIn` 静悄悄退回重载，看上去"有时会滑、
      // 有时闪一下"，像随机。带上这个头，这一发不进 SW，恒为网络那份新的。
      const html = await fetch('/', { credentials: 'same-origin', headers: { Accept: 'text/html' } })
        .then((r) => (r.ok ? r.text() : ''));
      const doc = new DOMParser().parseFromString(html, 'text/html');
      const list = listOf(gid, doc);
      node = (list && list.querySelector('li[data-item="' + item + '"]'))
        || (list && list.querySelector('a[href="' + url.replace(/"/g, '%22') + '"]'))?.closest('li[data-item]')
        || null;
    } catch { /* 抓不到就退回重载 */ }
    const ul = listOf(gid);
    if (!node || !ul) { fallback(!node ? 'row not in fetched page' : 'target group not found'); return; }
    ul.append(document.importNode(node, true)); // 服务端把它放在组末，新来的也在组末
    bindItem(ul.lastElementChild);
    bumpCount(gid, 1);
    syncEmpty(ul); // 拖进的是**空组**时，那句「这一组还没有书签」要跟着走
    // FLIP 的"旧位置"就取**你松手的地方**：那才是这一条在这一次动作里的来处。
    const li = ul.lastElementChild;
    if (from && from.x != null) flip(new Map([[li, from]]));
    else li.classList.add('is-new');
    showBanner(T('app.added.saved', groupTitle(gid)), 'ok');
  }

  // 成功横幅上那句组名，从**盘上刚写进去的那份 DOM** 里取（列表是服务端渲染的，
  // 接上来的那一行旁边就是它）。判据是 `groupTitle` 那一处，见它的注释。
  // 大卡带的标题也在内：那个 h2 带 data-group，挂在 ul 前面而不是 section 底下，
  // 写成 `section[data-gid] h2` 时存进大卡的那一条会显示组 id 而不是组名。

  // ── resolve：填标题/描述/建议分组/候选图标 ───────────────────────────────
  // 失焦就跑一次（不是每次按键）：粘贴 URL 的动作天然发生在 blur 之前，而每敲一个
  // 字符打一次请求会把 resolve 的冷却与限流打满。
  fUrl.addEventListener('blur', resolveNow);

  async function resolveNow() {
    const seq = ++statusSeq;
    const url = fUrl.value.trim();
    if (!url) return;
    if (!parseableAddress(url)) {
      say(T('app.resolveBadUrl'), 'bad', urlErr);
      return;
    }
    // 读之前先说"正在读…"——那是**这一次读取的状态**，归底部那条。
    say(T('app.reading'), 'busy');
    try {
      const r = await api('/api/v1/resolve', { url, profile });
      if (seq !== statusSeq) return;
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
      // **取 `r.suggestedGroup.id`**（2026-10-19 修）：服务端给的是 `{id,title}`
      // 对象（`resolveResponse.SuggestedGroup` 是 `*resolveGroup{ID,Title}`），
      // 而这里从前把整个对象跟 `o.value`（字符串）比 —— 恒不相等，于是**建议分组
      // 一次都没被选上**，浮层里那格永远停在默认的第一组。
      // 判据与服务端那份一致：对象里的 `id` 才是分组 id。
      if (r.suggestedGroup && fGroup.value !== NEW_GROUP) {
        const want = r.suggestedGroup.id;
        for (const o of fGroup.options) {
          if (o.value === want) { fGroup.value = want; break; }
        }
      }
      iconChoices(r.iconCandidates);
      say(r.title ? T('app.readOk') : T('app.readEmpty'), r.title ? 'ok' : 'warn');
    } catch (e) {
      // 这一轮已经被一次写入接手：它的错误说的是"这一次读取"，不该盖掉
      // "这一条存没存上"——那是人按过保存之后唯一关心的答案。
      if (seq !== statusSeq) return;
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
  // **id 的上限是 32**（`model.ValidSlug`：`^[a-z0-9][a-z0-9-]{0,31}$`）。从前这里
  // 只切到 40，于是**凡是派生出来的 id 超过 32 的地址都存不进去**：服务端回 422，
  // 人在屏上看到的只有「这一批没有写进去：这台机器不收刚才那些内容」，真话只在
  // 控制台里（2026-10-16 实测：`example.com/sortcheck-body-…` 派生出 36 字符的
  // id，整批被拒，而地址本身一点问题都没有）。
  //
  // 光切到 32 会**撞车**（两个长地址共用同一个 32 字前缀 → duplicate-id，也是 422）。
  // 所以切完补一段**由同一串文字算出来的稳定后缀**——与下一行分组 id 那个做法是同一
  // 个：可读的那一半留住，唯一性交给原文。同一个地址永远得到同一个 id。
  const SLUG_MAX = 32; // 判据只有 model 那一处正则；这里只是别把切出来的串送回去撞它
  function ascii(text) {
    const s = String(text).toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
    if (s.length <= SLUG_MAX) return s;
    const h = hash36(s);
    return s.slice(0, SLUG_MAX - h.length - 1) + '-' + h;
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
  // `hostname` 的第一段，于是 `10.0.2.3:6789` / `:7080` / `:23333` **全部**
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
      // `10.0.2.3.example.com` 那种仍走可读那条，而它不含私密信息。
      const isIPv4 = /^\d{1,3}(\.\d{1,3}){3}$/.test(host);
      if (isIPv4) return 'ip-' + (hash36(host + (u.port || '')) || 'item');
      // **整条路径 + query + hash**（2026-10-20 与扩展对上）：从前只取末段，
      // 于是 github.com/userA/project 与 github.com/userB/project 派生出同一个
      // id（撞 id = duplicate-id = 422），而两个视频/两篇文章（只有 query 不同）
      // 同样会撞。
      //
      // **这一段与 extension/bg.js 的派生是同一条判据**：同一个地址在页面与扩展
      // 上必须得到**同一个 id**，否则同一条书签会被存成两条（两边各存一条）。
      const base = host.split('.')[0] + '-' +
        (u.pathname.split('/').filter(Boolean).join('-') || '') +
        (u.search || '') + (u.hash || '');
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
    ++statusSeq; // 这一发写入的答案优先于任何还在路上的 resolve（见 say 上面那段）
    const url = fUrl.value.trim();
    // 空地址同样归那一格（与 resolve 的地址类失败同一处）：它说的是这一格，
    // 而这一格下面就是它自己。人按了「保存」得到一句飘在五格之外的话，
    // 得自己判断那句是在说哪一格——而这一格是唯一可能空的那一格。
    //
    // **先把焦点送过去，再写那句话**（2026-10-22 改）：焦点那一格有个
    // 「一敲就撤」的规矩（clearUrlErr：回到这一格就撤掉上一句，因为上一句已经
    // 不作数了）。而 `focus()` **会把焦点从「保存」那颗键搬回来**——于是那一次
    // focus 事件把**刚刚写上去的那句**当场擦掉，浮层里一个字都不剩。
    //
    // 顺序反过来就对了：focus 事件先跑（撤掉的是**上一轮**的话，本来就该撤），
    // 然后这一句才写上去。（焦点若本来就在这一格，focus() 不发事件，一样是
    // 「先撤旧的、再写新的」。）
    if (!url) { fUrl.focus(); say(T('app.err.url'), 'bad', urlErr); return; }
    // **坏地址不送 batch**（2026-10-24）：与 resolveNow 同一个判据，提前一步
    // 把必然 422 的那一发拦下来；焦点先送回去，再写那一格下面的话（顺序与
    // 空地址那条一样：focus 的「一敲就撤」会擦掉刚写的话）。
    if (!parseableAddress(url)) {
      fUrl.focus();
      say(T('app.resolveBadUrl'), 'bad', urlErr);
      return;
    }
    const name = itemName(fTitle.value.trim(), url);
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
      // **关掉浮层要 syncTrap()**（2026-10-20）：这一支从前只写
      // `sheet.hidden = true`，而 syncTrap 是**唯一**给 body 其余子元素
      // 摘/挂 inert 的地方——于是离线存完一条之后整页永久 inert：点不动、
      // Tab 也进不去，只能刷新。在线那一支（下面）一直记得调它。
      if (keepOpen) { for (const f of [fUrl, fTitle, fDesc]) f.value = ''; } else { sheet.hidden = true; syncTrap(); }
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
        // gid 与条目 id 一起带过重载：列表是服务端渲染的（不重载就看不到刚存的那条），
        // 而刚存的那一条要**被看见**——它带着 `is-new` 落位，屏上还有一句说它去了哪一组。
        reloadKeepingPlace({ gid: g.gid, item: value.id });
      }
    } catch (e) {
      // **连不上 ≠ 服务端说不行**（2026-10-24）：请求可能根本没到，也可能到了
      // 而响应丢了。统一记进待办，重放时按盘上现状对账——条目已在且地址相同
      // 就跳过，所以"存了但没收到 200"不会写成第二条，也不会丢掉这次输入。
      if (e.network) {
        enqueue({ gid: g.gid, ops: g.ops, value });
        for (const f of [fUrl, fTitle, fDesc]) f.value = '';
        iconChoices([]);
        sheet.hidden = true;
        syncTrap();
        showBanner(T('app.netQueued'), 'warn');
        return;
      }
      // 409 是"数据变了"，不是"再试一次就好"：让人自己看清了刷新。
      //
      // **地址类错误也归地址那一格**（2026-10-24）：resolve 在 blur 时已经写过
      // 一次同一句话，submit 这一发再写到底部就是屏上同一句红字两遍。只认
      // `url-incomplete` 一个 kind，不把 batch 的其它 field-invalid（例如分组名
      // 太长）也贴到地址格去。
      say(e.status === 409 ? T('app.conflict') : e.message, 'bad',
        e.kind === 'url-incomplete' ? urlErr : null);
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
  //
  // **清空时整个清单壳一起摘掉**（2026-10-24）：那份清单从 10-24 起包在一层
  // `.notice` 里（它多了一颗「补进去」）；只摘 `#lb-qlist` 会留下一个空壳，
  // 而离线时那颗重试键不画、又没有关闭键——空壳会一直挂到刷新。
  function closeQueue() {
    document.getElementById('lb-queue')?.remove();
  }

  function paintQueue() {
    const n = readQueue().length;
    let chip = $('#lb-q');
    if (n && !chip) {
      chip = el('button', { type: 'button', class: 'badge', id: 'lb-q',
        'aria-label': T('app.offline.aria'), on: { click: showQueue } });
      header.append(chip);
    }
    if (chip) chip.textContent = T('app.offline.chip', n);
    if (!n) { closeQueue(); if (chip) chip.remove(); }
    paintOnline();
  }

  function showQueue() {
    const old = $('#lb-qlist');
    if (old) { closeQueue(); return; }
    const q = readQueue();
    if (!q.length) return;
    const list = el('ul', { class: 'qlist', id: 'lb-qlist' }, q.map((entry, i) => el('li', {}, [
      el('b', { text: entry.value.name }),
      el('code', { text: entry.value.url }),
      el('button', { type: 'button', class: 'fld', text: T('app.offline.drop'), on: { click: () => {
        const left = readQueue();
        left.splice(i, 1);
        writeQueue(left);
        // 清单不是响应式的：必须把旧壳摘掉重画，否则删掉的那一条还留在屏上。
        closeQueue();
        paintQueue();
        if (readQueue().length) showQueue();
      } } }),
    ])));
    // **手动补一次**（2026-10-24）：自动重放只在 `online` 事件上跑；如果当时
    // 这台机器还没起来，队列会一直躺着等人。这一颗只在浏览器认为自己在线时
    // 出现——不在线时它只会白按。
    const retry = navigator.onLine
      ? el('button', { type: 'button', class: 'fld', text: T('app.offline.retry'), on: { click: () => { closeQueue(); replay(); } } })
      : null;
    header.after(el('div', { class: 'notice', id: 'lb-queue', role: 'status' }, [
      ...(retry ? [retry] : []),
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
    if (!q.length) { paintQueue(); closeQueue(); return; }
    replaying = true;
    let done = 0;
    const dropped = [];
    // **重放前先取最新 rev**（§8.7）：拿队列存下来的那个 rev 一定会撞 409，
    // 而那个 409 说的不是"数据变了"而是"你自己过期了"。同时留下盘上现状，
    // 给下面的**对账**用（2026-10-24）：请求可能已经写进去了，只是响应没回来。
    let doc = null;
    try {
      const cur = await (await fetch('/api/v1/data?profile=' + encodeURIComponent(profile),
        { credentials: 'same-origin' })).json();
      rev = cur.rev;
      doc = cur.data;
    } catch { /* 取不到就按队列里的 rev 试，撞了会逐条丢弃 */ }

    const itemURLs = new Map();
    const groupURLs = new Map();
    for (const g of (doc && doc.groups) || []) {
      const urls = new Set();
      for (const it of g.items || []) {
        itemURLs.set(g.id + '/' + it.id, it.url);
        urls.add(it.url);
      }
      groupURLs.set(g.id, urls);
    }
    const groupIds = new Set(((doc && doc.groups) || []).map((g) => g.id));
    const createdGroups = new Set();

    const left = [];
    for (const entry of q) {
      const key = entry.gid + '/' + entry.value.id;
      // 已经在盘上、而且是同一个地址：这一次的写入已经成功，只是响应丢了。
      if (itemURLs.has(key)) {
        if (itemURLs.get(key) === entry.value.url) { done++; continue; }
        // 同一个 id 却指向别的地址：不是"已存"，别静默吞；留着让人自己删/改。
        left.push(entry);
        continue;
      }
      // 同一个地址已经在这一组里（可能是 AI/CLI 用别的 id 存的）：也算到了。
      const knownURLs = groupURLs.get(entry.gid);
      if (knownURLs && knownURLs.has(entry.value.url)) {
        itemURLs.set(key, entry.value.url);
        done++;
        continue;
      }
      // 分组已经在盘上（或本次重放的前一条刚建出来）：只补条目，不再建组。
      let ops = entry.ops || [];
      if (groupIds.has(entry.gid) || createdGroups.has(entry.gid)) ops = [];
      ops = [...ops, { op: 'add', path: 'groups/' + entry.gid + '/items', value: entry.value }];
      try {
        const r = await api('/api/v1/batch', {
          profile, expectedRev: rev, by: 'web', ops,
        });
        rev = r.rev;
        itemURLs.set(key, entry.value.url);
        const urls = groupURLs.get(entry.gid) || new Set();
        urls.add(entry.value.url);
        groupURLs.set(entry.gid, urls);
        if (entry.ops && entry.ops.length) createdGroups.add(entry.gid);
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
    closeQueue();
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
    // **fetch 自己抛错时没有 status**：那是"连不上"（NAS 重启、Wi-Fi 掉了、
    // 页面开着而机器关机），不是服务端答了一个错。原样把 TypeError 的
    // "Failed to fetch" 印到中文页上，等于让人看一句英文技术话。离线队列
    // 只认 `navigator.onLine === false`，所以"Wi-Fi 还在、这台机器不在"
    // 这一档会走到这里——给它一句人话，并让调用方照常保留表单。
    let res;
    try {
      res = await fetch(path, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
        credentials: 'same-origin', // 会话 cookie 就是凭证（credScoped）
      });
    } catch {
      const e = new Error(T('app.netErr'));
      e.network = true;
      throw e;
    }
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
  // 「改完接着改下一组」那个一次性口令：/prefs/group 那一族按完会记它，重载回来
  // 之后再打开设置浮层。**读一次就删**（见下面 load 那处），它不是"用户开着
  // 设置"的状态——那个状态本来就该由浮层自己在 DOM 里表示。
  //
  // **它必须带 profile**：这一页是按档案渲染的，而 sessionStorage 是**这台机器
  // 这台浏览器**的。不带档案名的话，在 A 的页面上改一个颜色会顺手把 B 的设置
  // 面板也弹出来——而那次重载落点是 A，读到的是"有人要我开着面板"，它分不出是谁。
  const OPEN_KEY = 'lb-prefs-open-' + profile;
  // 拖进来那条的「存进哪一组」（2026-10-16）。同样是一次性口令、同样带 profile：
  // 它说的是"刚才那一次拖放"，而 sessionStorage 是这台机器这台浏览器的，
  // 不带档案名就会在 B 的页面上报 A 那次的结果。
  const DROPKEY = 'lb-dropped-' + profile;
  // 「刚才那一次重载是因为改了东西」的一次性标记（2026-10-16 动效整合）：
  // base.css 那一族给每张卡都写了入场 `rise`，而改完回来的这一页**不是到达**，
  // 是**变化**——整页十二张一起上浮会把真正变的那一条淹掉。记下这个，重载后由
  // app.css 的 `html[data-restore]` 掀掉那一族（app.js 只写标记，掀不掀是样式的判据）。
  // 同样读一次就删：留着的话，下一次**手动**刷新也会被当成"改完回来的"。
  const RESTORE_KEY = 'lb-restore-' + profile;
  // 面板左侧那一列**停在哪一节**的记忆（2026-10-15）。它必须跨一次重载活着：
  // 浮层里改完一组的颜色会 `reloadKeepingPlace()`，而重载后 `prefsLoaded` 复位、
  // 面板重新 fetch ——不记住它的话，"改第二十组"每按一次就被弹回第一类。
  //
  // **只带 profile**：与 OPEN_KEY 同一理由（sessionStorage 是这台机器这台浏览器
  // 的，而这一页按档案渲染）。key 用 radio 的 id 去掉前缀那截（look/out/groups），
  // 服务端那张名单不认就落回第一颗（httpapi 的 prefsTab）。
  const PREFS_TAB_KEY = 'lb-prefs-tab-' + profile;
  const prefsTab = (() => { try { return sessionStorage.getItem(PREFS_TAB_KEY) || ''; } catch { return ''; } })();
  const rememberPrefsTab = (id) => {
    const v = String(id || '').replace(/^prefs-tab-/, '');
    try { sessionStorage.setItem(PREFS_TAB_KEY, v); } catch { /* 存不下就当没记 */ }
  };
  // after 形如 `{gid, item}`：重载之后要在屏上说/动的那一条。
  // 两件都要**活过重载**（2026-10-16）：横幅与动效都是页面里画出来的节点，
  // location.reload 之后连同旧文档一起没了。留 gid/item 而不是留那句话——
  // 组名是服务端渲染的，重载之后才在页面上长出来，照那一刻的 DOM 去读才对。
  function reloadKeepingPlace(after) {
    try { sessionStorage.setItem(SCROLLKEY, String(scrollY)); } catch { /* 存不下就退回最上面 */ }
    try {
      sessionStorage.setItem(RESTORE_KEY, '1');
      if (after && after.gid) sessionStorage.setItem(DROPKEY, after.gid + '\n' + (after.item || ''));
    } catch { /* 存不下就算了：少说一句提示，不影响数据 */ }
    location.reload();
  }
  const back = (() => { try { return Number(sessionStorage.getItem(SCROLLKEY) || '0'); } catch { return 0; } })();
  if (back) {
    sessionStorage.removeItem(SCROLLKEY);
    addEventListener('load', () => scrollTo(0, back), { once: true });
  }
  // 「改完接着改下一组」那个口令（/prefs/group 那一族按完会记它）。
  // 读完即删——留着的话，下一次**手动**刷新也会把设置面板弹出来，而没有人
  // 想要那样。取不到就算了：拉不到面板时 prefsOpen 自己会退回整页 /prefs。
  if ((() => { try { return sessionStorage.getItem(OPEN_KEY) === '1'; } catch { return false; } })()) {
    try { sessionStorage.removeItem(OPEN_KEY); } catch { /* 存不下就当没记 */ }
    addEventListener('load', () => { prefsOpen(); }, { once: true });
  }
  // 拖进来那一条的「存进哪一组」：与上面同一种手法（读完即删），记的是 gid 与条目 id。
  if ((() => { try { return sessionStorage.getItem(DROPKEY) || ''; } catch { return ''; } })()) {
    const [gid, item] = (() => {
      try { const v = sessionStorage.getItem(DROPKEY) || ''; sessionStorage.removeItem(DROPKEY); return v.split('\n'); }
      catch { return ['', '']; }
    })();
    // 掀掉整页入场的那一族：要在**样式生效之前**落，而 module 脚本是 deferred
    // 的——它在 DOMContentLoaded 之前跑，第一次绘制通常还没发生。万一晚了一步，
    // 也只是那一族没被掀掉（退回到旧样子），不会坏。
    addEventListener('load', () => {
      if (item) document.querySelector('li[data-item="' + item + '"]')?.classList.add('is-new');
      const head = document.querySelector('h2[data-group="' + gid + '"]'); // 大卡带的标题也在内，见 groupTitle
      const name = (head && (head.getAttribute('title') || head.textContent) || '').trim();
      showBanner(T('app.added.saved', name || gid), 'ok');
    }, { once: true });
  }
  // 「刚才那次重载是改完回来的」：掀掉 base.css 那一族的入场（app.css 判它）。
  if ((() => { try { return sessionStorage.getItem(RESTORE_KEY) === '1'; } catch { return false; } })()) {
    try { sessionStorage.removeItem(RESTORE_KEY); } catch { /* 存不下就当没记 */ }
    document.documentElement.setAttribute('data-restore', '');
  }

  // ── ⌘/Ctrl K 命令面板（§8.6）───────────────────────────────────────────
  // **叠加能力而不是默认视图**：它是覆盖在上层的一个框，页面本身照常可点、
  // 可用浏览器找词。做成"另一个页面"的话，⌘K 就把搜索框和阅读位置全丢了。
  const cmdkInput = el('input', { type: 'text', id: 'lb-k', class: 'fld', role: 'combobox',
    'aria-expanded': 'true', 'aria-controls': 'lb-k-list', 'aria-autocomplete': 'list',
    placeholder: T('app.cmdk.placeholder'), autocomplete: 'off' });
  const cmdkList = el('ul', { class: 'cmdk-list', id: 'lb-k-list', role: 'listbox' });
  const cmdk = el('div', { class: 'cmdk', hidden: 'hidden', role: 'dialog', 'aria-modal': 'true', 'aria-label': T('app.cmdk.aria') }, [
    cmdkInput, cmdkList,
  ]);
  body.append(cmdk);

  let cmdkHits = [], cmdkSel = 0;
  function cmdkOpen() {
    // **记住是谁打开它的**（2026-10-20）：关掉时要把焦点还给那一位——
    // 那个按钮/输入框此刻多半还停在原地。
    cmdk.__opener = document.activeElement;
    cmdk.hidden = false;
    syncTrap();
    cmdkInput.value = '';
    cmdkRender();
    cmdkInput.focus();
  }
  // **焦点要还回去**（2026-10-20）：Esc 关掉 ⌘K 之后，焦点原本停在那个被
  // hidden 掉的输入框上——节点不在了，焦点掉回 <body>，键盘用户被扔到页面
  // 最顶端。台面面板那一份（prefsClose）一直记得还。
  function cmdkClose() {
    const opener = cmdk.__opener || document.activeElement;
    cmdk.hidden = true;
    syncTrap();
    if (opener && opener.isConnected) opener.focus();
  }

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
    // 多选从前**只有一个入口：长按卡片**（HOLD_MS 那一段）。它对手指是自然的，
    // 对鼠标用户则是"这个功能不存在"——而批量打标签、批量删、一次挪好几条
    // 只有这一个门。它确实写在快捷键清单里（`?` 那一屏），可那一屏要先按 `?`
    // 才知道有。面板是这一页上唯一一处明写着"做点什么"的地方，动作该列在这儿：
    // 长按照旧能用，这一格只是让不知道那只手势的人也能找到同一件事。
    actions.push({ kind: 'action', label: T('app.cmdk.multi'), run: () => { cmdkClose(); enterMulti(); } });
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
      const name = (li.querySelector('b')?.textContent || '').toLowerCase();
      const url = itemURL(li).toLowerCase();
      // 判据与 `filter.go` 的 matchItem 是**同一组字段**：名字、地址、说明、每一个
      // 别名、每一个标签，再加"分组标题命中就把整组带出来"。从前这里只比名字与地址，
      // 于是同一个词在同一个产品里有两个答案——实测（同一份数据、两条通路各跑 11 个词）：
      // 「NAS」整页 8 条 / 面板 2 条，「影音」（那是组名）3 条 / 0 条，「pt」「备份」
      // 「对象存储」都是服务端有、面板回一句"没有叫「X」的书签"。
      // 值从 li 上的 data-desc / data-alias / data-tags 读，多个值以换行连接。
      const gtitle = groupName(groupOf(li)).toLowerCase();
      const extra = [li.dataset.desc || '', ...(li.dataset.alias || '').split('\n'), ...(li.dataset.tags || '').split('\n')];
      if (!q || name.includes(q) || url.includes(q) || gtitle.includes(q)
        || extra.some((x) => x && x.toLowerCase().includes(q))) {
        total++;
        if (hits.length < 8) {
          // `external` 与正文那一支同源：**它没有 <a href> 就是点不动**，
          // 于是「点不动」这件事在 DOM 上是现成的，不另找判据。
          hits.push({ kind: 'item', label: li.querySelector('b')?.textContent || '',
            url: itemURL(li), el: li, external: !li.querySelector('a[href]') });
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
        // **每行一个稳定 id**（2026-10-20）：输入框要靠 aria-activedescendant
        // 指着当前那一行——它是一个 listbox，焦点一直在输入框上，读屏要靠
        // 这一格才知道"现在选中的是哪一条"。没有它，上下移动只有底色变化，
        // 读屏一个字都不说。
        id: `lb-k-opt-${cmdkList.children.length}`,
        on: {
          click: () => cmdkRun(h),
          mouseenter: () => { cmdkSel = cmdkHits.indexOf(h); cmdkPaint(); },
        },
      }, h.kind === 'more'
        // 这一行不是书签，不能长得像书签：没有地址、没有类型角标，只有一句动作。
        ? [el('span', { text: h.label })]
        : [
          el('b', { text: h.label }),
          h.external
            // 复制而不是跳转，这一行得先说清按下去会发生什么——否则「按了没反应」
            // 只是换了个更隐蔽的形状（地址确实进了剪贴板，可屏上什么都没变）。
            ? [el('code', { text: h.url }), el('span', { class: 'tag', text: T('app.cmdk.external') })]
            : h.url ? el('code', { text: h.url }) : el('span', { class: 'tag', text: T('app.cmdk.kind') }),
        ]);
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
    // **告诉读屏"现在选中的是哪一行"**（2026-10-20）：焦点始终在输入框上
    // （它是个 listbox，焦点不能落在 option 上），所以读屏只能靠这一格知道
    // 当前项。没有它，上下移动只有底色变化——听屏的人一个字都收不到。
    const sel = cmdkList.children[cmdkSel];
    if (sel && sel.id) cmdkInput.setAttribute('aria-activedescendant', sel.id);
    else cmdkInput.removeAttribute('aria-activedescendant');
  }
  function cmdkRun(hit) {
    cmdkClose();
    // **外部条目复制地址，不跳转**（2026-10-19 修）。
    //
    // 面板扫的是**全部** `li[data-item]`，模板那一支却把点不动的外部地址
    // （chrome:// / file:// …）渲染成 `<code>` 纯文本，于是 `itemURL` 能把真实
    // 地址读回来——到这里为止都对。原先这里无条件 `location.href = hit.url`，
    // 于是按下去浏览器拒绝跳转、页面原地不动、**一声不吭**：这正是 §8.1
    // 「点了没反应的东西不进界面」说的那种承诺。
    //
    // 模板那一支已经把这枚徽标换成了能按的复制键（见下面那个
    // `.plain .badge.warn.ext` 循环），判据是同一个 `itemURL`；面板这一支漏了
    // 同一层，于是**同一个地址在正文里能复制、在面板里点了没反应**。
    // 出路（复制到地址栏或拖到书签栏）由模板那条路径给出，那边本来就写着。
    if (hit.kind === 'item') {
      if (hit.external) { copyText(hit.url); return; }
      location.href = hit.url;
      return;
    }
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
  // ── 拖拽落点的坐标（2026-10-15 三个洞都出在这一族）─────────────────────────
  //
  // **认 [data-gid] 而不是 section**：正文组是 section[data-gid]，而大卡带是
  // ul.feat-row[data-gid]——带子里一段段共用同一个 <section class="feat">，
  // 那一段没有自己的 section。从前这里写的是 `li.closest('section').dataset.gid`，
  // 症状很具体：大卡带里取到 undefined，于是发出去的 op 是
  // `groups/undefined/items/…`，服务端回「没有分组 undefined」，拖一下弹一条红条。
  const groupOf = (node) => node.closest('[data-gid]');
  const itemsOf = (group) => [...group.querySelectorAll('li[data-item]')];

  // 这一条的地址在 DOM 上由**哪一个元素承载**：卡片一共三种形态，各有一个载体——
  //   · 可点的（正文 `.card`、大卡 `.feat-card`）：href；
  //   · 正文里点不动的外部地址：`<code>` 的文本（模板与复制键读同一处）；
  //   · **大卡带里点不动的外部地址**：屏上一个地址都不印（只有主机名），模板在卡片上
  //     写了 `data-addr`（见 profile.tmpl 那段注释：这一行是唯一一处「屏上没有地址」）。
  //
  // 从前各处各写一遍查找，于是第三种形态一处都没覆盖到：**编辑模态读成空串**，
  // 照发一条 `set …/url=""` → 整批 422，屏上只说「这一条没存上」；⌘K 按地址也搜不到
  // 它（同一个词在整页搜索里能搜到、在面板里搜不到）。判据收在这一处（2026-10-16）。
  const itemURL = (li) => {
    const a = li.querySelector('a[href]');
    const href = a && a.getAttribute('href');
    if (href) return href;
    const code = li.querySelector('code');
    if (code && code.textContent.trim()) return code.textContent.trim();
    const raw = li.querySelector('[data-addr]');
    return (raw && raw.getAttribute('data-addr')) || '';
  };

  // 「屏上有哪几个分组」这一份集合，判据收在这一处。四个功能各要一次：
  // 拖网址进来的兜底（页面上第一组）、添加浮层的分组下拉、编辑模态的分组下拉、
  // 多选的「移动到」名单。前三个与第四个从前各写一遍选择器，其中排法切换那一族还写的是
  // `main section[data-gid]`——于是**大卡组不在其中**：它没有自己的 section（容器是
  // ul.feat-row）。那处仍然自己遍历是有意的，见它上面的注释。
  const groupsOnPage = () => [...document.querySelectorAll('[data-gid]')];

  // 一组的列表是哪一个 `<ul>`：**组容器有两种形状**——正文是 `section[data-gid] > ul`，
  // 而设成大卡的那一组被**整组**搬出了正文（exec.go），它的容器是 `ul.feat-row[data-gid]`
  // 自己（profile.tmpl 那两支）。从前两处都写死 `section[data-gid=…]`，于是**大卡带永远
  // 找不到**：排序每一次都走「算不出落点就退回重载」那一支——服务端搬对了，屏上却整页
  // 闪一次、每张卡重播入场，而那恰好是页面最显眼的那一块；拖网址进来则静悄悄退回重载。
  // 同一个坑 `pickMoveTarget` 踩过一次（「移动到」名单里根本没有大卡组），判据留在这一处。
  // 第二个参数是根：抓回来的那一页（`spliceCardIn`）要在**它自己的 document** 里找。
  const listOf = (gid, root) => {
    const host = (root || document).querySelector('[data-gid="' + gid + '"]');
    if (!host) return null;
    return host.tagName === 'UL' ? host : host.querySelector(':scope > ul');
  };

  // **空组那句「这一组还没有书签」要跟着改**（2026-10-16）：就地移动不走重载，于是
  // 把第一条搬进空组之后，卡片在那儿、那一句还在说这一组是空的；从一组搬空之后反过来。
  //
  // **只翻那一句的 hidden，不自己造它**（profile.tmpl 那一支永远画着它，有内容时
  // hidden）：文字只有模板那一处来源。脚本里另抄一份就是两处会漂，而它连语言包的
  // `app.*` 切片都进不去（app.js 读不到别的前缀）。
  //
  // 判据是"模板那一支画没画"：只管正文那种组（`ul` 挂在 section 底下），大卡带
  // 模板刻意没有这一句——凭空给它造一句才是新添的毛病。
  function syncEmpty(list) {
    if (!list || !list.parentElement || list.parentElement.tagName !== 'SECTION') return;
    const hint = list.parentElement.querySelector(':scope > p.grp-empty');
    if (!hint) return;
    hint.hidden = !!list.querySelector('li[data-item]');
  }

  // 一个组的显示名，**判据只有一处**：`h2[data-group="<gid>"]`。
  //
  // 正文组与**大卡带的组标题都带这个属性**（带子那一段的 h2 在 ul 前面，从容器往上找
  // 拿到的也是它）。从前是两份：一份按 gid 全局查，一份按「离这一组最近的那个 h2」
  // （`:scope > h2 || previousElementSibling`）。两份在**正文那一段之外就分叉**了——
  // 带子里一段段共用同一个 section，只认 `:scope > h2` 的话带子里任何一条都拿到第一段
  // 的组名，编辑模态里「我改的是哪一组」会答错组。
  //
  // 读的顺序：title（服务器写的全名，含拍平后的父路径）→ `.sec-leaf`（屏上那个叶名）
  // → gid。**不能读 h2 的 textContent**：排法切换那颗字形键是 append 进 h2 的，
  // 一并读进来，组名就变成「应用▦」这种。
  function groupTitle(gid) {
    const head = gid && document.querySelector('h2[data-group="' + gid + '"]');
    if (!head) return gid || '';
    return (head.getAttribute('title') || head.querySelector('.sec-leaf')?.textContent || gid).trim();
  }

  // 同上，只是从容器出发（多选与编辑那两个「移动到/分组」名单在按容器遍历）。
  function groupName(group) {
    if (!group) return '';
    if (group.tagName === 'H2') {
      return (group.getAttribute('title') || group.querySelector('.sec-leaf')?.textContent || group.dataset.group || '').trim();
    }
    return groupTitle(group.dataset.gid);
  }

  function opsFor(li, fields) {
    const gid = groupOf(li)?.dataset.gid;
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
  // 落在这一行的**上半还是下半**——队尾能不能够着，全看这一句。
  // 从前只有一种落点（index = 被指着那条的下标，也就是"插到它前面"），
  // 于是能插到任何一条前面，却插不到任何一条后面。
  const dropAfter = (li, e) => {
    const r = li.getBoundingClientRect();
    return e.clientY > r.top + r.height / 2;
  };
  const clearDropMarks = () => document.querySelectorAll('.drop, .drop-after, .drop-tail')
    .forEach((n) => n.classList.remove('drop', 'drop-after', 'drop-tail'));

  // 拖起来的那一条：**源组 id 在拖起那一刻就记住**。跨组时落点算出来的是**目标**
  // 组，而 op 的 path 指源组——两件事的答案不在同一个地方，而 dataTransfer 里只有
  // 条目 id（那是跨组唯一对得上的键），组只能从被拖的这一行自己读。
  let dragSrc = null;
  // dragstart 时量的**每一行的位置**（FLIP 的"First"那一半）。排序要的是"这一行
  // 从这儿滑到那儿"，而量位置必须在 DOM 被改**之前**。
  //
  // **改成懒的**（2026-10-16）：从前在 dragstart 就把全页每一行量一遍，而 dragstart
  // 在**每一次**按下时就发生——包括那些根本没落点的拖（拖到侧栏、拖出窗外、拖两下
  // 又缩回去）。四百条的书签页上那是每次拖动 400 个 getBoundingClientRect 加一个
  // 400 项的 Map，而落点往往一个都用不上。现在等**第一次真的经过一个落点**再量
  // （dragover 里，见 ensureDragBefore）——那一刻 DOM 还没动，量到的仍是原位。
  let dragBefore = null;
  const ensureDragBefore = () => {
    if (dragBefore) return;
    dragBefore = new Map([...document.querySelectorAll('li[data-item]')].map((n) => [n, n.getBoundingClientRect()]));
  };
  // 落点读源组时**必须与 MIME 对上**：只认自己那套 MIME 是原来的判据（从桌面或
  // 另一个标签页拖进来的东西不该动书签顺序），而 MIME 对上了却找不到源组时宁可
  // 不动——猜错的后果是把条目写进另一个组。
  const srcOf = (e) => {
    const id = e.dataTransfer.getData('application/x-lb-item');
    if (!id) return null;
    if (!dragSrc || dragSrc.id !== id) return null;
    return dragSrc;
  };
  // **dragover 只认自家那一种拖动**（2026-10-16 修）。dragover 阶段拿不到 getData
  // （保护模式里只有 types 可读），所以判据是"这次拖动带没带自家那个 MIME"。
  //
  // 从前那一族 dragover 不判，于是**把页面上的一段选中文本拖到某张卡上**也会亮出
  // "落在这里"的标记、光标变成移动，松手时 `srcOf` 判空静默 return——界面上给了
  // 一个做不到的承诺（§8.1「点了没反应的东西不进界面」）。外来链接那一路早被
  // 捕获阶段那条按停了，剩下这一路就是从网页上拖文字/图片进来的那些。
  const isInternalDrag = (e) => !!dragSrc
    && [...(e.dataTransfer && e.dataTransfer.types || [])].includes('application/x-lb-item');
  // 落点标记**一次只亮一个**。从前是"当前这行 toggle + dragleave 清掉"，而同一
  // 次悬停里祖先那个组级落点也会跟着亮（那才是当初给 dragover 加 stopPropagation
  // 的原因）。现在由 dragover 每次先全清再点一个，dragleave 那一族监听整个不需要
  // 了——"亮着哪一个"由"指针此刻在谁身上"这件事单独决定。
  const markDrop = (node, after) => {
    clearDropMarks();
    if (!node) return;
    if (node.tagName === 'H2') { node.classList.add('drop-tail'); return; }
    node.classList.toggle('drop', !after);
    node.classList.toggle('drop-after', after);
  };

  // ── 动效：排序就地完成，不整页重载（2026-10-16）────────────────────────
  // 此前每一次排序都以 `reloadKeepingPlace()` 收尾，于是"松手"看到的是**整页闪一次**，
  // 每张卡重播一遍入场；而排序是全案唯一一个"人一直盯着那一行"的动作。
  // 判据只有一条：**屏上的落点必须与发出去的那条 op 是同一份**——index 不另算，
  // `moveInPlace` 复刻的正是服务端那次插入（先摘再插）。跨组还要改两处计数，
  // 否则屏上说的数与盘上存的不一样。落点算不出来就**退回重载**：宁可旧样子，不能错样子。
  // FLIP：量旧位置 → 改 DOM → 量新位置 → 无过渡地摆回旧位置 → 下一帧放开补间。
  // 全程只动 transform（§8.1）；减弱动效那条闸关掉 transition 后退化成"直接落位"。
  function flip(before) {
    if (!before) return;
    const moved = [];
    for (const [n, was] of before) {
      if (!n.isConnected) continue;
      const now = n.getBoundingClientRect();
      const dx = was.x - now.x, dy = was.y - now.y;
      if (!dx && !dy) continue;
      n.style.transition = 'none';
      n.style.transform = `translate(${dx}px,${dy}px)`;
      moved.push(n);
    }
    if (!moved.length) return;
    requestAnimationFrame(() => requestAnimationFrame(() => {
      for (const n of moved) {
        n.style.transition = 'transform .19s cubic-bezier(.2,.8,.2,1)';
        n.style.transform = '';
      }
      // 补间跑完把内联样式清掉：留着会让下一次 FLIP 的起点算错（它量的是当前 rect）。
      setTimeout(() => { for (const n of moved) { n.style.transition = ''; n.style.transform = ''; } }, 240);
    }));
  }

  // 一组在屏上的**计数格**：侧栏/顶栏导航那一格里的 `<b class="cnt">`，
  // 大卡带那一行的 `.feat-n`。
  //
  // **判据只有一处**：bumpCount（改它）与 groupLabel（读它）共用它——两处各写
  // 一遍选择器，早晚会漂。
  //
  // **按整段 href 比，不按前缀比**（2026-10-16 审计时看到的）：`^=` 会把 `#g-news` 与
  // `#g-news2` 一起算进来——两组 id 互为前缀时（导入按文件夹名生成 id，这种一对很常见），
  // 搬一条进 `news`，屏上 `news2` 的计数也跟着 +1，而盘上没动它。
  // 带 ?q= 时 href 后面跟着查询串，所以判据是「等于 `#g-<id>` 或以 `#g-<id>?` 开头」。
  function countNodes(gid) {
    const out = [];
    for (const a of document.querySelectorAll('a[href^="#"]')) {
      const href = a.getAttribute('href') || '';
      for (const p of ['#g-' + gid, '#f-' + gid]) {
        if (href === p || href.startsWith(p + '?')) { const n = a.querySelector('b.cnt'); if (n) out.push(n); }
      }
    }
    const fn = document.querySelector('h2.feat-g[data-group="' + gid + '"] .feat-n');
    if (fn) out.push(fn);
    return out;
  }

  function bumpCount(gid, by) {
    const bump = (n) => { n.textContent = String(Math.max(0, (Number(n.textContent) || 0) + by)); };
    for (const n of countNodes(gid)) bump(n);
  }

  // 一个组在「挑一组」的那些名单里**该叫什么**（2026-10-22）。
  //
  // **同名要能分开**：台面面板「＋ 新分组」不查重名（id 由服务端派生，名字原样留下）、
  // 「改名」那一格也不查，浏览器导出的书签里也有两个同名文件夹——所以磁盘上躺着两个
  // 叫「常用」的组是**正常形状**。侧栏那一份导航分得清（它带着条目数：常用 1 / 常用 4），
  // 而这三处（添加浮层的下拉、多选里的「移动到」、编辑模态里的「分组」）从前只印标题，
  // 于是两个「常用」长得一模一样——人挑了哪一组全靠猜，存错了要回头翻。
  //
  // **判据只有一处**：同一个标题在页面上出现不止一次时才补上条目数；标题唯一时
  // 一个字都不多（那一档最常见，不该被噪声打扰）。
  //
  // **条目数取的是侧栏那一格里的同一个数**（countNodes 那一处，bumpCount 改的也是
  // 它），所以这几处与导航永远说同一件事。
  //
  // **歧义也从导航那几行判**：每一行的 title 属性是那一组的**整条路径**（与服务端
  // 渲染给正文节标题的那一份同源），于是「有哪几个组、谁跟谁同名」在导航与这三处
  // 之间是**同一份判据**，而不是这里再数一遍。
  //
  // **为什么不用 groupsOnPage()**：那一份是 `const`，而添加浮层的下拉**在模块
  // 初始化的时候**就把它建好了——那时候 groupsOnPage 还在 TDZ 里（声明在文件后面），
  // 引用它会让整个模块抛 ReferenceError，于是**下拉里一条选项都没有**（实测）。
  // 导航那几行是**服务端渲染**的，初始化时就在 DOM 里。
  //
  // 收 (gid, 名字) 而不是收一个元素：三处给的元素不一样（添加浮层给的是 h2，
  // 另外两处给的是带 data-gid 的容器），而**名字与 gid 都有**。
  function groupLabel(gid, name) {
    if (!name || !gid) return name;
    let same = 0;
    for (const a of document.querySelectorAll('.gnav a[title]')) {
      if ((a.getAttribute('title') || '').trim() === name) same++;
    }
    if (same < 2) return name;
    const n = countNodes(gid)[0];
    return T('app.group.count', name, n ? (Number(n.textContent) || 0) : 0);
  }

  // index 为 null = 追加到末尾（与服务端 `index` 缺席同义）。
  //
  // **要挪的是被拖的那一条，不是落点那一行**（2026-10-16 实拍踩过）：落点那行只负责
  // 算出 index。传错的后果很安静——把落点那行摘下来再按同一下标插回去等于什么都没发生，
  // 而服务端挪的是被拖的那条，于是**屏上与盘上分家**。查 li 还**必须限定在源组那一个
  // `<ul>` 里**：大卡带（`ul.feat-row`）把同一组整个印在页面最前面，不限定就可能
  // 挪走卡带那一份。
  // `before` 由落点处理器**在 await 之前**取出来传进来：dragend 在 drop 之后立刻触发，
  // 而 applyOps 还要一个来回——等真正要用时它已经被清成 null（位置对了、动画没有）。
  function moveInPlace(dstGid, index, before) {
    const srcList = dragSrc && listOf(dragSrc.gid);
    const li = srcList && srcList.querySelector('li[data-item="' + dragSrc.id + '"]');
    const ul = listOf(dstGid);
    if (!li || !ul) { reloadKeepingPlace(); return; }
    li.remove(); // 同组时它自己还在列表里：先摘掉，下标才对得上
    // 插到哪里由 `itemsOf` 说了算，与落点那一份**同一判据**：那处算 index 用的是
    // `li[data-item]`，这里插进去用的是 `ul.children`——两者一旦不是同一批元素，
    // 就是一个下标差一位的错，而屏上看不出来。
    const sibs = itemsOf(ul);
    ul.insertBefore(li, index == null ? null : sibs[index] || null);
    if (dragSrc.gid !== dstGid) {
      bumpCount(dragSrc.gid, -1);
      bumpCount(dstGid, 1);
      syncEmpty(srcList);
    }
    syncEmpty(ul);
    flip(before);
  }


  for (const badge of document.querySelectorAll('.plain .badge.warn.ext')) {
    const plain = badge.parentElement;
    // 地址也从 `itemURL` 那一处读（判据只有一处）：这一行本来就有一份完整的地址在
    // `<code>` 里，而各处自己读 `<code>` 的形状就是"有一种卡片形态没有 code"。
    const url = itemURL(badge.closest('li') || badge);
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

  // 每一行的把手、复制键与 `draggable` 属性挂在这里（**拖动的监听不在这里**，
  // 2026-10-16 起全部委托到 `<main>` 上，见下面那个委托块）。
  //
  // **它必须是一个函数而不是一个循环**：拖进来的那一条是从服务端片段接上来的新
  // 节点（见接卡片那段），新节点上的把手/复制键一个都没有——不抽成函数就没有
  // "只给这一行补挂一遍"这个出口，于是要么整段重挂（已挂过的会挂两遍，一次拖动
  // 触发两次排序）要么新行没有把手。
  function bindItem(li) {
    // 幂等：它会 append 一颗把手，绑两遍就是两颗把手。判据写在节点上而不是靠调用点自觉。
    if (li.dataset.lbItemBound) return;
    li.dataset.lbItemBound = '1';
    // **这一行该有的全部监听都从这里出去**（2026-10-16）：长按多选那一族从前是页面
    // 加载时另扫一轮挂上去的，于是**拖进来的那一行没有它**——长按没反应、在多选里
    // 点它也不切换。新节点是接卡片那段 `spliceCardIn` 接进来的，它只调这一个函数。
    bindMulti(li);
    // 把手与拖拽挂在**这一行的主体**上：可点的地址是 .card，点不动的那几行是 .plain。
    // 2026-10-12 之前这里只认 .card，注释写的是"非 http 的不给把手（不给做不到的承诺）"——
    // 那句把两件事混成了一件：**打不开**是浏览器的限制，**改不了**不是。
    // 就地编辑写的是 name/desc/url，与协议无关；把把手摘掉等于让一条 chrome://
    // 或 file:// 的书签在界面上永远改不了名、换不了说明，而浏览器收藏夹导入
    // 恰好会把这类地址成批收进来（AGENTS.md：file: 照收）。
    const card = li.querySelector('.card, .feat-card, .plain');
    if (!card) return; // 三种形态都不是（坏档那一屏根本没有 li）才真的没有把手
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
    // （属性已经留下了）第二次才动。拖动的**监听**不在这里（见下面那个委托块）。
    card.setAttribute('draggable', 'true');
  }
  for (const li of document.querySelectorAll('li[data-item]')) bindItem(li);

  // ── 拖放：四种落点，一个委托块（2026-10-16）────────────────────────────
  // 从前是「每一行挂五个（dragstart/dragend/dragover/dragleave/drop）+ 每个组
  // 标题挂三个」，而它们做的事全都只取决于**事件落在谁身上**。四百条的书签页
  // 上那是两千多个闭包与一组两两无关的 stopPropagation（"不让祖先那个组级落点
  // 同时亮着"就是其中一条存在的原因）。委托之后每种事件各一个处理器，而"落在谁
  // 身上"由 closest() 说——与排序真正操作的盒子（li[data-item]）是同一份判据。
  //
  // **挂在 main 上而不是每行**还有第二个好处：接进来的新节点（见 spliceCardIn）
  // 根本不需要为拖放补挂任何东西，bindItem 只剩把手、复制键与 draggable 三件事。
  // 挂在 main 上而不是每行：profile.tmpl 只有一个 `<main role="main">`（第 120 行，
  // 另一处在 816 是坏档那一屏，它压根没有 li），书签列表整个在它里面。退到 body
  // 是为了这一段在别的形状里也不至于因为选不到就静悄悄不绑。
  const dragRoot = document.querySelector('main') || body;
  dragRoot.addEventListener('dragstart', (e) => {
    const card = e.target.closest && e.target.closest('.card, .feat-card, .plain');
    const li = card && card.closest('li[data-item]');
    if (!li) return;
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
    dragSrc = { id: li.dataset.item, gid: groupOf(li)?.dataset.gid || '' };
  });
  // 拖到**某一行**上：上半=插到它前面，下半=插到它后面（队尾因此够得着）。
  // 拖到**组标题**上 = 追加到那一组末尾（不给 index，服务端 move 缺席即末尾）。
  // 拖到两样都不是的地方（侧栏、行与行之间的空隙）= **清掉标记，不给落点**。
  const dropTargetOf = (e) => {
    const t = e.target;
    if (!t || !t.closest) return null;
    const li = t.closest('li[data-item]');
    if (li) return { node: li, after: dropAfter(li, e) };
    const head = t.closest('h2[data-group]');
    return head ? { node: head, after: false } : null;
  };
  dragRoot.addEventListener('dragover', (e) => {
    if (!isInternalDrag(e)) return;      // 外来的一律不接管（见 isInternalDrag 那段）
    // 整片 main 都阻止默认：否则松手时浏览器会拿拖着的文字/地址去导航，而这一屏
    // 的落点只有下面那两种。
    e.preventDefault();
    e.stopPropagation();
    ensureDragBefore();
    const t = dropTargetOf(e);
    markDrop(t && t.node, t && t.after);
    e.dataTransfer.dropEffect = t ? 'move' : 'none';
  });
  dragRoot.addEventListener('drop', async (e) => {
    if (!isInternalDrag(e)) return;
    e.preventDefault();
    e.stopPropagation();
    const before = dragBefore;
    clearDropMarks();
    const src = srcOf(e);
    const t = dropTargetOf(e);
    if (!src || !t) return;
    const head = t.node.tagName === 'H2';
    // **自己拖到自己身上**：这一行不动，也不发请求。
    if (!head && src.id === t.node.dataset.item) return;
    const dst = head ? t.node.dataset.group : groupOf(t.node).dataset.gid;
    if (!head) {
      const sibs = itemsOf(groupOf(t.node));
      let index = sibs.indexOf(t.node) + (t.after ? 1 : 0);
      // 同组往下挪时，被拖的那一条**还算在列表里**：直接落到目标下标会插到
      // 它后面去（想放第 3 位，结果停在第 4 位）。取走它之后的位置才是答案。
      const from = sibs.findIndex((x) => x.dataset.item === src.id);
      if (src.gid === dst && from >= 0 && from < index) index -= 1;
      // **path 指源组、to 指目标组**。从前两个都填了目标组——同组时两者相等，
      // 于是那条 bug 在同组下完全隐形，一跨组就变成"那个组里没有这条"，整批不写。
      if (!await applyOps([{ op: 'move', path: `groups/${src.gid}/items/${src.id}`, to: dst, index }], 'app.ops.move')) return;
      moveInPlace(dst, index, before);
    } else {
      // **不给 index**：服务端 move 在 index 缺席时就是"追加到目标组末尾"
      // （ops.go 里 pos = bound）。自己再算一个下标，等于把"末尾"重算一遍——
      // 而队尾那个 bug 正是这么来的。
      if (!await applyOps([{ op: 'move', path: `groups/${src.gid}/items/${src.id}`, to: dst }], 'app.ops.move')) return;
      moveInPlace(dst, null, before);
    }
  });
  // 拖起来的那一位松开（或拖出窗外）时的收尾：清标记、清这一次拖的状态。
  // 从前这三条挂在每一行上，而一次拖动只会有一个 dragend。
  dragRoot.addEventListener('dragend', () => { dragSrc = null; dragBefore = null; clearDropMarks(); });

  // 组级落点（拖到组标题上 = 追加到那一组末尾，2026-10-15 建的）现在并进上面那个
  // 委托块了：`dropTargetOf` 里 `h2[data-group]` 那一支就是它。
  //
  // 为什么当初需要这一级：从前唯一的落点是某一条，于是「挪到队尾」根本没有出口
  // （插到任何一条前面都行，插到任何一条后面都不行），而**空的组一次都落不进去**
  // ——它里面一个 li 都没有。组标题是每一组都有的那一件（正文 section 与大卡带
  // 都是 h2[data-group]），所以它当队尾落点。

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
    const name = li.querySelector('b')?.textContent || '';
    // 地址从**这一行本来显示它的那个元素**上读，三种形态各有一个载体，判据只有
    // `itemURL` 一处（大卡带里点不动的外部地址那一行屏上不印地址，模板写在 data-addr 上——
    // 从前这里按 href 读，读成空串就会照发一条 `set …/url=""`，整批 422）。
    const url = itemURL(li);
    // 说明从 `data-desc` 读，**不读 `<i>`**（2026-10-24 修）：没有说明的卡片
    // 那一格印的是主机名（`<i class="host">`），拿它当说明会在下一次保存时把
    // example.com 写进空说明里——用户会在历史里看到一条自己没做过的"改说明"。
    const desc = li.dataset.desc || '';
    const group = groupOf(li);
    const gid = group?.dataset.gid || '';
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
    // 分组那一格（2026-10-16）。这里从前是一句「这一条在「应用」里」——它说得出
    // 在哪一组，却改不了。把一条书签挪到别组在界面上只剩两条路：拖（只有鼠标
    // 做得顺手）与多选里的「移动到」（要先长按卡片，而那个暗示在屏幕上看不见）。
    // 把那句话换成一颗能挑的，"说清在哪一组"这个职责一个字没少，多的正是那个
    // 本来没有出口的"能配"。
    const gs = el('select', { id: 'lb-edit-group', class: 'fld' });
    for (const g of groupsOnPage()) {
      const o = el('option', { value: g.dataset.gid, text: groupLabel(g.dataset.gid, groupName(g)) });
      if (g.dataset.gid === gid) o.selected = true;
      gs.append(o);
    }

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
          // **没改的字段不发**（2026-10-24 修）：从前名字、说明、地址三格永远发，
          // 于是打开一条按「保存」什么都不改，也会写一版快照、rev +1，并在历史
          // 里留下"改名 / 改说明 / 改地址"三条假动作。没改就没改，与分组那一族
          // 「没改就不算一次改名」同一判据。
          const fields = {};
          const nextName = n.value.trim() || name;
          if (nextName !== name) fields.name = nextName;
          if (d.value !== desc) fields.desc = d.value;
          if (u.value.trim() !== url) fields.url = u.value.trim();
          if (ic.value !== curIcon) fields.icon = ic.value;
          const ops = opsFor(li, fields);
          // 挪组排在改字段**之后**：那些 set 走的是现在这一组的路径，先挪就把
          // 它们指到还没有的那一组上去（ops 在锁内按顺序应用，顺序就是语义）。
          if (gs.value && gs.value !== gid) {
            ops.push({ op: 'move', path: `groups/${gid}/items/${li.dataset.item}`, to: gs.value, index: 0 });
          }
          // 空批也会写一版 rev：打开看看再关掉，不该在「回退」里多一条。
          if (!ops.length) { closeEdit(); return; }
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
        el('label', { text: T('app.field.title') }, [n]), el('label', { text: T('app.field.desc') }, [d]), el('label', { text: T('app.field.url') }, [u]),
        el('label', { text: T('app.field.group') }, [gs]),
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
  // ops.go 那边校验只收 tiles/list/feature —— **feature 是第三档**：那一组不再出现在
  // 正文里，而是被渲染层挪到页面顶部独占一块，见 model.LayoutFeature），
  // 页面上一个入口都没有——而"哪一组用磁贴、
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
    const mk = (label, fn, cls) => bar.append(el('button', { type: 'button', class: cls ? 'fld ' + cls : 'fld', text: label, on: { click: fn } }));
    mk(T('app.multi.move', n), pickMoveTarget);
    mk(T('app.multi.tags', n), pickTags);
    // 删掉那一颗要**看着就不一样**（2026-10-15）。五颗键同底同边同字色，而其中
    // 只有一颗通向"这一批没了"。它不是实心红——实心红留给"按下去就不可逆"的
    // 确认页那一颗（button.danger），这一颗只是打开确认，与表格里「删档」和
    // 设置里「删掉这一组」同一档：描边转红。
    mk(T('app.multi.delete', n), pickDelete, 'danger');
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
  // **绑在 bindItem 里，不在这里另起一轮**（2026-10-16 横扫时撞出来的）：这一族从前的
  // 写法是「页面加载时扫一遍 li，逐个挂上」，而**拖进来的那一行是后来接进 DOM 的**
  // （`spliceCardIn` 只补挂了把手/复制/拖动）。于是拖进来的书签**长按不进多选**，
  // 而在多选里点它也不切换——症状是"这一条和别的行为不一样"，且没有任何报错。
  // 一行该有哪些监听，从此只有 bindItem 一个出口。
  function bindMulti(li) {
    if (li.dataset.lbMultiBound) return;
    li.dataset.lbMultiBound = '1';
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
  // 页面加载时每一行都走 `bindItem`（它在前面跑，而 `bindMulti` 是函数声明、会提升），
  // **这里不再另扫一轮**：两轮绑定就是"同一个判断有两处出口"的那种形状，下一次加
  // 监听的人只会改其中一处。

  const pickedItems = () =>
    [...document.querySelectorAll('li[data-item]')].filter((li) => multi.picked.has(li.dataset.item));

  async function pickMoveTarget() {
    const picked = pickedItems();
    if (!picked.length) return;
    // **每一组都要在**：[data-gid] 而不是 section[data-gid]——大卡带的组容器是
    // ul.feat-row，写成 section 时「移动到」那一份名单里**根本没有大卡组**。
    const groups = groupsOnPage();
    const dlg = el('div', { class: 'sheet', role: 'dialog', 'aria-modal': 'true', 'aria-label': T('app.move.aria') }, [
      el('h2', { text: T('app.move.title', picked.length) }),
      el('div', { class: 'sheet-actions' }, groups.map((g) => el('button', {
        type: 'button', class: 'fld', text: groupLabel(g.dataset.gid, groupName(g)),
        on: {
          click: async () => {
            dlg.remove(); // 从前是 dlg.hidden = true：节点留在 DOM 里，第二次进多选会攒出第二个同名 dialog
            const to = g.dataset.gid;
            // **不给 index**（2026-10-16 查排序时发现）：ops 在锁内按顺序应用，
            // 缺席即追加到末尾（ops.go 里 pos = bound）。从前每一条都写 `index: 0`
            // ——于是第 2 条把第 1 条顶到后面、第 3 条再顶前两条，选三条搬过去
            // 落成的顺序是**选中顺序整个反过来**。多选搬组本来就是一次搬一批，
            // 批内的先后是屏上那一列从上到下的顺序，不该被倒过来。
            const ops = picked.map((li) => ({
              op: 'move',
              path: `groups/${groupOf(li).dataset.gid}/items/${li.dataset.item}`,
              to,
            }));
            if (await applyOps(ops, 'app.ops.bulkMove')) reloadKeepingPlace();
          },
        },
      }))),
      el('div', { class: 'sheet-actions' }, [el('button', {
        type: 'button', class: 'fld', text: T('app.move.cancel'), on: { click: () => dlg.remove() },
      })]),
    ]);
    // 记住是谁打开它的（2026-10-20）：关掉时把焦点还回去。
    dlg.__opener = document.activeElement;
    body.append(dlg);
    // **这一层也要 syncTrap()**（2026-10-20）：它是 role=dialog aria-modal=true，
    // 可没被摘进 inert 的话，背景页照样能聚焦、能滚动，Tab 从它身上直接走到
    // 背后——aria-modal 只是一句空声明。打开浮层/台面/⌘K/图标库那几处都记得调，
    // 这三处原先漏了。
    syncTrap();
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
                path: `groups/${groupOf(li).dataset.gid}/items/${li.dataset.item}/tags`,
                value: list,
              }));
              if (await applyOps(ops, 'app.ops.bulkTags')) reloadKeepingPlace();
            },
          },
        }),
        el('button', { type: 'button', class: 'fld', text: T('app.generic.cancel'), on: { click: () => dlg.remove() } }),
      ]),
    ]);
    dlg.__opener = document.activeElement;
    body.append(dlg);
    syncTrap();
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
                path: `groups/${groupOf(li).dataset.gid}/items/${li.dataset.item}`,
              }));
              if (await applyOps(ops, 'app.ops.bulkDelete')) reloadKeepingPlace();
            },
          },
        }),
        el('button', { type: 'button', class: 'fld', text: T('app.generic.cancel'), on: { click: () => dlg.remove() } }),
      ]),
    ]);
    dlg.__opener = document.activeElement;
    body.append(dlg);
    syncTrap();
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
    //
    // **外部条目与 Enter 那一支同判**（2026-10-19 修）：这一处读的是同一份
    // `cmdkHits`、同一个 `cmdkSel`，所以它就是 `cmdkRun` 的第二个入口。
    // 修那一支时漏了它，于是同一个条目**按回车复制地址、按 ⌘↵ 照样跳**——
    // 两个键差一个字，行为却相反，而"点不动的地址"那个问题原样还在。
    // 判据共用 hit.external，不在这里重问一遍协议。
    if (mod(e) && e.key === 'Enter' && !cmdk.hidden) {
      e.preventDefault();
      const hit = cmdkHits[cmdkSel];
      if (hit && hit.url && !hit.external) window.open(hit.url, '_blank', 'noopener');
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
      // **开关都要 syncTrap()**（2026-10-20）：这一层是 role="dialog"
      // aria-modal="true"，可它没被摘进 inert 的话，背景页照样能聚焦、
      // 能滚动，Tab 从它身上直接走到背后——aria-modal 只是一句空声明。
      // 打开时要把 ⌘K 让开（下面那一支），关掉时要把背景还回来。
      syncTrap();
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
  //
  // **kind 决定它退不退**（2026-10-16）：`'ok'` 是**成了**，会自己消失；`'bad'` 是
  // **没成**，留在屏上等人按——§8.7「写失败一律显式报错」，一条自己走掉的失败提示
  // 等于把"刚才没存进去"变成"什么都没发生"；不给是中性提示（离线、多选、有新
  // 版本），既不是成了也不是没成，就别自己动。判据不在重不重要，在**它是不是
  // 一次结果**。
  const bannerLife = 4600; // 够读完一句中文的两倍长；数字在 CSS 那条细线上也要用
  let bannerTimer = 0;
  function showBanner(text, kind) {
    const old = $('#lb-banner');
    if (old) old.remove();
    clearTimeout(bannerTimer); // 上一条还没走完的倒计时不能替这一条续命
    const cls = 'notice' + (kind === 'bad' ? ' bad' : kind === 'ok' ? ' ok' : '');
    // **role 按 kind 选**（2026-10-20）："没存进去"（bad）是** assertive**——
    // 读屏要立刻念出来；其余（中性提示、成了）是 polite，等个空隙再念。
    // 从前一律 role="status"（polite），于是"刚才没存进去"可能被安静地略过，
    // 而同一文件里字段错误那一条用的是 role="alert"——两套播报词汇。
    const p = el('p', { class: cls, id: 'lb-banner', role: kind === 'bad' ? 'alert' : 'status' }, [
      el('span', { text }),
      el('button', { type: 'button', class: 'dismiss', text: T('app.sw.dismiss'), 'aria-label': T('app.sw.aria'),
        on: { click: () => p.remove() } }),
    ]);
    if (kind === 'ok') armBanner(p);
    // 横幅落在**顶栏之后**，不落进顶栏里（2026-10-15 取景后改判）。
    // 原先是 `header.prepend`：header 是 flex 行，于是"已保存。""离线了"
    // "已经在多选里"每一条都成了标题的**兄弟**，把「我的书签」与搜索框整排
    // 往右推（实拍：多选提示 600px 宽，标题被顶到 x=645）。一条会自己出现又
    // 自己消失的临时消息，不该每出现一次就重排一次主结构。
    header.after(p);
  }

  // 自己会走的那一条：倒计时 + **指针移上去或焦点落进去就暂停**——一条正在读的
  // 字在读完之前消失是这一族最容易犯的错，而键盘用户没法"移到上面"，所以两路都挂。
  function armBanner(p) {
    let left = bannerLife;
    let t = 0;
    const tick = () => {
      left -= 50;
      p.style.setProperty('--wait', String(Math.max(0, left / bannerLife)));
      if (left <= 0) { p.remove(); return; }
      t = setTimeout(tick, 50);
    };
    const pause = () => clearTimeout(t);
    const resume = () => { if (p.isConnected) { clearTimeout(t); tick(); } };
    p.addEventListener('pointerenter', pause);
    p.addEventListener('focusin', pause);
    p.addEventListener('pointerleave', resume);
    p.addEventListener('focusout', resume);
    tick();
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

  // 备份导入成功后重载的那一页：补一句成功话（浮层里那条已随重载消失）。
  //
  // **必须在文件末尾**：showBanner 是函数声明、会提升，但它读的 bannerLife 是
  // 一个 const——放在文件前半段调用时那一格还在 TDZ 里，异常被 catch 吞掉，
  // 于是"重载了、设置生效了，却没有成功话"。这条实测抓到的就是它（2026-10-24）。
  try {
    if (sessionStorage.getItem(RESTORE_OK)) {
      sessionStorage.removeItem(RESTORE_OK);
      showBanner(T('app.backup.imported'), 'ok');
    }
  } catch (e) {
    // 存不下就不说，独立页那条蓝色提示仍在；但别静默吞掉真正的脚本错误。
    console.warn('lb: restore banner failed', e);
  }
})();