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
    "app.field.title": "名称",
    "app.field.desc": "描述",
    "app.field.group": "分组",
    "app.field.icon": "图标",
    "app.ph.title": "网站标题，可改",
    "app.ph.desc": "一句话说明，可不填",
    "app.ph.group": "新分组叫什么",
    "app.icon.none": "不选图标（用名字第一个字）",
    "app.icon.auto": "抓这个网站的图标",
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
    "app.cmdk.none": "没有叫「%s」的书签，也没有对得上的动作。",
    "app.cmdk.empty": "这里还没有可列的东西。",
    "app.clip.denied": "这个浏览器不给读剪贴板，手动贴一下地址。",
    "app.clip.noturl": "剪贴板里那段不像一个网址，手动贴一下。",
    "app.clip.noread": "读不到剪贴板（系统没给这个权限），手动贴一下地址。",
    "app.copy.aria": "复制「%s」的链接",
    "app.copy.done": "已复制",
    "app.copy.fail": "复制不了，已经帮你把地址选中了，按 Ctrl+C",
    "app.edit.aria": "改「%s」",
    "app.edit.fallbackName": "这一条",
    "app.layout.tiles": "磁贴",
    "app.layout.rows": "单列",
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
    "app.keys.q": "打开这份清单",
    "app.keys.long": "进多选：一次移动或打标签好几条",
    "app.keys.arrows": "在面板里选择",
    "app.keys.esc": "关掉当前浮起来的那一层",
    "app.keys.escAll": "Esc 关掉这一页",
    "app.sw.dismiss": "知道了",
    "app.sw.aria": "关掉这条提醒",
    "app.sw.updated": "书签页面已更新。点刷新用上新的。",
    "app.generic.cancel": "取消",
    "app.generic.save": "存",
    "app.ops.item": "这一条",
    "app.ops.move": "移动",
    "app.ops.layout": "切换布局",
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
    "app.offline.aria": "Show entries queued while offline",
    "app.offline.drop": "Drop this one",
    "app.offline.list": "These were saved while offline and will go through once you are back online.",
    "app.offline.blocked": "No network, so this is read-only for now. Additions are queued and sent automatically when you are back.",
    "app.offline.done": "%d queued entries have been sent",
    "app.offline.dropped": "%d entries clashed with someone else's changes and were not merged automatically. Please add them again: %s",
    "app.offline.left": "%d entries still not saved: %s",
    "app.saving": "Saving…",
    "app.saved": "Saved.",
    "app.conflict": "This profile changed underneath you (something else may have edited it). Refresh and try again.",
    "app.cmdk.placeholder": "Search bookmarks, or do something",
    "app.cmdk.more": "Search the whole page for “%s” · %d matches in total",
    "app.cmdk.aria": "Command palette",
    "app.cmdk.clipboard": "Save the address on the clipboard",
    "app.cmdk.history": "See this profile’s change history",
    "app.cmdk.kind": "action",
    "app.cmdk.none": "No bookmark named “%s”, and no matching action either.",
    "app.cmdk.empty": "There is nothing to list yet.",
    "app.clip.denied": "This browser will not let the page read the clipboard. Paste the address yourself.",
    "app.clip.noturl": "What is on the clipboard does not look like an address. Paste it yourself.",
    "app.clip.noread": "Could not read the clipboard (the system did not grant permission). Paste the address yourself.",
    "app.copy.aria": "Copy the link for %s",
    "app.copy.done": "Copied",
    "app.copy.fail": "Could not copy it — the address is selected now, press Ctrl+C",
    "app.edit.aria": "Edit “%s”",
    "app.edit.fallbackName": "this entry",
    "app.layout.tiles": "Tiles",
    "app.layout.rows": "Single column",
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
    "app.tags.title": "Tag these %d entries",
    "app.tags.label": "Tags",
    "app.tags.ph": "Separate them with spaces or commas",
    "app.tags.empty": "The tags are empty. Write at least one.",
    "app.keys.aria": "Keyboard shortcuts",
    "app.keys.title": "Keyboard shortcuts",
    "app.keys.k": "Open the command palette",
    "app.keys.enter": "In the palette, open the selected bookmark in a new tab",
    "app.keys.slash": "Focus the search box",
    "app.keys.n": "Open the add sheet",
    "app.keys.q": "Open this list",
    "app.keys.long": "Enter multi-select: move or tag several at once",
    "app.keys.arrows": "Move the selection in the palette",
    "app.keys.esc": "Close the topmost open layer",
    "app.keys.escAll": "Esc closes this panel",
    "app.sw.dismiss": "Got it",
    "app.sw.aria": "Dismiss this notice",
    "app.sw.updated": "The bookmark page has been updated. Refresh to use it.",
    "app.generic.cancel": "Cancel",
    "app.generic.save": "Save",
    "app.ops.item": "this entry",
    "app.ops.move": "the move",
    "app.ops.layout": "the layout change",
    "app.ops.bulkMove": "the bulk move",
    "app.ops.bulkTags": "the bulk tags",
    "app.ops.delete": "delete",
    "app.ops.bulkDelete": "the bulk delete",
    "app.edit.where": "This entry is in “%s”",
    "app.delete.item": "Delete this bookmark",
    "app.delete.confirm": "Delete “%s”? Press again to delete it",
    "app.delete.bulk": "Delete these %d bookmarks?",
    "app.delete.confirmMany": "Delete %d bookmarks",
    "app.delete.more": "…and %d more not listed",
    "app.delete.aria": "Bulk delete",
    "app.delete.undoHint": "You can bring this version back from “Revert” in the top bar",
    "app.applyOps.fail": "%s could not be saved: %s",
    "app.opsConflict": "This profile changed underneath you. Refresh and try again.",
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

  // ── 小工具 ────────────────────────────────────────────────────────────────
  const $ = (sel, root = document) => root.querySelector(sel);
  const el = (tag, attrs = {}, kids = []) => {
    const n = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs)) {
      if (k === 'text') n.textContent = v; // **永远走 textContent**：书签名、
      // desc、标题都是用户或第三方给的，innerHTML 会把它们当标记解析。
      else if (k === 'on') for (const [ev, fn] of Object.entries(v)) n.addEventListener(ev, fn);
      else if (v !== '' && v != null) n.setAttribute(k, v);
    }
    for (const c of [].concat(kids)) n.append(c);
    return n;
  };
  const say = (msg, kind = 'info') => {
    if (!msg) { status.textContent = ''; status.className = 'sheet-status'; return; }
    status.textContent = msg;
    status.className = 'sheet-status ' + kind;
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
    const top = [...document.querySelectorAll('#lb-sheet, .cmdk, .sheet')].filter((n) => !n.hidden).pop();
    for (const el of document.body.children) {
      if (!top || el === top) el.removeAttribute('inert');
      else el.setAttribute('inert', '');
    }
  }

  function openSheet() {
    sheet.hidden = false;
    syncTrap();
    say('');
    fUrl.focus();
  }

  const openBtn = el('button', {
    type: 'button', id: 'lb-add', class: 'add', text: T('app.add'), title: T('app.addTitle'),
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
  const fIcon = el('select', { id: 'lb-icon', class: 'fld' });

  const status = el('p', { class: 'sheet-status', role: 'status', 'aria-live': 'polite' });
  const saveBtn = el('button', { type: 'submit', class: 'fld', text: T('app.save') });
  const moreBtn = el('button', { type: 'button', class: 'fld more', text: T('app.saveMore'), on: { click: () => submit(true) } });

  const form = el('form', {
    on: { submit: (e) => { e.preventDefault(); submit(false); } },
  }, [
    el('label', { text: T('app.field.url') }, [fUrl]),
    // 字段名与就地编辑统一成「名称」（数据里就是 name）。这里从前叫「标题」、
    // 就地编辑叫「名字」——同一个东西在两层里两个名字。
    el('label', { text: T('app.field.title') }, [fTitle]),
    el('label', { text: T('app.field.desc') }, [fDesc]),
    el('label', { text: T('app.field.group') }, [fGroup, fNewGroup]),
    el('label', { text: T('app.field.icon') }, [fIcon]),
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

  // 分组下拉的选项来自**服务端已经渲染在页面上的分组**。不重新拉一次 /api/v1/data：
  // 一次浮层开合不该为了拿一份没变的数据多发一个请求，而它还会带来"两处分组列表"
// 迟早不同步的那一类 bug。
  for (const h2 of document.querySelectorAll('main h2[data-group]')) {
    fGroup.append(el('option', { value: h2.dataset.group, text: h2.textContent.trim() }));
  }
  // 这里从前在末尾追加一个「（不分组）」空值选项，而 submit() 判到空值就报
  // "先选一个分组——这个版本的分组是必选的"：**界面摆了一条走不通的路**。
  // 现在没有分组可挑的档案（刚建好、一条书签都没有）改为直接问"新分组叫什么"，
  // 一次 batch 里先建组再放条目——两件事本来就属于同一个意图。
  fNewGroup.hidden = fGroup.options.length > 0;
  if (!fGroup.options.length) fGroup.hidden = true;

  // 图标下拉在 resolve 之前是**空的**（零选项的下拉框）：人看不到能选什么，
  // 也选不到"抓这个网站的图标"。两个语义值常驻，resolve 回来的候选再插进去。
  function iconChoices(candidates) {
    fIcon.innerHTML = '';
    fIcon.append(
      el('option', { value: '', text: T('app.icon.none') }),
      el('option', { value: 'auto', text: T('app.icon.auto') }),
    );
    for (const c of candidates || []) {
      fIcon.append(el('option', { value: c, text: c.replace(/^[a-z-]+:/, '') }));
    }
  }
  iconChoices([]);

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
    say(T('app.reading'), 'busy');
    try {
      const r = await api('/api/v1/resolve', { url, profile });
      if (r.detail || r.code) { say(r.detail || T('app.readFail'), 'bad'); return; }
      if (!fTitle.value) fTitle.value = r.title || '';
      if (!fDesc.value) fDesc.value = r.desc || '';
      if (r.suggestedGroup && !fGroup.hidden) {
        for (const o of fGroup.options) {
          if (o.value === r.suggestedGroup) { fGroup.value = r.suggestedGroup; break; }
        }
      }
      iconChoices(r.iconCandidates);
      say(r.title ? T('app.readOk') : T('app.readEmpty'), r.title ? 'ok' : 'warn');
    } catch (e) {
      // **写失败一律显式报错，不静默吞**（§8.7）：一条"正在读取…"永远停在那儿，
      // 用户会以为它在加载。
      say(T('app.readErr', e.message), 'bad');
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
  function slug(text, fromURL) {
    if (fromURL) {
      let base = '';
      try {
        const u = new URL(text);
        base = u.hostname.replace(/^www\./i, '').split('.')[0] + '-' +
               (u.pathname.split('/').filter(Boolean).pop() || '');
      } catch { base = ''; }
      return ascii(base) || ascii(text) || hash36(String(text)) || 'item';
    }
    return ascii(text) || 'g-' + hash36(String(text));
  }

  // 分组这一格：有分组可挑就挑一个；一份分组都没有（刚建好的档案）就用
  // 「新分组叫什么」那一格，把建组与放条目放进**同一批** ops——batch 是全成
  // 或全败的，不会出现"组建了条目没进去"。
  function groupOpsAndTarget() {
    if (!fGroup.hidden) {
      const gid = fGroup.value;
      if (!gid) return { err: T('app.err.group') };
      return { gid, ops: [] };
    }
    const title = fNewGroup.value.trim();
    if (!title) return { err: T('app.err.groupName') };
    const gid = slug(title);
    return { gid, ops: [{ op: 'add', path: 'groups', value: { id: gid, title, layout: 'tiles', items: [] } }] };
  }

  async function submit(keepOpen) {
    const url = fUrl.value.trim();
    if (!url) { say(T('app.err.url'), 'bad'); return; }
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
      // batch 的 422 说的是"ops 校验未过，整批未写入"——那是**协议话**，真正的原因
      // 在 errors[0].reason（"条目 x 的图标名不在索引里"这种）。只报 detail 等于
      // 告诉人"失败了"而不说为什么失败。
      const first = Array.isArray(data.errors) && data.errors[0];
      const why = (first && (first.reason || first.field)) || data.detail || `HTTP ${res.status}`;
      const e = new Error(why);
      e.status = res.status;
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
      if (!q || name.includes(q) || url.includes(q)) {
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
      showBanner(T('app.applyOps.fail', T(what), e.status === 409 ? T('app.opsConflict') : e.message));
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
    card.append(pencil);
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

  // 就地编辑：三个字段 + 存 / 取消 / 删掉这条。
  //
  // 注释先前写"改完就写，不摆保存大按钮（失焦即写那条裁定见 §8.4）"——那是**旧的**
  // 交互，代码里早就换成「存 / 取消」两颗按钮（失焦即写在"改一半点到别处"时会
  // 静默落库，而这一格没有第二次机会反悔）。注释替不存在的实现说话，比没注释更坏，
  // 所以这一句改口成它今天的样子。
  //
  // **要有关掉它的出路**：从前这一格只有一个「存」，按 Esc 什么都不做
  // （Esc 只关浮层与面板），再点 ✎ 也被 `if (form) return` 挡掉——
  // 手滑点开的人只能刷新页面才能退出。
  function closeEdit(li) {
    const f = li.querySelector('.inline-edit');
    if (!f) return;
    const pen = li.querySelector('.pencil');
    f.remove();
    pen?.focus();
  }

  function editItem(li) {
    if (li.querySelector('form')) return;
    const a = li.querySelector('.card');
    const name = li.querySelector('b')?.textContent || '';
    // 地址从**这一行本来显示它的那个元素**上读：链接形态读 href，点不动的那一行
    // 读 <code> 的文本（模板与复制键读的是同一处，不另抄一份 URL）。
    const url = a ? (a.getAttribute('href') || '') : (li.querySelector('code')?.textContent || '').trim();
    const desc = li.querySelector('i')?.textContent || '';
    const sec = li.closest('section');
    const gid = sec?.dataset.gid || '';
    // 分组那一行不是装饰：面板盖住卡片之后，"我在改哪一组的哪一条"只剩这里能说，
    // 而删除那一步的确认句要引用它。
    const gname = groupName(sec);
    const f = (k, v) => el('input', { type: 'text', class: 'fld', name: k, value: v, autocomplete: 'off' });
    const n = f('name', name), d = f('desc', desc), u = f('url', url);
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
      class: 'inline-edit',
      on: {
        submit: async (e) => {
          e.preventDefault();
          const ops = opsFor(li, { name: n.value.trim() || name, desc: d.value, url: u.value.trim() });
          if (await applyOps(ops, 'app.ops.item')) reloadKeepingPlace();
        },
        // 这一层浮在卡片之上，Esc 该先关它、回到 ✎，而不是把底下的浮层一起关掉。
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
          closeEdit(li);
        },
      },
    }, [el('p', { class: 'edit-where', text: T('app.edit.where', gname) }),
        el('label', { text: T('app.field.title') }, [n]), el('label', { text: T('app.field.desc') }, [d]), el('label', { text: T('app.field.url') }, [u]),
        el('div', { class: 'row' }, [
          el('button', { type: 'submit', class: 'fld', text: T('app.generic.save') }),
          el('button', { type: 'button', class: 'fld', text: T('app.generic.cancel'), on: { click: () => closeEdit(li) } }),
        ]),
        el('div', { class: 'row' }, [del]),
        undo]);
    undo.hidden = true;
    li.append(form);
    // 面板宽是"至少 300px"（放得下一条完整 URL），而最后一列的磁贴只有 213px 宽——
    // 实测那颗右边缘落在 1475 而视口是 1440：**35px 悬在屏幕外**，「取消」与
    // 「删掉这条」那半截看不见也点不到。量一次，出屏就改成贴着卡片右边缘长出来。
    const r = form.getBoundingClientRect();
    if (r.right > innerWidth - 8) form.classList.add('flip');
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
      // 一层一层关：先关最上面浮起来的那一个。顺序反了的话，在浮层里按 Esc
      // 会连浮层和面板一起关掉——那是一次按键毁掉两个状态。
      //
      // 多选排第一：进多选时那句提示明写「按 Esc 退出」，而从前这一支里
      // 根本没有 exitMulti——说了没做。就地编辑的表单比它更靠上层，先关它。
      const editing = document.querySelector('.inline-edit');
      if (editing) { closeEdit(editing.closest('li')); return; }
      if (multi.on) { exitMulti(); return; }
      const dlg = [...document.querySelectorAll('.sheet')].find((x) => x !== sheet);
      if (dlg) { dlg.remove(); return; }
      for (const n of [help, cmdk, sheet]) {
        if (!n.hidden) { n.hidden = true; syncTrap(); return; }
      }
      const banner = $('#lb-banner');
      if (banner) banner.remove();
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