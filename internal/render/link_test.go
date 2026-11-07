package render

import (
	"github.com/rockbenben/launchbay/internal/model"
	"regexp"
	"strings"
	"testing"
)

func TestLinkModeCacheRuleSingleAddressAndExternal(t *testing.T) {
	s := seeded(t)
	save(t, s, "dad", "desk", "dsm")
	r := renderer(t, s)
	page := func(mode model.LinkMode, rule string) string {
		p, err := r.ProfilePage(PageRequest{ID: "dad", LinkMode: mode, LinkRule: rule})
		if err != nil {
			t.Fatal(err)
		}
		return string(p.Body)
	}
	lan, wan := page(model.LinkLAN, "private-ipv4"), page(model.LinkWAN, "wan-domain")
	li := regexp.MustCompile(`(?s)<li data-item="dsm".*?</li>`)
	if li.FindString(lan) == "" || li.FindString(lan) != li.FindString(wan) {
		t.Fatal("single-address DOM changed across modes")
	}
	if strings.Contains(li.FindString(wan), "data-addr-") {
		t.Fatal("single-address DOM grew")
	}
	manual := page(model.LinkLAN, "manual")
	if lan == manual {
		t.Fatal("same-mode cache lost different rule tooltip")
	}
	b, _ := s.LoadBookmarks("dad")
	b.Groups[0].Layout = model.LayoutFeature
	b.Groups[0].Items[0].URL = "file:///C:/readme"
	b.Groups[0].Items[0].URLs = &model.ItemURLs{WAN: "https://nas.example.com/a?q=%22x%22&y=1"}
	if err := s.SaveBookmarks("dad", b); err != nil {
		t.Fatal(err)
	}
	wan = page(model.LinkWAN, "manual")
	if !strings.Contains(wan, `href="https://nas.example.com/a?q=%22x%22&amp;y=1"`) || !strings.Contains(wan, `data-addr-lan="file:///C:/readme"`) {
		t.Fatal("feature item did not use effective protocol/escaped attributes")
	}
	b.Groups[0].Items[0].URLs.WAN = "javascript:alert(1)"
	if err := s.SaveBookmarks("dad", b); err != nil {
		t.Fatal(err)
	}
	wan = page(model.LinkWAN, "manual")
	if strings.Contains(wan, `href="javascript:`) || strings.Contains(wan, `href="https://nas.example.com`) {
		t.Fatal("bad WAN did not fall back")
	}
}
