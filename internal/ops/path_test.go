package ops

import "testing"

func TestParsePathAcceptsGrammar(t *testing.T) {
	for _, tc := range []struct {
		path                string
		wantKind            kind
		wantGroup, wantItem string
		wantField           string
	}{
		{path: "groups", wantKind: kindGroups},
		{path: "groups/nas", wantKind: kindGroup, wantGroup: "nas"},
		{path: "groups/nas/title", wantKind: kindGroupField, wantGroup: "nas", wantField: "title"},
		{path: "groups/nas/items", wantKind: kindItems, wantGroup: "nas"},
		{path: "groups/nas/items/jellyfin", wantKind: kindItem, wantGroup: "nas", wantItem: "jellyfin"},
		{path: "groups/nas/items/jellyfin/url", wantKind: kindItemField, wantGroup: "nas", wantItem: "jellyfin", wantField: "url"},
	} {
		got, err := ParsePath(tc.path)
		if err != nil {
			t.Fatalf("ParsePath(%q) 意外报错: %v", tc.path, err)
		}
		if got.kind != tc.wantKind || got.groupID != tc.wantGroup || got.itemID != tc.wantItem || got.field != tc.wantField {
			t.Fatalf("ParsePath(%q) = %+v", tc.path, got)
		}
	}
}

func TestParsePathRejectsEverythingElse(t *testing.T) {
	for _, path := range []string{
		"", "items", "groups/", "groups/nas/items/", "groups/nas/items/a/b/c",
		"groups/[0]/items", "groups/nas/nonsense", "groups/nas/items/jellyfin/password",
		"groups/nas/items/jellyfin/Id",
	} {
		if _, err := ParsePath(path); err == nil {
			t.Fatalf("ParsePath(%q) 该报错，却成功了", path)
		}
	}
}
