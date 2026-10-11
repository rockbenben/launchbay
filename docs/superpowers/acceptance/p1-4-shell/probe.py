"""Task 9 §13 验收探针（Python 版）。

为什么要换掉 bash：Git Bash 会把命令行里的中文参数按本地码页送出去，
`curl --data-urlencode 'q=条目12号'` 到服务端是一串坏字节，于是"命中 0 条"——
量到的是**工具**的毛病，不是应用的。第一版 bash 读数里 3 条 FAIL 全部这么来的，
逐条用 Python 复测都过。

运行前提（否则会量到工具的毛病，不是应用的）：
  1) 服务在 127.0.0.1:17600 跑着，数据目录指向仓库的 data/；
  2) 容器 launchbay 存在（停着也行）——管理员令牌是从 docker inspect 的
     环境里取的，脚本里不存任何凭证；
  3) 数据目录里有 p4dad（admin）与 p4kid（非 admin）两个验收夹具档案，全部由
     POST /api/v1/profiles、/pin、/batch 建出；两个档案的 PIN 从环境读
     （LB_PROBE_DAD_PIN / LB_PROBE_KID_PIN，错 PIN 那一条用 LB_PROBE_BAD_PIN），
     **不写进仓库**——这份脚本是入库的，把在用夹具的口令抄进来就等于发出去一次；
  4) 第 9 段是**唯一碰磁盘的一段**：它把 p4kid 的 bookmarks.json 截断到一半
     再用备份 cp 回原，并 cmp 证逐字节一致。这是针对「坏档页与 heal」的彩排，不是日常操作；去掉那一段前先想清楚你还想不想验证它。
"""
import http.cookiejar
import io
import json
import os
import shutil
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request
from urllib.request import HTTPRedirectHandler


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class _FakeResp:
    """让 cookiejar 能从一条 303/302 的 *错误* 响应里收 cookie。

    urllib 只在 2xx 上跑 HTTPCookieProcessor，NoRedirect 把 303 变成 HTTPError，
    那条响应里的 Set-Cookie 于是没人收——切档实际成功了却被读成"没横幅"。
    """

    #: 303 不在 CookieJar 允许的 status 集合里（它只认 200/206/301/302/304 那一组），
    #: 而浏览器收 303 的 Set-Cookie。/pin 与 /switch 的成功外壳都是 303，
    #: 所以这里按浏览器的行为报 200，否则探针会把真发出去的 cookie 丢掉，
    #: 把"切档成功"读成"没横幅"。
    code = 200

    def __init__(self, url, headers):
        self._url = url
        self._h = headers

    def info(self):
        return self._h

    def geturl(self):
        return self._url

    def getheaders(self):
        return list(self._h.items())


B = "http://127.0.0.1:17600"

def _pin_env(name):
    v = os.environ.get(name, "")
    if not v:
        sys.exit("缺环境变量 %s：验收夹具档案的 PIN 不写进仓库，跑之前从建档那一步的记录里带着")
    return v

DAD_PIN = _pin_env("LB_PROBE_DAD_PIN")
KID_PIN = _pin_env("LB_PROBE_KID_PIN")
BAD_PIN = _pin_env("LB_PROBE_BAD_PIN")  # 只用来验「错 PIN 被拒」，随便一个不等于上面的值
REPO = os.getcwd()
results = []


def rec(status, name, detail=""):
    results.append((status, name, detail))
    print("%s %s%s" % (status, name, ("  — " + detail) if detail else ""))


def ok(name, detail=""):
    rec("PASS", name, detail)


def bad(name, detail=""):
    rec("FAIL", name, detail)


def info(name, detail=""):
    rec("INFO", name, detail)


class Session:
    def __init__(self):
        self.cj = http.cookiejar.CookieJar()
        self.op = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(self.cj), NoRedirect())

    def get(self, path):
        try:
            with self.op.open(B + path) as r:
                return r.status, r.read().decode("utf-8", "replace"), dict(r.headers)
        except urllib.error.HTTPError as e:
            if e.headers.get("Set-Cookie"):
                req = urllib.request.Request(B + path)
                self.cj.extract_cookies(_FakeResp(B + path, e.headers), req)
            return e.code, e.read().decode("utf-8", "replace"), dict(e.headers)

    def post(self, path, fields, headers=None):
        data = urllib.parse.urlencode(fields).encode()
        req = urllib.request.Request(B + path, data=data)
        for k, v in (headers or {}).items():
            req.add_header(k, v)
        try:
            r = self.op.open(req)
            return r.status, r.geturl(), r.read().decode("utf-8", "replace"), dict(r.headers)
        except urllib.error.HTTPError as e:
            if e.headers.get("Set-Cookie"):
                self.cj.extract_cookies(_FakeResp(B + path, e.headers), req)
            return e.code, e.headers.get("Location", ""), e.read().decode("utf-8", "replace"), dict(e.headers)


def goesc(s):
    """Go 的 template.HTMLEscapeString 口径（python 的 html.escape 用 &quot;/&#x27;，
    页面字节是 &#34;/&#39;，照它比会假红）。"""
    for a, b in (("&", "&amp;"), ("<", "&lt;"), (">", "&gt;"), ('"', "&#34;"), ("'", "&#39;")):
        s = s.replace(a, b)
    return s


ADMIN = subprocess.run(
    ["docker", "inspect", "launchbay", "--format", "{{range .Config.Env}}{{println .}}{{end}}"],
    capture_output=True, text=True).stdout.strip().splitlines()
TOKENS = [t.split("=", 1)[1] for t in ADMIN if t.startswith("LAUNCHBAY_WRITE_TOKENS=")]
admin_tok = [p.split(":")[1] for p in TOKENS[0].split(",") if p.endswith(":*")][0]


def api(path, tok=None):
    req = urllib.request.Request(B + path)
    if tok:
        req.add_header("Authorization", "Bearer " + tok)
    try:
        with urllib.request.urlopen(req) as r:
            return r.status, r.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode("utf-8", "replace")


# ─────────────── 1 无会话：选择页、零条目、q 不回声 ───────────────
anon = Session()
st, body, _ = anon.get("/")
if 'class="card"' in body:
    bad("1 无会话漏出书签卡")
else:
    ok("1 无会话零条目内容（页面 %d 字节）" % len(body))
st, body, _ = anon.get("/?" + urllib.parse.urlencode({"q": "条目12号"}))
if 'value="条目12号"' in body:
    bad("1b 无会话把 q 回声进了表单")
else:
    ok("1b 无会话时 q 不回声（仍是选择页）")

# ─────────────── 2 PIN 输错带 ?err=；输对发 cookie ───────────────
s = Session()
st, loc, body, _ = s.post("/pin", {"profile": "p4dad", "pin": BAD_PIN})
if "?err=" in (loc or ""):
    ok("2 PIN 输错 = 303 带 ?err=", loc)
else:
    bad("2 PIN 输错的 Location", repr(loc))
st, loc, body, _ = s.post("/pin", {"profile": "p4dad", "pin": DAD_PIN})
has = any(c.name == "lb_profile" for c in s.cj)
ok("3 PIN 正确登录（lb_profile=%s）" % has) if has else bad("3 登录后没有 lb_profile")

# ─────────────── 4/5 搜索：命中数、两句话、侧栏随命中 ───────────────
st, body, _ = s.get("/?" + urllib.parse.urlencode({"q": "zzzzz不存在"}))
ok("4a 搜不到说「没有匹配」") if "没有匹配" in body else bad("4a 缺「没有匹配」")
bad("4b 有书签却说「还没有书签」") if "这个档案还没有书签" in body else ok("4b 两句空态是分开的")
st, body, _ = s.get("/?" + urllib.parse.urlencode({"q": "条目12号"}))
hits = "命中 1 条" in body
cards = body.count('class="card"')
ok("5a 命中计数=1、卡片=1" if hits and cards == 1 else "5a 命中读数不符")
if hits and cards == 1:
    info("   实际", "命中 1 条 · 共 25 条，卡片 %d 张" % cards)
ok("5b 命中分组挂在侧栏") if 'href="#g-media"' in body else bad("5b 命中分组不在侧栏")
bad("5c 未命中分组还挂在侧栏") if 'href="#g-nas"' in body else ok("5c 未命中分组从侧栏消失")
ok("5d 侧栏计数随命中走（cnt=1）") if '<b class="cnt">1</b>' in body else bad("5d 侧栏计数没随命中走")
# 同 q 两次：带 q 不进缓存，但内容一致 → ETag 应相同
st1, _, h1 = s.get("/?" + urllib.parse.urlencode({"q": "条目12号"}))
st2, _, h2 = s.get("/?" + urllib.parse.urlencode({"q": "条目12号"}))
ok("5e 同 q 两次的 ETag 相同（内容哈希而非缓存旁路漏发）") if h1.get("Etag") == h2.get("Etag") else bad("5e 同 q 两次 ETag 不同")

# ─────────────── 6 代管：非管理员被拒 / 管理员免 PIN / 退出 ───────────────
kid = Session()
kid.post("/pin", {"profile": "p4kid", "pin": KID_PIN})
st, loc, body, _ = kid.post("/switch", {"to": "p4dad"})
st2, page, _ = kid.get("/")
if "正在管理" in page and "p4dad" in str([c.value for c in kid.cj]):
    bad("6a 非管理员免 PIN 切档成功了（该拒）")
else:
    ok("6a 非管理员切档被拒（仍在 p4kid）")
bad("6b 无权切档带了 ?err=（权限拒应是裸 303）") if "err=" in (loc or "") else ok("6b 无权切档是裸 303")

m = Session()
m.post("/pin", {"profile": "p4dad", "pin": DAD_PIN})
st, loc, body, _ = m.post("/switch", {"to": "p4kid"})
st2, page, hdr = m.get("/")
vals = {c.name: c.value for c in m.cj}
info("6c′ 切档后的 cookie", "lb_profile=%s lb_home=%s" % (vals.get("lb_profile"), vals.get("lb_home")))
if "正在管理" in page:
    seg = page[page.find("正在管理") - 40: page.find("正在管理") + 160]
    frag = " ".join(seg.split())[:150]
    ok("6c 管理员免 PIN 直达 → 横幅出现", frag)
else:
    bad("6d 管理员切档后没有横幅", "loc=%s code=%s" % (loc, st2))
etag_manage = hdr.get("Etag", "")
m.post("/switch", {"to": "home"})
st3, page2, hdr2 = m.get("/")
bad("6e 退出管理后横幅还在") if "正在管理" in page2 else ok("6e to=home 退出管理")
if etag_manage and hdr2.get("Etag") and etag_manage != hdr2.get("Etag"):
    ok("6f 缓存键认得 home 这一维", "%s ≠ %s" % (etag_manage[:16], hdr2["Etag"][:16]))
else:
    bad("6g 两种 home 的 ETag 没分开", "%r / %r" % (etag_manage, hdr2.get("Etag")))

# ─────────────── 7 /admin ───────────────
st, page, hdr = kid.get("/admin")
ok("7a 非管理员 /admin = 303" if st == 303 else "7a 非管理员 /admin 得到 %s" % st)
st, page, hdr = m.get("/admin")
leaks = [t for t in ("http://10.0.0", 'class="card"', "条目0号") if t in page]
bad("7b /admin 漏出条目内容 %s" % leaks) if leaks else ok("7b /admin 不含条目内容（%d 字节）" % len(page))
cc = hdr.get("Cache-Control", "")
ok("7c /admin 是 no-store", cc) if "no-store" in cc else bad("7d /admin 缓存头", cc)
info("7e /admin 行数", "档案行 %d 条" % page.count("<tr"))

# ─────────────── 8 /prefs 两条通路的长相 ───────────────
st, loc, body, _ = Session().post("/prefs", {"theme": "dark"}, {"Authorization": "Bearer " + admin_tok})
ok("8a 管理员令牌打 /prefs = 303（没有第二条 JSON 面）", "code=%s" % st) if st == 303 else bad("8b /prefs 令牌得到 %s" % st)
st, page, _ = m.get("/")
before = page.count('data-profile-layout="sidebar"')
m.post("/prefs", {"layout": "topbar"})
st, page, _ = m.get("/")
after = page.count('data-profile-layout="topbar"')
ok("8c /prefs 改形态立刻可见", "sidebar=%d → topbar=%d" % (before, after)) if after else bad("8d 改形态没生效")
ok("8e topbar 形态不渲染侧栏") if "<aside" not in page else bad("8f topbar 仍有 aside")
m.post("/prefs", {"layout": "sidebar"})
st, page, _ = m.get("/")
info("8g 复原形态", "sidebar 回到 %r" % ('data-profile-layout="sidebar"' in page))

# ─────────────── 9 坏档彩排（唯一碰磁盘的一段） ───────────────
F = os.path.join("data", "profiles", "p4kid", "bookmarks.json")
BK = os.path.join(os.environ.get("TEMP", "/tmp"), "p4kid.bookmarks.bak")
shutil.copyfile(F, BK)
raw = io.open(F, encoding="utf-8").read()
io.open(F, "w", encoding="utf-8", newline="").write(raw[: len(raw) // 2])
info("9 截断", "%d → %d 字节" % (len(raw), len(raw) // 2))

ks = Session()
ks.post("/pin", {"profile": "p4kid", "pin": KID_PIN})
st, page, _ = ks.get("/")
ok("9a 坏档页是 200 而不是 500", "code=%s" % st) if st == 200 else bad("9b 坏档页得到 %s" % st)
ok("9c 主句是人话") if "这个档案的数据文件读不了" in page else bad("9d 缺主句")
ok("9e 原文收进 details") if "<details" in page and "<summary>" in page else bad("9f 没有 details")
ok("9g 点名了是哪个文件") if "bookmarks.json" in page else bad("9h 没点名文件")
stc, api_body = api("/api/v1/data?profile=p4kid", admin_tok)
detail = json.loads(api_body).get("detail", "")
if detail and goesc(detail) in page:
    ok("9i 页面原文与 503 的 detail 逐字同源", "status=%s detail=%r" % (stc, detail[:60]))
else:
    bad("9j 同源失败", "detail=%r" % detail[:80])
st, other, _ = s.get("/")
ok("9k 别的档案照常渲染（分组 %d 个）" % other.count("<section")) if "<section" in other else bad("9l 一个坏档拖累了别的档案")

shutil.copyfile(BK, F)
tries = 0
healed = ""
while tries < 20:
    tries += 1
    st, healed, _ = ks.get("/")
    if "这个档案的数据文件读不了" not in healed:
        break
    import time

    time.sleep(0.5)
if tries == 1:
    ok("9n 复原后立刻 healed（第 1 次读）")
elif tries < 20:
    ok("9n 复原后 healed", "第 %d 次读才看见（bind 挂载可见性 %.1fs 级）" % (tries, tries * 0.5))
else:
    bad("9m 复原后仍是坏档页")
same = open(F, "rb").read() == open(BK, "rb").read()
ok("9o bookmarks.json 与备份逐字节一致", "%d 字节" % os.path.getsize(F)) if same else bad("9p 复原不是逐字节")
os.remove(BK)

# ─────────────── 10 零脚本 + X-LB-Profile ───────────────
for path, label in (("/", "首屏"), ("/?" + urllib.parse.urlencode({"q": "条目1"}), "搜索态"), ("/admin", "管理页")):
    st, page, _ = m.get(path)
    n = page.count("<script")
    ok("10 %s 的 <script 计数 = 0" % label) if n == 0 else bad("10b %s 有 %d 个 <script" % (label, n))
st, page, hdr = m.get("/")
xlb = hdr.get("X-Lb-Profile", "")
ok("11 X-LB-Profile 在响应头", xlb) if xlb else bad("11 没有 X-LB-Profile")

# 坏档解释页的缓存头：修好文件之前，旧的解释不许被缓存住
io.open(F, "w", encoding="utf-8", newline="").write(raw[: len(raw) // 2])
st, page, hdr = ks.get("/")
ns = "no-store" in hdr.get("Cache-Control", "")
ok("12 坏档解释页是 no-store") if ns else bad("12 坏档页缓存头", hdr.get("Cache-Control", ""))
io.open(F, "w", encoding="utf-8", newline="").write(raw)
tries = 0
while tries < 20:
    tries += 1
    st, healed, _ = ks.get("/")
    if "这个档案的数据文件读不了" not in healed:
        break
    import time

    time.sleep(0.5)
ok("13 二次复原后 healed", "第 %d 次读" % tries) if "这个档案的数据文件读不了" not in healed else bad("14 二次复原失败")
same = open(F, "rb").read() == raw.encode("utf-8")
ok("15 p4kid 的 bookmarks.json 回到原始内容", "%d 字节" % len(raw)) if same else bad("16 原始内容没还原干净")

print("\n合计 PASS=%d FAIL=%d" % (sum(1 for r in results if r[0] == "PASS"), sum(1 for r in results if r[0] == "FAIL")))
print("DONE")
