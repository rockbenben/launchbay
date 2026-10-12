// import.js —— 把**浏览器自己的书签树**导进 launchbay。
//
// 为什么非得是扩展：书签数据住在浏览器主进程，网页（渲染进程）拿不到，CDP 也没有
// 这个接口。`chrome.bookmarks` / `browser.bookmarks` 是扩展专属的，而且不在
// content script 里。这是平台边界，不是"还没做"。
//
// 为什么权限放在 optional_permissions：Chrome 商店对 `bookmarks` 权限会弹醒目警告
// （"你的书签能透露大量隐私"）。为"右键收藏当前页"装这个扩展的人不该被那句话吓跑，
// 所以把它列进 optional_permissions，只在用户真的点了下面那枚按钮时才
// `chrome.permissions.request()`（必须在用户手势里调用）。
//
// 为什么在扩展里拼 Netscape HTML 而不是发 JSON：服务端那条 `bookimport.Convert`
// 吃的就是 Netscape 格式，而所有浏览器的导出也都是这个格式。在扩展里拼一份发过去，
// 服务端**一行都不用改**，它与"手动导出文件再上传"走的是同一个 Convert。
const esc = (s) => String(s == null ? '' : s)
  .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

// Chrome/Firefox 的书签树：叶子节点带 url，其余带 children。
// 空的文件夹直接不进输出——建一个空分组在 launchbay 那边只是个空壳。
function nodeToHtml(node) {
  if (node.url) {
    const when = node.dateAdded ? Math.floor(node.dateAdded / 1000) : 0;
    return `<DT><A HREF="${esc(node.url)}" ADD_DATE="${when}">${esc(node.title || node.url)}</A>\n`;
  }
  const kids = (node.children || []).filter((c) => c.url || (c.children || []).length);
  if (!kids.length) return '';
  let out = `<DT><H3>${esc(node.title || '未命名')}</H3>\n<DL><p>\n`;
  for (const k of kids) out += nodeToHtml(k);
  return out + '</DL><p>\n';
}

// buildNetscape 把 getTree() 的结果拼成一份 Netscape 书签文件。
//
// 浏览器那几个内置根文件夹（书签栏／其他书签／手机书签）**按它们自己的标题**进
// 输出，不按 id 硬编码成某个名字：标题是浏览器按当前语言给的，认它比认 id 准
// （Chrome 固定 id 1/2/3，Firefox 的树根完全是另一套）。
function buildNetscape(tree) {
  let body = '';
  for (const root of tree) {
    for (const child of (root.children || [])) body += nodeToHtml(child);
  }
  return '<!DOCTYPE NETSCAPE-Bookmark-file-1>\n' +
    '<META HTTP-EQUIV="Content-Type" CONTENT="text/html; charset=UTF-8">\n' +
    '<TITLE>Bookmarks</TITLE>\n<H1>Bookmarks</H1>\n<DL><p>\n' + body + '</DL><p>\n';
}

// ensurePermission 必须在用户手势里调用，否则 Chrome 直接拒绝。
async function ensureBookmarksPermission() {
  const have = await chrome.permissions.contains({ permissions: ['bookmarks'] });
  if (have) return true;
  return chrome.permissions.request({ permissions: ['bookmarks'] });
}

// pickProfile 挑一枚档案作用域的令牌。**管理员那把裸 key 不进这张表**——它是
// LAUNCHBAY_ADMIN_TOKEN，不是 name:secret:<档案 id> 那一族，填进任何一行都必然
// 401/403（服务端只管账号，管不到任何人的书签）。所以这里挑不到就是挑不到，
// 没有就直说，而不是发一个必然被拒的请求。
function pickProfile(cfg) {
  const ids = Object.keys(cfg.tokens || {});
  if (cfg.defaultProfile && cfg.tokens[cfg.defaultProfile]) return cfg.defaultProfile;
  return ids[0] || '';
}

// askPlan 走第一步：发原始 HTML 换一份"会写什么"的报告。不写盘。
async function askPlan(cfg, html) {
  const id = pickProfile(cfg);
  if (!id) throw new Error('还没有一枚档案作用域的令牌——先在上面填一枚。');
  const r = await fetch(cfg.base + '/api/v1/import', {
    method: 'POST',
    headers: { Authorization: 'Bearer ' + cfg.tokens[id], 'Content-Type': 'text/html; charset=utf-8' },
    body: html,
  });
  const text = await r.text();
  if (!r.ok) {
    let detail = text;
    try { detail = JSON.parse(text).detail || text; } catch { /* 不是 JSON 就原样带出来 */ }
    throw new Error('HTTP ' + r.status + '：' + detail);
  }
  // 服务端回的 profile 就是令牌作用域那个；本地这枚 id 只用于第二步取同一枚令牌。
  return { ...JSON.parse(text), localProfile: id };
}

// applyPlan 走第二步：确认写入。
async function applyPlan(cfg, plan) {
  const r = await fetch(cfg.base + '/api/v1/import/apply', {
    method: 'POST',
    headers: { Authorization: 'Bearer ' + cfg.tokens[plan.localProfile], 'Content-Type': 'application/json' },
    body: JSON.stringify({ token: plan.token }),
  });
  const text = await r.text();
  if (!r.ok) {
    let detail = text;
    try { detail = JSON.parse(text).detail || text; } catch { /* 同上 */ }
    throw new Error('HTTP ' + r.status + '：' + detail);
  }
  return JSON.parse(text);
}
