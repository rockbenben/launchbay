package model_test

import (
	"net/netip"
	"testing"

	"github.com/rockbenben/launchbay/internal/model"
)

// effective(lan) = url；effective(wan) = urls.wan 非空且有效 ? urls.wan : url。
// 四条规则住在 model.EffectiveURL 一处：屏上 href、搜索、读侧净化都调它。
func TestEffectiveURL_四条规则(t *testing.T) {
	lan := model.Item{URL: "http://10.0.0.3:5000/"}
	both := model.Item{URL: "http://10.0.0.3:5000/", URLs: &model.ItemURLs{WAN: "https://nas.example.com/"}}
	broken := model.Item{URL: "http://10.0.0.3:5000/", URLs: &model.ItemURLs{WAN: "file:///C:/x"}}
	cases := []struct {
		name string
		it   model.Item
		mode model.LinkMode
		want string
	}{
		{"单地址-内网", lan, model.LinkLAN, "http://10.0.0.3:5000/"},
		{"单地址-外网落回主地址", lan, model.LinkWAN, "http://10.0.0.3:5000/"},
		{"双地址-内网", both, model.LinkLAN, "http://10.0.0.3:5000/"},
		{"双地址-外网", both, model.LinkWAN, "https://nas.example.com/"},
		{"坏的第二地址读侧净化", broken, model.LinkWAN, "http://10.0.0.3:5000/"},
		{"空串模式=自动=主地址", both, "", "http://10.0.0.3:5000/"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := model.EffectiveURL(c.it, c.mode); got != c.want {
				t.Fatalf("EffectiveURL = %q，要 %q", got, c.want)
			}
		})
	}
}

// 写侧闸只认 http/https + ASCII 主机名；file:/chrome:// 留给主地址那一格。
func TestCheckWanURL_闸住协议与非ASCII主机名(t *testing.T) {
	ok := []string{
		"http://10.0.0.3:5000/",
		"https://nas.example.com/x?a=1",
		"http://" + netip.AddrFrom4([4]byte{203, 0, 113, 9}).String() + ":7600/",
	}
	bad := []string{
		"",
		"file:///C:/x",
		"chrome://version/",
		"mailto:a@b",
		"javascript:alert(1)",
		"http://例え.テスト/",
		"http://",
		"http://a b/",
	}
	for _, u := range ok {
		if err := model.CheckWanURL(u); err != nil {
			t.Errorf("CheckWanURL(%q) 应通过，得到 %v", u, err)
		}
	}
	for _, u := range bad {
		err := model.CheckWanURL(u)
		if err == nil {
			t.Errorf("CheckWanURL(%q) 应拒，却通过了", u)
			continue
		}
		if got := err.Error(); !contains(got, "urls/wan") {
			t.Errorf("CheckWanURL(%q) 的错误要点名 urls/wan，得到 %q", u, got)
		}
	}
}

func TestValidLinkMode_空串合法而不认识的值非法(t *testing.T) {
	for _, s := range []string{"", "auto", "lan", "wan"} {
		if !model.ValidLinkMode(s) {
			t.Errorf("ValidLinkMode(%q) 应收", s)
		}
	}
	for _, s := range []string{"LAN", "proxy", " "} {
		if model.ValidLinkMode(s) {
			t.Errorf("ValidLinkMode(%q) 应拒", s)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
