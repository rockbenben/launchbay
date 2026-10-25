package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 服务端表单的原生校验要关掉（2026-10-24）。
//
// 添加浮层在 2026-10-22 去掉 `required`、2026-10-24 又因为 `type=url` 加上了
// `novalidate`——理由只有一个：浏览器原生气泡**跟着浏览器界面语言走**，而且会
// 抢在产品自己的 submit 前面。服务端渲染的表单（建档 / 登录 / 设 PIN / 导入）
// 一直没跟上：别人的浏览器是英文时，中文页面上先弹一句英文「Please fill out
// this field」，而页面那边准备好的 `noname` / `badpin` / `badkey` / 「没有选文件」
// 永远到不了。
//
// 这一条钉的是**表单壳**：下面这些 action 的表单必须带 `novalidate`。`required`
// / `minlength` / `maxlength` 保留——它们还给读屏"这一格必填"的语义，浏览器
// 只是不再替产品决定怎么报错。
func Test服务端表单关掉原生校验(t *testing.T) {
	web := "web"
	checks := []struct{ file, marker string }{
		{"picker.tmpl", `action="/setup"`},
		{"picker.tmpl", `action="/pin"`},
		{"picker.tmpl", `action="/new"`},
		{"admin.tmpl", `action="/admin" class="adm newprofile"`},
		{"adminlogin.tmpl", `action="/admin/login"`},
		{"adminaction.tmpl", `action="{{.Action}}" class="act"`},
		{"import.tmpl", `action="/import" enctype`},
		{"profile.tmpl", `action="/prefs/pin?tab=pin"`},
	}
	for _, c := range checks {
		b, err := os.ReadFile(filepath.Join(web, c.file))
		if err != nil {
			t.Fatalf("读 %s: %v", c.file, err)
		}
		src := string(b)
		at := strings.Index(src, c.marker)
		if at < 0 {
			t.Errorf("%s 里找不到 %q——测试的锚点漂了，不是表单缺 novalidate", c.file, c.marker)
			continue
		}
		start := strings.LastIndex(src[:at], "<form")
		if start < 0 {
			t.Errorf("%s 的 %q 不在 <form> 标签里", c.file, c.marker)
			continue
		}
		end := strings.Index(src[start:], ">")
		if end < 0 {
			t.Errorf("%s 的 <form> 标签没有闭合括号", c.file)
			continue
		}
		tag := src[start : start+end+1]
		if !strings.Contains(tag, "novalidate") {
			t.Errorf("%s 的 %s 表单没有 novalidate：原生气泡会先于页面语言的错误出现\n%s",
				c.file, c.marker, tag)
		}
	}
}
