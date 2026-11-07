package ops

import (
	"encoding/json"
	"github.com/rockbenben/launchbay/internal/model"
	"strings"
	"testing"
)

func TestWANSetClearAndAtomicity(t *testing.T) {
	path := "groups/nas/items/dsm/urls/wan"
	original := seed()
	b, errs, _ := Run(original, []Op{{Op: "set", Path: path, Value: mustValue(t, "https://nas.example.com/a?x=1&y=2")}}, nil)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if original.Groups[0].Items[0].URLs != nil {
		t.Fatal("input mutated")
	}
	it := b.Groups[0].Items[0]
	if it.URL != original.Groups[0].Items[0].URL || it.URLs.WAN != "https://nas.example.com/a?x=1&y=2" {
		t.Fatal(it)
	}
	cleared, errs, _ := Run(b, []Op{{Op: "set", Path: path, Value: mustValue(t, "")}}, nil)
	if len(errs) > 0 || cleared.Groups[0].Items[0].URLs != nil {
		t.Fatal(errs, cleared)
	}
	raw, _ := json.Marshal(cleared)
	if strings.Contains(string(raw), `"urls"`) {
		t.Fatal(string(raw))
	}
	for _, value := range []json.RawMessage{json.RawMessage(`null`), json.RawMessage(`{}`), json.RawMessage(`1`), mustValue(t, "file:///C:/x"), mustValue(t, "https://例え.テスト/")} {
		out, errs, _ := Run(b, []Op{{Op: "set", Path: path, Value: value}}, nil)
		if len(errs) != 1 || errs[0].Field != "urls/wan" {
			t.Fatalf("%s: %+v", value, errs)
		}
		_ = out
		if b.Groups[0].Items[0].URLs.WAN != it.URLs.WAN {
			t.Fatal("failed batch mutated input")
		}
	}
	if _, err := ParsePath("groups/nas/items/dsm/urls/lan"); err == nil || !strings.Contains(err.Error(), "主地址") {
		t.Fatal(err)
	}
	if sh := ShapeOf(path); sh.Kind != ShapeItemURLsWAN || sh.Field != "urls/wan" {
		t.Fatal(sh)
	}
}

func TestWANAddUsesSameValidationForItemsAndGroups(t *testing.T) {
	for _, value := range []string{"https://nas.example.com", "file:///C:/x", ""} {
		it := model.Item{ID: "new", Name: "new", URL: "http://10.0.0.5", URLs: &model.ItemURLs{WAN: value}}
		for _, op := range []Op{
			{Op: "add", Path: "groups/nas/items", Value: mustValue(t, it)},
			{Op: "add", Path: "groups", Value: mustValue(t, model.Group{ID: "new", Title: "new", Items: []model.Item{it}})},
		} {
			out, errs, _ := Run(seed(), []Op{op}, nil)
			if value == "file:///C:/x" {
				if len(errs) != 1 || errs[0].Field != "urls/wan" {
					t.Fatal(errs)
				}
				continue
			}
			if len(errs) > 0 {
				t.Fatal(errs)
			}
			if value == "" {
				raw, _ := json.Marshal(out)
				if strings.Contains(string(raw), `"urls"`) {
					t.Fatal(string(raw))
				}
			}
		}
	}
}
