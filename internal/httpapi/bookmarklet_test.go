package httpapi

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// bookmarklet 那条链（spec §6.3）：`/?add=<URL>&t=<标题>` → PIN 步骤 → 档案首屏
// → ⑤ 的浮层预填。
//
// **它在 ④ 之后一直是断的**：选择页逐卡片带了 `keep_add`，而 P1-④ 是零 JS 的、
// 没有"前端接住"这件事，于是 ?add= 在输完 PIN 那一步静默丢了。④ 的注释里写着
// "由前端（P1-④）或重定向接住"，当时两端都没落地。⑤ 有浮层了才接得起来。
func TestBookmarklet_add穿过PIN步骤不丢(t *testing.T) {
	_, h := setupServer(t)
	add := "http://10.0.0.3:5000/某页面?a=1&b=2"
	title := "带 & 和 ? 的标题"

	// 1) 未登录：选择页的每张卡片都得带上它
	rec := get(h, "/?add="+url.QueryEscape(add)+"&t="+url.QueryEscape(title), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("未登录应出选择页，实际 %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `name="keep_add"`) {
		t.Fatalf("选择页没带 keep_add：\n%.600s", body)
	}
	if !strings.Contains(body, "keep_title") {
		t.Errorf("选择页没带 keep_title——只带地址不带标题，进门后得自己再填一遍")
	}

	// 2) PIN 之后要接回来
	post := postForm(h, "/pin", (&url.Values{
		"profile": {"dad"}, "pin": {"1234"},
		"keep_add": {add}, "keep_title": {title},
	}).Encode(), "")
	if post.Code != http.StatusSeeOther {
		t.Fatalf("进门该 303，实际 %d", post.Code)
	}
	loc := post.Header().Get("Location")
	if !strings.HasPrefix(loc, "/?add=") {
		t.Fatalf("进门后的重定向没把 add 接回来，Location=%q", loc)
	}
	// 转义要正确：地址里本身带 ? & = ，不解义拼回去会截断成半条 URL
	gotAdd := loc[strings.Index(loc, "add=")+4:]
	if i := strings.Index(gotAdd, "&"); i > 0 {
		gotAdd = gotAdd[:i]
	}
	back, err := url.QueryUnescape(gotAdd)
	if err != nil || back != add {
		t.Errorf("add 没能原样回来：得到 %q，err=%v", back, err)
	}
}

// 已经登录时，?add= 要一路到首屏的 data 属性上（⑤ 的浮层从那里读）。
func TestBookmarklet_已登录时直接落在浮层的数据上(t *testing.T) {
	_, h := setupServer(t)
	dad := sessionCookie(t, loginAs(t, h, "dad", "1234"))
	add := "https://example.com/some/page"

	rec := get(h, "/?add="+url.QueryEscape(add)+"&t="+url.QueryEscape("标题"), dad)
	if rec.Code != http.StatusOK {
		t.Fatalf("该 200，实际 %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-add="`+add+`"`) {
		t.Errorf("首屏的 body 上没有 data-add，⑤ 的浮层就预填不了：\n%.600s", body)
	}
	if strings.Contains(body, "<a href=\"http://10.0.0.3:5000/某页面") {
		t.Error("add 里的地址被当成页面里的链接渲染了——它只是数据")
	}
}

// /bookmarklet 自己算地址，且**绝不吐出令牌**。
func TestBookmarklet_端点零令牌且认Host(t *testing.T) {
	_, h := setupServer(t)

	rec := getBearer(h, "/bookmarklet", "tok-admin")
	if rec.Code != http.StatusOK {
		t.Fatalf("/bookmarklet 该 200，实际 %d", rec.Code)
	}
	js := rec.Body.String()
	for _, bad := range []string{"tok-admin", "tok-dad", "Bearer ", "LAUNCHBAY_WRITE_TOKENS"} {
		if strings.Contains(js, bad) {
			t.Errorf("bookmarklet 里出现了 %q——令牌写进任何书签 URL 等于明文外发（§6.3）", bad)
		}
	}
	for _, want := range []string{"javascript:", "/?add=", "encodeURIComponent(location.href)", "encodeURIComponent(document.title)"} {
		if !strings.Contains(js, want) {
			t.Errorf("bookmarklet 里缺 %q", want)
		}
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Errorf("Content-Type = %q，要 text/plain（这是给人复制的一行，不是要被执行的脚本）", ct)
	}
}

// **端点存在而界面上没人提**，就是一个够不着的能力：家里人不会去猜这台机器上
// 还有一个 /bookmarklet。这条钉的是"管理页上找得到它，而且说清了怎么用"——
// 只给一个裸链接同样不合格，因为"怎么把这一行 javascript: 变成书签栏里那个能点
// 的按钮"没人天生知道。
// **这条钉的是整条地址，不是子串**（2026-10-19）。
//
// 原先 `TestBookmarklet_端点零令牌且认Host` 只断言 `javascript:`、`/?add=`
// 这些子串，于是 `handleBookmarklet` 把 Host 里的**端口剥掉**这件事一路绿灯
// 活到了今天：输出的地址恒是 `http://<host>/?add=`，而本案的默认端口是 7600
// （compose 的 `7600:7600`，spec §6.3 写的就是 `http://<NAS_IP>:7600/?add=`）。
// 按默认配置装好、配好书签栏拖进去之后，按它跳的是 80 端口——那台机器上 80
// 端口什么都没有。**症状是"按了没反应/打不开"，而这条按钮唯一的卖点是
// "配一次就永远对"**；只有把这台机器反代到 80 端口的人才会"看起来是好的"，
// 而 §11 明写不挂反代。
//
// 子串断言挡不住这一类：改坏的那一处，一个断言都没碰。
func TestBookmarklet_地址保留端口(t *testing.T) {
	_, h := setupServer(t)

	for _, tc := range []struct{ in, want string }{
		// 默认部署那一档：IPv4 与主机名都必须带端口活着
		{"10.0.0.5:7600", "http://10.0.0.5:7600/?add="},
		{"nas.local:7600", "http://nas.local:7600/?add="},
		{"127.0.0.1:17891", "http://127.0.0.1:17891/?add="},
		// 反代到 80 端口的那一档：没有端口就是没有端口，别自己加
		{"10.0.0.5", "http://10.0.0.5/?add="},
		// 端口是 80 时原样留着（浏览器照样通）：**不为"好看"去特判剥掉**——
		// 那正是这一段当初写错的那个形状，只是方向反过来。
		{"10.0.0.5:80", "http://10.0.0.5:80/?add="},
		// IPv6：切点必须在 ] 之后。裸写的 [fd00::5] 要是按"最后一个冒号"切，
		// 会被腰斩成 [fd00。
		{"[fd00::5]:7600", "http://[fd00::5]:7600/?add="},
		{"[fd00::5]", "http://[fd00::5]/?add="},
	} {
		req := httptest.NewRequest(http.MethodGet, "/bookmarklet", nil)
		req.Host = tc.in
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("Host=%q 该 200，实际 %d", tc.in, rec.Code)
			continue
		}
		if js := rec.Body.String(); !strings.Contains(js, "'"+tc.want) {
			t.Errorf("Host=%q → 地址被改坏了：想要 %q，整条是 %s", tc.in, tc.want, js)
		}
	}
}

// 形状不合法仍是 400 `host-invalid`，且**端口那一段不许放行非数字**：
// `validHostShape` 跟着地址形状一起改，放行了冒号不等于放行一切。
func TestBookmarklet_不合法形状仍400(t *testing.T) {
	_, h := setupServer(t)

	for _, bad := range []string{
		`evil"onload=x`,  // 引号：它要拼进 JS 字符串字面量
		`a:b:c`,          // 两个冒号不是 host:port
		`10.0.0.5:76 00`, // 端口里带空格
		`10.0.0.5:port`,  // 端口不是数字
	} {
		req := httptest.NewRequest(http.MethodGet, "/bookmarklet", nil)
		req.Host = bad
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("Host=%q 该 400，实际 %d（形状不合法却放行了）", bad, rec.Code)
		}
	}
}

func TestBookmarklet_管理页上有入口和用法(t *testing.T) {
	_, h := setupServer(t)
	// 管理页现在要管理权（2026-10-12）：管理员是机器级的一把 key，登进 dad 不算。
	admin := adminOnlyCookie(t)

	rec := get(h, "/admin", admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("/admin 该 200，实际 %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`href="/bookmarklet"`, "javascript:", "书签栏"} {
		if !strings.Contains(body, want) {
			t.Errorf("管理页上找不到「%s」——bookmarklet 够不着了", want)
		}
	}
	// 管理页是零 JS 页面（§8.1），给它挂脚本就是新开了一个改动面却换不到任何
	// 功能：那段说明是纯静态的。
	if strings.Contains(body, "<script") {
		t.Error("管理页不许有脚本——它这一族的判据就是零 JavaScript")
	}
}
