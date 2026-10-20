package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/rockbenben/launchbay/internal/apierr"
)

// 停用的档案在**令牌那一族**上的那一句（2026-10-20 补上）。
//
// 从前只有浏览器那条 PIN 门在拒停用的档案，档案作用域令牌**照读照写**——于是
// 「停用」对扩展与脚本是**空的**（扩展里那句「这一份被停用了」一次都不会出现）。
// 现在那六条（data/export/batch/revert/import/import-apply）都在
// profileDisabledRefuse 那一个判据上挡它。
//
// **403 而不是 401**（2026-10-20 改判）：停用不是凭证问题——令牌作用域正对、
// 凭证是好的，被拒的理由是这一份的状态。答 401 会把出路指反（让人以为自己的
// 令牌坏了去换一把，而它没坏）。这一条把浏览器那条 PIN 门也一起对齐了：
// **同一个 kind、同一个状态**（apierr 登记表自己的纪律：同一个码在两个端点上
// 答不同状态，读者就该怀疑其中一处写错了）。
func Test停用档案在令牌那一族答403(t *testing.T) {
	s, h := setupBatch(t)
	disableViaStore(t, s, "dad", true)

	// 那六条里挑三种形状的：读（data）、写（batch）、恢复（revert）。
	// 同一个 kind、同一个状态 —— 钉住它，免得哪天又分家。
	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"读", "GET", "/api/v1/data?profile=dad", ""},
		{"写", "POST", "/api/v1/batch", `{"profile":"dad","by":"ai","ops":[]}`},
		{"恢复", "POST", "/api/v1/revert?profile=dad&to-rev=1", ""},
	} {
		rec := post(h, tc.path, tc.body, "", dadTok)
		if tc.method == "GET" {
			rec = getBearer(h, tc.path, dadTok)
		}
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s 一条停用的档案该 403，得到 %d：%s", tc.name, rec.Code, rec.Body)
			continue
		}
		if !strings.Contains(rec.Body.String(), `"kind": "`+apierr.ProfileDisabled+`"`) {
			t.Errorf("%s 那条响应的 kind 不是 %q：%s", tc.name, apierr.ProfileDisabled, rec.Body)
		}
	}

	// 管理员那一族**不受这条约束**：停用要能被启回来，而管理员管的正是账号层面。
	if rec := post(h, "/api/v1/profiles/disable", `{"profile":"dad","disabled":false}`, "", "tok-admin"); rec.Code != http.StatusOK {
		t.Errorf("管理员要把停用启回来，该 200，得到 %d：%s", rec.Code, rec.Body)
	}
	if rec := getBearer(h, "/api/v1/data?profile=dad", dadTok); rec.Code != http.StatusOK {
		t.Errorf("启回来之后档案作用域令牌该读得动了，得到 %d：%s", rec.Code, rec.Body)
	}
}

// 浏览器那条 PIN 门（JSON 那一支）答的**也是 403**：同一个 kind、同一个状态。
//
// 这一条是上面那条的另一半——apierr 的登记表里 `profile-disabled` 只记一个状态码，
// 而从前浏览器那条 401、令牌那条 403，于是「同一个码在两个端点上答不同状态」。
func Test停用档案的PIN门也答403(t *testing.T) {
	s, h := setupBatch(t)
	disableViaStore(t, s, "kid", true)

	rec := post(h, "/pin", `{"profile":"kid","pin":"5678"}`, "", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("停用的档案走 JSON 那条 PIN 门该 403，得到 %d：%s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"kind": "`+apierr.ProfileDisabled+`"`) {
		t.Errorf("那一条响应的 kind 不是 %q：%s", apierr.ProfileDisabled, rec.Body)
	}
}
