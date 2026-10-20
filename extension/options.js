// options.js —— 扩展的设置页。
//
// 它把令牌放在 chrome.storage.local（**不是 sync**）：sync 跟着浏览器账号上云，
// 而那是这台机器的写权限。这一页里没有任何"导出配置"按钮，也正是同一个理由。
//
// 页面上**一句中文都不写**：每一句都住在 `_locales/*/messages.json`，按浏览器
// 界面语言取（`i18n.js` 的 `t`）。`bg.js` 与这一页共用同一批 key——同一件事在
// 同一条流水线上不该有两种说法。
import { t } from './i18n.js';
import { ensureBookmarksPermission, buildNetscape, askPlan, applyPlan } from './import.js';

const KEY = 'lb-config';

const rows = document.getElementById('rows');
const msg = document.getElementById('msg');

// applyI18n：页面的话**全部来自目录**，HTML 里不另留一份中文兜底——留了就等于
// 同一条句子有两处可改，而这两处一定会分家（主站那边的模板同样是 `{{.T "key"}}`，
// 兜底不住）。`data-i18n-rich` 那三节带 <b> 与 <code>，它们才用 innerHTML：
// 填进去的是仓库里的静态文本，没有一个字节来自用户输入或来自那台机器，
// 而允许的标签集合由 internal/i18n/extcatalog_test.go 钉死。
function applyI18n() {
  // lang 也一起改：读屏软件按它选发音，页面翻成英文而 lang 还是 zh-CN，
  // 那些英文句子就会被按中文音节念出来。
  document.documentElement.lang = chrome.i18n.getUILanguage();
  document.title = t('pageTitle');
  for (const el of document.querySelectorAll('[data-i18n]')) el.textContent = t(el.dataset.i18n);
  for (const el of document.querySelectorAll('[data-i18n-rich]')) el.innerHTML = t(el.dataset.i18nRich);
}

async function load() {
  const got = await chrome.storage.local.get(KEY);
  const c = { base: '', tokens: {}, defaultProfile: '', ...(got[KEY] || {}) };
  document.getElementById('base').value = c.base;
  document.getElementById('prof').value = c.defaultProfile;
  renderRows(c);
}

function renderRows(c) {
  rows.innerHTML = '';
  const ids = [...new Set([...Object.keys(c.tokens), c.defaultProfile].filter(Boolean))];
  if (!ids.length) ids.push('');
  for (const id of ids) rows.append(tokenRow(id, c.tokens[id] || ''));
  const add = document.createElement('button');
  add.type = 'button';
  add.textContent = t('addRow');
  // **追加一行，不重建整表**。旧写法是 `renderRows({tokens:{}})`，于是点一下
  // "再加一枚"会把已经填好的那几行连字带行全部清空——用户看到的是"我刚才填的
  // 令牌没了"，而这一页没有任何撤销。空表重建还会顺手塞一行 'dad'，
  // 那是一枚别人机器上多半不存在的档案 id。
  add.onclick = () => { rows.insertBefore(tokenRow('', ''), add); rows.querySelector('input[type=text]:last-of-type')?.focus(); };
  rows.append(add);
}

function tokenRow(id, token) {
  const row = document.createElement('div');
  row.className = 'row';
  row.style.marginTop = '10px';
  const name = document.createElement('input');
  name.type = 'text';
  name.value = id;
  // placeholder 用的是列头那同一句：窄窗口那一档列头撤掉了（见 options.html 的
  // @media 520），这一格得自己说得出自己是什么，而不能靠上面那行小字。
  name.placeholder = t('colId');
  name.setAttribute('aria-label', t('colId'));
  const tok = document.createElement('input');
  tok.type = 'password';
  tok.value = token;
  tok.placeholder = t('tokenPlaceholder');
  tok.setAttribute('aria-label', t('tokenAria', id || t('thisProfile')));
  tok.autocomplete = 'off';
  tok.dataset.profile = id;
  const del = document.createElement('button');
  del.type = 'button';
  del.textContent = t('deleteRow');
  // 删掉就是**从待保存的表里移除**：save() 只从现存 DOM 收集，所以点完再保存才生效
  // （和"填了不保存不算数"是同一条规矩）。旧代码在这里往 dataset 上写一个
  // `__del__<id>` 的标记，而 save() 遍历的是 `rows.querySelectorAll` —— 那一行
  // 早就被 row.remove() 摘掉了，标记没人看得见：半套机制，删了。
  del.onclick = () => { row.remove(); say(t('rowRemovedNote'), ''); };
  row.append(name, tok, del);
  return row;
}

async function save() {
  const tokens = {};
  // 一行一读：档案 id 与它的令牌**住在同一个 row 里**，所以按行配对。
  // 旧写法先把令牌按 dataset 收一遍，再靠 `indexOf(inp)` 的位置去猜"这一行
  // 的名字框是第几个"——两趟循环 + 位置耦合，中间任何一行少个输入框就会串位，
  // 把 A 档案的令牌存到 B 档案名下（那是往别人的台子里写）。
  for (const row of rows.querySelectorAll('.row')) {
    const name = row.querySelector('input[type=text]');
    const tok = row.querySelector('input[type=password]');
    if (!name || !tok) continue;
    const id = name.value.trim(), v = tok.value.trim();
    if (id && v) tokens[id] = v;
  }
  const c = {
    base: document.getElementById('base').value.trim().replace(/\/+$/, ''),
    defaultProfile: document.getElementById('prof').value.trim(),
    tokens,
  };
  await chrome.storage.local.set({ [KEY]: c });
  say(t('saved'), 'ok');
}

function say(text, cls) {
  msg.textContent = text;
  msg.className = cls || '';
}

// 「试一下这台机器」测的是**用户真正要成立的那件事**：这枚令牌能不能打开这个
// 档案。旧写法打的是 `GET /api/v1/profiles`（管理员端点），而家里绝大多数人
// 只有档案作用域的令牌——于是这个诊断按钮对**配置完全正确**的人永远回一句
// 红色的"权限不够"，把人对着一个假故障查半天。
async function test() {
  const c = await collect();
  if (!c.base) return say(t('needBase'), 'bad');
  const id = c.defaultProfile || Object.keys(c.tokens)[0];
  const token = c.tokens[id];
  if (!id || !token) return say(t('needToken'), 'bad');
  try {
    const r = await fetch(c.base + '/api/v1/data?profile=' + encodeURIComponent(id), {
      headers: { Authorization: 'Bearer ' + token },
    });
    // 401/404 这两句与 bg.js 的通知**共用同一批 key**：同一件事在同一条流水线上
    // 不该有两种说法（那儿是"点了没反应"，这儿是"点了，看看答什么"）。
    if (r.ok) return say(t('testOK', id), 'ok');
    if (r.status === 403) return say(t('test403', id), 'bad');
    if (r.status === 401) return say(t('err401'), 'bad');
    if (r.status === 404) return say(t('err404', id), 'bad');
    return say(t('testOther', r.status), 'bad');
  } catch (e) {
    return say(t('netFail', e.message), 'bad');
  }
}

async function collect() {
  const got = await chrome.storage.local.get(KEY);
  return { base: '', tokens: {}, defaultProfile: '', ...(got[KEY] || {}) };
}

document.getElementById('save').onclick = save;
document.getElementById('test').onclick = test;

// ── 从浏览器书签树导入（import.js 提供读树、拼 Netscape 与两次请求）────────
const bmReport = document.getElementById('bmreport');
const bmApply = document.getElementById('bmapply');
const bmCancel = document.getElementById('bmcancel');
let bmPlan = null; // 第一步的结论：报告 + 一次性令牌

function bmReset() {
  bmPlan = null;
  bmReport.hidden = true;
  bmReport.textContent = '';
  bmApply.hidden = true;
  bmCancel.hidden = true;
}

// importBookmarks 是第一步：申请权限 → 读树 → 拼 Netscape → 发给服务端换报告。
// **没有用户手势 chrome.permissions.request 会直接被拒**，所以它必须整个待在
// 这个 click 处理里——这也是为什么这一段不能在页面加载时就跑。
async function importBookmarks() {
  const c = await collect();
  if (!c.base) return say(t('needBase'), 'bad');
  say(t('reading'));
  try {
    if (!(await ensureBookmarksPermission())) {
      bmReset();
      return say(t('noPerm'), 'bad');
    }
    const tree = await chrome.bookmarks.getTree();
    const html = buildNetscape(tree);
    bmPlan = await askPlan(c, html);
    // reportHtml is rendered by the same server-side renderer the web page uses,
    // so the extension does not paste markdown markers into the user's face.
    // Fall back to textContent when the server is older than this field.
    if (bmPlan.reportHtml) bmReport.innerHTML = bmPlan.reportHtml;
    else bmReport.textContent = bmPlan.report;
    bmReport.hidden = false;
    bmApply.hidden = false;
    bmCancel.hidden = false;
    // 跳过的条数**必须在这里说一句**：只报"读到 N 条"会让人以为文件里就
    // 这么多书签，而少的那几条去了哪里只有下面那份报告知道。
    const missed = bmPlan.skipped ? t('missed', bmPlan.skipped) : '';
    say(t('planSummary', bmPlan.count, bmPlan.groups, missed), 'ok');
  } catch (e) {
    bmReset();
    say(t('importFail', e.message), 'bad');
  }
}

// applyBookmarks 是第二步。令牌一次性：这一发点完就作废，重放也写不进第二次。
async function applyBookmarks() {
  const c = await collect();
  if (!bmPlan) return;
  bmApply.disabled = true;
  try {
    const r = await applyPlan(c, bmPlan);
    const warn = (r.warnings || []).length ? t('warnClause', r.warnings.join(t('warnSep'))) : '';
    bmReset();
    // **条数用计划里的 count（书签条数），不用 r.applied**：applied 数的是**应用了
    // 几条 op**（建一个分组也是一条 op），说成「导入 N 条」会把分组的数目算成
    // 书签的数目（openapi.json 与 AGENTS.md 都写明了这一点）。
    say(t('applied', bmPlan.count, r.rev, warn), 'ok');
  } catch (e) {
    bmApply.disabled = false;
    say(t('applyFail', e.message), 'bad');
  }
}

document.getElementById('bmimport').onclick = importBookmarks;
document.getElementById('bmapply').onclick = applyBookmarks;
document.getElementById('bmcancel').onclick = () => { bmReset(); say(t('cancelled'), 'ok'); };

applyI18n();
load();
