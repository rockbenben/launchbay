// options.js —— 扩展的设置页。
//
// 它把令牌放在 chrome.storage.local（**不是 sync**）：sync 跟着浏览器账号上云，
// 而那是这台机器的写权限。这一页里没有任何"导出配置"按钮，也正是同一个理由。
const KEY = 'lb-config';

const rows = document.getElementById('rows');
const msg = document.getElementById('msg');

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
  add.textContent = '＋ 再加一枚（另一个档案）';
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
  name.placeholder = 'dad';
  name.setAttribute('aria-label', '档案 id');
  name.style.flex = '0 0 9em';
  const tok = document.createElement('input');
  tok.type = 'password';
  tok.value = token;
  tok.placeholder = '这一枚档案作用域的令牌';
  tok.setAttribute('aria-label', (id || '这个档案') + ' 的令牌');
  tok.autocomplete = 'off';
  tok.dataset.profile = id;
  const del = document.createElement('button');
  del.type = 'button';
  del.textContent = '删掉这一枚';
  // 删掉就是**从待保存的表里移除**：save() 只从现存 DOM 收集，所以点完再保存才生效
  // （和"填了不保存不算数"是同一条规矩）。旧代码在这里往 dataset 上写一个
  // `__del__<id>` 的标记，而 save() 遍历的是 `rows.querySelectorAll` —— 那一行
  // 早就被 row.remove() 摘掉了，标记没人看得见：半套机制，删了。
  del.onclick = () => { row.remove(); say('这一枚要在下一次点「存设置」时才真的去掉。', ''); };
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
  say('已保存。', 'ok');
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
  if (!c.base) return say('先填地址。', 'bad');
  const id = c.defaultProfile || Object.keys(c.tokens)[0];
  const token = c.tokens[id];
  if (!id || !token) return say('先填一枚档案作用域的令牌（档案 id 和它的令牌要在同一行）。', 'bad');
  try {
    const r = await fetch(c.base + '/api/v1/data?profile=' + encodeURIComponent(id), {
      headers: { Authorization: 'Bearer ' + token },
    });
    if (r.ok) return say(`连上了，这枚令牌能开「${id}」这一份。`, 'ok');
    if (r.status === 403) return say(`连上了，但这枚令牌管不到「${id}」——它的作用域是别的档案。把 id 改成令牌对应的那一份，或换一枚令牌。`, 'bad');
    if (r.status === 401) return say('这台机器不认这枚令牌：多半是少复制了字符，或者机器那边换过令牌。', 'bad');
    if (r.status === 404) return say(`这台机器上没有叫「${id}」的档案。`, 'bad');
    return say('这台机器答了 HTTP ' + r.status + '，地址或档案可能不对。', 'bad');
  } catch (e) {
    return say('连不上：' + e.message + '（地址里含端口吗？例如 http://10.0.0.3:7600）', 'bad');
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
  if (!c.base) return say('先填地址。', 'bad');
  say('正在读书签…');
  try {
    if (!(await ensureBookmarksPermission())) {
      bmReset();
      return say('没有拿到「读书签」这个权限，导不了。（你可以只走网页那条路：手动导出 .html 再上传。）', 'bad');
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
    const missed = bmPlan.skipped ? `，另有 ${bmPlan.skipped} 条没能进来（报告里逐条列了原因）` : '';
    say(`读到 ${bmPlan.count} 条书签、${bmPlan.groups} 个分组${missed}。先看下面那份报告，确认了才写进去。`, 'ok');
  } catch (e) {
    bmReset();
    say('没导成：' + e.message, 'bad');
  }
}

// applyBookmarks 是第二步。令牌一次性：这一发点完就作废，重放也写不进第二次。
async function applyBookmarks() {
  const c = await collect();
  if (!bmPlan) return;
  bmApply.disabled = true;
  try {
    const r = await applyPlan(c, bmPlan);
    const warn = (r.warnings || []).length ? '（' + r.warnings.join('；') + '）' : '';
    bmReset();
    say(`已导入 ${r.applied} 条，rev 现在是 ${r.rev}${warn}`, 'ok');
  } catch (e) {
    bmApply.disabled = false;
    say('没写进去：' + e.message, 'bad');
  }
}

document.getElementById('bmimport').onclick = importBookmarks;
document.getElementById('bmapply').onclick = applyBookmarks;
document.getElementById('bmcancel').onclick = () => { bmReset(); say('没导入，书签一点没动。', 'ok'); };

load();
